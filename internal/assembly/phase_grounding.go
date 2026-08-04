package assembly

import (
	"context"

	"github.com/ac-prometheus/athena-class-agent/internal/awareness"
)

// GroundingPhase implements Phase for Phase 6 — environmental grounding.
// Surfaces time, timezone, location, and weather at session start.
// CharsUsed is always 0 — grounding is not charged against the dynamic budget.
type GroundingPhase struct{}

// NewGroundingPhase returns a GroundingPhase ready for use.
func NewGroundingPhase() *GroundingPhase { return &GroundingPhase{} }

func (p *GroundingPhase) Name() string     { return "grounding" }
func (p *GroundingPhase) Priority() int    { return 600 }
func (p *GroundingPhase) MinBudget() int   { return 1 }
func (p *GroundingPhase) IsFatal() bool    { return false }

// Assemble builds the Phase 6 grounding block from the operator-configured environment.
// Skippable under budget pressure (MinBudget=1). CharsUsed is 0 — not charged.
func (p *GroundingPhase) Assemble(_ context.Context, cfg *AssembleConfig, _ *DepthManifest, _ int) (PhaseResult, error) {
	gd := awareness.Gather(cfg.grounding)
	content := "=== GROUNDING ===\n" + gd.Format() + "\n"
	return PhaseResult{Content: content, CharsUsed: 0}, nil
}
