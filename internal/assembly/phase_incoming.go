package assembly

import (
	"context"
	"fmt"
	"strings"
)

// IncomingPhase implements Phase for Phase 5 — unread message count and inband notes.
// This phase is mandatory (MinBudget 0) and never charged against the dynamic budget.
// Message polling is handled by the tool layer; the count surfaces here so the agent
// knows whether to call check_messages immediately.
type IncomingPhase struct{}

// NewIncomingPhase returns an IncomingPhase ready for use.
func NewIncomingPhase() *IncomingPhase { return &IncomingPhase{} }

func (p *IncomingPhase) Name() string     { return "incoming" }
func (p *IncomingPhase) Priority() int    { return 500 }
func (p *IncomingPhase) MinBudget() int   { return 0 } // mandatory — never skip

// Assemble builds the Phase 5 incoming block: unread message count and any inband notes
// (e.g. interrupt notices from CheckpointScan). CharsUsed is always 0 — incoming is
// never charged against the dynamic budget.
func (p *IncomingPhase) Assemble(_ context.Context, cfg *AssembleConfig, manifest *DepthManifest, _ int) (PhaseResult, error) {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("=== INCOMING ===\nUnread messages: %d\n", manifest.UnreadMessages))
	for _, note := range cfg.InbandNotes {
		b.WriteString(note + "\n")
	}
	return PhaseResult{Content: b.String(), CharsUsed: 0}, nil
}
