package metabolism

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// Pipeline coordinates post-session metabolism: salience scoring,
// T2→T3 compression, and (future) dream cycle. Dispatched asynchronously
// from Session.End() via a durable job record.
type Pipeline struct {
	store  pkg.T2QueryStore
	aegis  pkg.ContentGateway
	scorer SalienceScorer
}

// NewPipeline creates a metabolism pipeline with the given dependencies.
func NewPipeline(store pkg.T2QueryStore, aegis pkg.ContentGateway) *Pipeline {
	return &Pipeline{
		store:  store,
		aegis:  aegis,
		scorer: NewHeuristicScorer(),
	}
}

// ProcessSession runs the metabolism pipeline for a completed session.
// Phase 1: Score salience on all T2 logs.
// Phase 2: Aegis-gated T2→T3 compression (separate file).
// Phase 3: Dream cycle (Sprint 4 — not implemented here).
func (p *Pipeline) ProcessSession(ctx context.Context, sessionID string) error {
	slog.Info("metabolism: starting", "session", sessionID)

	// Phase 1: Salience scoring
	// QueryLogs with limit=0 returns all logs for the session.
	logs, err := p.store.QueryLogs(ctx, sessionID, 0)
	if err != nil {
		return fmt.Errorf("metabolism: load T2 logs: %w", err)
	}

	scores, err := p.scorer.ScoreLogs(ctx, logs)
	if err != nil {
		slog.Warn("metabolism: salience scoring failed — using defaults", "err", err)
	} else {
		// ExperientialLog has no SalienceScore field — log the scored results
		// for observability. Compression (Phase 2) will consume them directly
		// from the SalienceResult slice rather than via a field on the log.
		for i, score := range scores {
			if i < len(logs) {
				slog.Debug("metabolism: salience score",
					"log_id", logs[i].ID,
					"score", score.Score,
					"compression_resist", score.CompressionResist,
					"reason", score.Reason,
				)
			}
		}
		slog.Info("metabolism: salience scored", "session", sessionID, "logs", len(logs))
	}

	// Phase 2: T2→T3 compression (TODO: implement in compress.go)
	// Phase 3: Dream cycle (Sprint 4)

	slog.Info("metabolism: complete", "session", sessionID)
	return nil
}
