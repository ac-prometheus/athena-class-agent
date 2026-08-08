package assembly

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ac-prometheus/athena-class-agent/internal/awareness"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// ContinuityPhase implements Phase for Phase 2 — bridge synthesis + recent T3 + active T4 threads.
// This phase is always attempted; errors are non-fatal (assembler logs and continues).
type ContinuityPhase struct{}

// NewContinuityPhase creates a ContinuityPhase.
func NewContinuityPhase() *ContinuityPhase {
	return &ContinuityPhase{}
}

func (p *ContinuityPhase) Name() string   { return "continuity" }
func (p *ContinuityPhase) Priority() int  { return 200 }
func (p *ContinuityPhase) MinBudget() int { return 0 }     // always attempted
func (p *ContinuityPhase) IsFatal() bool  { return false }

// Assemble builds the Phase 2 continuity block: orientation bridge synthesis, the two most
// recent T3 narrative summaries, and active T4 reflection threads. CharsUsed is charged against
// the dynamic budget. Errors are non-fatal — the assembler will log-and-continue.
func (p *ContinuityPhase) Assemble(ctx context.Context, cfg *AssembleConfig, manifest *DepthManifest, remaining int) (PhaseResult, error) {
	if cfg.store == nil {
		return PhaseResult{}, nil
	}
	result, abstained, err := p.assembleContinuity(ctx, cfg, manifest)
	if err != nil {
		return PhaseResult{}, err
	}
	return PhaseResult{Content: result, CharsUsed: len(result), BridgeAbstained: abstained}, nil
}

// assembleContinuity builds Phase 2 — bridge synthesis + most recent T3 + active T4 threads.
func (p *ContinuityPhase) assembleContinuity(
	ctx context.Context,
	cfg *AssembleConfig,
	manifest *DepthManifest,
) (string, bool, error) {
	var parts []string
	var abstained bool

	narratives, err := cfg.store.SearchNarrative(ctx, nil, 3)
	if err != nil {
		return "", false, fmt.Errorf("phase 2: T3 search: %w", err)
	}
	manifest.NarrativeTotal = len(narratives)
	if len(narratives) == 0 {
		manifest.AccessHints = append(manifest.AccessHints,
			"similarity search unavailable — retrieval is recency-only; use memory search for targeted lookups")
	}

	var recentNarrative string
	for i, n := range narratives {
		if i == 0 {
			recentNarrative = n.Content
		}
		manifest.NarrativeLoaded++
		parts = append(parts, n.Content)
		if i >= 1 {
			break // load at most 2 recent narratives in continuity phase
		}
	}

	reflections, err := cfg.store.SearchReflections(ctx, nil, 5)
	if err != nil {
		slog.Warn("phase 2: T4 search failed", "err", err)
	} else {
		manifest.ReflectionTotal = len(reflections)
		if len(reflections) == 0 {
			manifest.AccessHints = append(manifest.AccessHints,
				"reflection search unavailable — use memory search for targeted reflection lookups")
		}
		var activeThreads []string
		for _, r := range reflections {
			manifest.ReflectionLoaded++
			activeThreads = append(activeThreads, r.Content)
		}

		bridgeAllowed := cfg.llmFn != nil && recentNarrative != ""
		if bridgeAllowed {
			if cfg.Plan == nil {
				bridgeAllowed = false
				slog.Info("assembly: bridge skipped — no lifecycle plan (conservative default)")
			} else if cfg.Plan.BridgePolicy != pkg.BridgeAutoWithAbstention {
				bridgeAllowed = false
				slog.Info("assembly: bridge skipped per lifecycle policy", "policy", cfg.Plan.BridgePolicy)
			}
		}
		if bridgeAllowed {
			br, err := awareness.SynthesizeBridge(ctx, recentNarrative, activeThreads, cfg.bridge, cfg.llmFn)
			if err != nil {
				slog.Warn("phase 2: bridge synthesis failed", "err", err)
			} else {
				abstained = br.Abstained
				if !br.Abstained && br.Content != "" {
					parts = append([]string{"=== ORIENTATION BRIDGE ===\n" + br.Content}, parts...)
				}
			}
		}
	}

	if len(parts) == 0 {
		return "", abstained, nil
	}
	return "=== CONTINUITY ===\n" + strings.Join(parts, "\n---\n") + "\n", abstained, nil
}
