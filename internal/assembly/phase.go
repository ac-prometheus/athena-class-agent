package assembly

import "context"

// Phase is a single stage of context assembly. Phases are executed in Priority
// order to build the agent's Tier 1 working memory. The registry enables
// per-mode context generation and agent-customizable assembly.
type Phase interface {
	Name() string
	Priority() int    // lower = earlier; determines execution order
	MinBudget() int   // minimum remaining chars to run; 0 = never skip (mandatory)
	IsFatal() bool    // true = errors abort the session; false = log-and-continue
	Assemble(ctx context.Context, cfg *AssembleConfig, manifest *DepthManifest, remaining int) (PhaseResult, error)
}

// PhaseResult is the output of a single phase's assembly.
type PhaseResult struct {
	Content   string // text block to append to SystemPrompt
	CharsUsed int    // chars to deduct from remaining budget; 0 = not charged

	// Structured outputs for phases that return more than content.
	// nil when not applicable. Avoids type-asserting concrete phase types.
	IdentityDocs    any  // *identity.IdentityDocs when set by identity phase
	IntegrityReport any  // *identity.IntegrityReport when set by identity phase
	BridgeAbstained bool // set by continuity phase
}

// PhaseRegistry is an ordered list of phases, sorted by Priority.
type PhaseRegistry []Phase

// DefaultPhases returns the six standard context assembly phases.
func DefaultPhases(identityDir string) PhaseRegistry {
	return PhaseRegistry{
		NewIdentityPhase(identityDir),
		NewContinuityPhase(),
		NewWorldModelPhase(),
		NewEchoPoolPhase(),
		NewIncomingPhase(),
		NewGroundingPhase(),
	}
}
