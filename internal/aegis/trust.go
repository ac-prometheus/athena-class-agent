package aegis

import (
	"context"
	"errors"
	"log/slog"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// TrustConfig holds tunable parameters for the trust scorer.
type TrustConfig struct {
	SkepticalPrior   float64 // default trust for new sources
	RampInteractions int     // interactions needed to move past skeptical prior
	MaxTrust         float64 // ceiling on trust score
}

// DefaultTrustConfig returns production defaults.
func DefaultTrustConfig() TrustConfig {
	return TrustConfig{
		SkepticalPrior:   0.40,
		RampInteractions: 5,
		MaxTrust:         0.95,
	}
}

// TrustScorer computes trust scores for content sources.
type TrustScorer struct {
	cfg   TrustConfig
	store pkg.TrustStore
}

// NewTrustScorer constructs a TrustScorer.
func NewTrustScorer(cfg TrustConfig, store pkg.TrustStore) *TrustScorer {
	return &TrustScorer{cfg: cfg, store: store}
}

// Score returns the trust score for source after accounting for interaction history
// and any injection flags. Increments the interaction count as a side effect.
func (t *TrustScorer) Score(ctx context.Context, source string, flags []PatternMatch) (float64, error) {
	score, interactions, err := t.store.GetTrust(ctx, source)
	if err != nil {
		if !errors.Is(err, errNotFound) {
			return t.cfg.SkepticalPrior, err
		}
		// New source — use skeptical prior.
		score = t.cfg.SkepticalPrior
		interactions = 0
	}

	// Blend toward skeptical prior during ramp period.
	if interactions < t.cfg.RampInteractions {
		weight := float64(interactions) / float64(t.cfg.RampInteractions)
		score = t.cfg.SkepticalPrior + weight*(score-t.cfg.SkepticalPrior)
	}

	// Multiplicative penalty per injection flag, floor at 0.10.
	for range flags {
		score *= 0.7
		if score < 0.10 {
			score = 0.10
			break
		}
	}

	if score > t.cfg.MaxTrust {
		score = t.cfg.MaxTrust
	}

	if err := t.store.UpdateTrust(ctx, source, score); err != nil {
		slog.Warn("aegis: failed to update trust score", "source", source, "err", err)
	}
	if err := t.store.IncrementInteractions(ctx, source); err != nil {
		slog.Warn("aegis: failed to increment interactions", "source", source, "err", err)
	}

	return score, nil
}

// errNotFound is a sentinel used when GetTrust finds no row.
// Store implementations return this to distinguish missing vs. error.
var errNotFound = errors.New("trust: source not found")
