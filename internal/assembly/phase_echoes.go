package assembly

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log/slog"
	"strings"
)

// EchoPoolPhase implements Phase for Phase 4 — stochastic T3/T4 echo retrieval with
// inverse-recency bias. This phase is the first to be cut under budget pressure.
// When cfg.edges is set and contradictionProbability > 0, one contradicting belief
// may be stochastically appended to surface productive tension.
type EchoPoolPhase struct{}

// NewEchoPoolPhase returns an EchoPoolPhase ready for use.
func NewEchoPoolPhase() *EchoPoolPhase { return &EchoPoolPhase{} }

func (p *EchoPoolPhase) Name() string     { return "echo-pool" }
func (p *EchoPoolPhase) Priority() int    { return 400 }
func (p *EchoPoolPhase) MinBudget() int   { return 8000 }
func (p *EchoPoolPhase) IsFatal() bool    { return false }

// Assemble builds the Phase 4 echo pool block. Requires an embedding provider;
// returns an empty result (not an error) when the provider is nil.
// CharsUsed is charged against the dynamic budget.
func (p *EchoPoolPhase) Assemble(ctx context.Context, cfg *AssembleConfig, manifest *DepthManifest, remaining int) (PhaseResult, error) {
	if cfg.provider == nil {
		manifest.AccessHints = append(manifest.AccessHints,
			"echo pool requires embedding provider — use memory search for serendipitous retrieval")
		return PhaseResult{}, nil
	}

	// Seed with a generic introspection phrase; the stochastic slot favours older material
	// via inverse-recency weighting in the vector search implementation.
	seed := "past experience insight reflection memory"
	vec, err := cfg.provider.Embed(ctx, seed)
	if err != nil {
		return PhaseResult{}, fmt.Errorf("phase 4: seed embedding: %w", err)
	}

	echoes, err := cfg.store.SearchReflections(ctx, vec, 2)
	if err != nil {
		return PhaseResult{}, fmt.Errorf("phase 4: reflection echo search: %w", err)
	}
	if len(echoes) == 0 {
		return PhaseResult{}, nil
	}

	var parts []string
	echoIDs := make([]string, 0, len(echoes))
	for _, e := range echoes {
		parts = append(parts, summarize(e.Content, 300))
		echoIDs = append(echoIDs, e.ID)
	}

	// Stochastic contradiction retrieval: with configurable probability, surface
	// one belief that contradicts something already in the echo pool.
	if cfg.edges != nil && cfg.contradictionProbability > 0 && cryptoRandFloat() < cfg.contradictionProbability {
		if id, content, found := findContradiction(ctx, cfg, echoIDs); found {
			parts = append(parts, "[contradiction] "+summarize(content, 300))
			slog.Info("assembly: stochastic contradiction surfaced", "belief_id", id)
		}
	}

	content := "=== ECHO POOL ===\n" + strings.Join(parts, "\n---\n") + "\n"
	return PhaseResult{Content: content, CharsUsed: len(content)}, nil
}

// findContradiction queries memory_edges for contradicts edges linked to any echo pool member
// and returns the first contradicting belief's ID and content.
func findContradiction(ctx context.Context, cfg *AssembleConfig, echoIDs []string) (id, content string, found bool) {
	for _, echoID := range echoIDs {
		edges, err := cfg.edges.GetEdges(ctx, echoID, "from")
		if err != nil {
			continue
		}
		for _, e := range edges {
			if e.EdgeType != "contradicts" {
				continue
			}
			// Fetch the contradicting belief from T4.
			reflections, err := cfg.store.SearchReflections(ctx, nil, 20)
			if err != nil {
				continue
			}
			for _, r := range reflections {
				if r.ID == e.ToID {
					return r.ID, r.Content, true
				}
			}
		}
	}
	return "", "", false
}

// cryptoRandFloat returns a cryptographically random float64 in [0, 1).
// Used for contradiction retrieval gating where belief store privacy matters.
func cryptoRandFloat() float64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 1.0 // fail closed — don't surface contradiction on rand failure
	}
	return float64(binary.LittleEndian.Uint64(buf[:])>>11) / (1 << 53)
}
