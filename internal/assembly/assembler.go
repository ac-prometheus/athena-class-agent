package assembly

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/ac-prometheus/athena-class-agent/internal/awareness"
	"github.com/ac-prometheus/athena-class-agent/internal/identity"
	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// ContextAssembler builds the Tier 1 context window via registry-based phase execution.
// Phases are sorted by Priority and executed in order; budget-sensitive phases are
// skipped when remaining chars fall below their MinBudget threshold.
type ContextAssembler struct {
	identityDir string
	budget      int           // max tokens for dynamic context (rough: 4 chars ≈ 1 token)
	phases      PhaseRegistry // ordered by Priority; set by NewContextAssembler
}

// AssembledContext is the output of a full phase assembly.
type AssembledContext struct {
	SystemPrompt    string
	IdentityDocs    *identity.IdentityDocs
	IntegrityReport *identity.IntegrityReport
	BridgeAbstained bool
	DepthManifest   *DepthManifest
	Manifest        *pkg.AssemblyManifest // what was loaded, omitted, and why
}

// DepthManifest records how much of each tier was available vs. loaded.
// Appended to the system prompt as a structured text block so the agent
// knows what it's missing and can issue targeted retrieval calls.
type DepthManifest struct {
	NarrativeTotal   int
	NarrativeLoaded  int
	ReflectionTotal  int
	ReflectionLoaded int
	EntityTotal      int
	EntityLoaded     int
	ProfileTotal     int
	ProfileLoaded    int
	UnreadMessages   int
	AccessHints      []string
}

// AssembleConfig groups the runtime dependencies Assemble needs.
// Keeping them off the struct avoids turning ContextAssembler into a god object.
type AssembleConfig struct {
	store    pkg.MemoryStore
	edges    pkg.EdgeStore // for contradiction retrieval (Phase 6); may be nil
	anchors  pkg.IdentityAnchorStore
	provider pkg.EmbeddingProvider
	grounding awareness.GroundingConfig
	bridge   awareness.BridgeConfig
	llmFn    func(string) (string, error) // for bridge synthesis
	contradictionProbability float64      // 0.30 default; 0 disables

	// InbandNotes are injected at Phase 5 Incoming (e.g. interrupt notices from CheckpointScan).
	InbandNotes []string

	// DB is used for the witness check and other operational table queries.
	// May be nil; when nil, the witness check is skipped with a warning.
	// Deprecated: prefer AssemblyStore for all operational table access.
	DB platform.DB

	// AssemblyStore is the repository port for identity persistence operations
	// (witness letter check, operator action logging). When set, supersedes raw DB access.
	AssemblyStore pkg.AssemblyStore

	// SkipWitnessCheck bypasses the witness letter requirement when true.
	// Only for automated testing — logged explicitly to operator_actions when set.
	SkipWitnessCheck bool

	// Plan is the resolved LifecyclePlan from the lifecycle resolver.
	// When set, assembly uses it to select phases, configure bridge policy,
	// and populate the manifest with PlanID. When nil, assembly runs the
	// default full phase set (backward compatible with Phase 1 callers).
	Plan *pkg.LifecyclePlan

	// SessionID is set by the caller for manifest and plan linkage.
	SessionID string
}

// MinimalAssembleConfig returns an AssembleConfig with nil dependencies — suitable for
// Phase 1/2 sessions where memory, embeddings, and bridge aren't yet wired.
func MinimalAssembleConfig() AssembleConfig {
	return AssembleConfig{}
}

// SetStore wires the MemoryStore dependency into the config.
// Required for continuity phase (T3 narrative retrieval) and echo-pool phase.
// Returns the receiver pointer for method chaining.
func (c *AssembleConfig) SetStore(store pkg.MemoryStore) *AssembleConfig {
	c.store = store
	return c
}

// SetProvider wires the EmbeddingProvider into the config.
// Required for echo-pool phase (semantic echo retrieval).
// Returns the receiver pointer for method chaining.
func (c *AssembleConfig) SetProvider(provider pkg.EmbeddingProvider) *AssembleConfig {
	c.provider = provider
	return c
}

// SetBridge wires the BridgeConfig for orientation bridge synthesis.
// Only meaningful when the lifecycle plan's BridgePolicy is BridgeAutoWithAbstention.
// Returns the receiver pointer for method chaining.
func (c *AssembleConfig) SetBridge(bridge awareness.BridgeConfig) *AssembleConfig {
	c.bridge = bridge
	return c
}

// DefaultBridgeConfig returns the recommended bridge configuration for production sessions.
// AbstainRate of 0.20 means roughly one in five sessions skips bridge synthesis,
// preventing it from becoming a mechanical ritual (matches Aurora's design).
func DefaultBridgeConfig() awareness.BridgeConfig {
	return awareness.BridgeConfig{AbstainRate: 0.20}
}

// NewContextAssembler creates an assembler with the given identity directory and token budget.
// The phase registry is initialised via DefaultPhases; callers may replace it by setting
// the phases field directly if custom assembly is needed.
func NewContextAssembler(identityDir string, budget int) *ContextAssembler {
	if budget <= 0 {
		budget = 160000 // 85% of a 200k window, matching Aurora's design
	}
	return &ContextAssembler{
		identityDir: identityDir,
		budget:      budget,
		phases:      DefaultPhases(identityDir),
	}
}

// Assemble runs the registered phases in Priority order and returns a fully
// populated AssembledContext. Returns an error immediately if the identity phase
// fails — the session must not start under tampered or missing identity.
func (a *ContextAssembler) Assemble(ctx context.Context, cfg AssembleConfig) (*AssembledContext, error) {
	manifest := &DepthManifest{}
	var sections []string
	budgetChars := a.budget * 4 // chars available (4 chars ≈ 1 token)
	remaining := budgetChars

	// Defensive sort: DefaultPhases is already ordered, but custom registries may not be.
	phases := make(PhaseRegistry, len(a.phases))
	copy(phases, a.phases)
	sort.Slice(phases, func(i, j int) bool {
		return phases[i].Priority() < phases[j].Priority()
	})

	assembled := &AssembledContext{DepthManifest: manifest}

	// Phase tracking for AssemblyManifest.
	var phasesRun []string
	var phasesSkipped []string
	skipReasons := make(map[string]string)
	phaseCosts := make(map[string]int) // phase name → token estimate (chars/4)

	for _, phase := range phases {
		if phase.MinBudget() > 0 && remaining < phase.MinBudget() {
			slog.Info("assembly: phase skipped (budget pressure)",
				"phase", phase.Name(),
				"remaining", remaining,
				"minBudget", phase.MinBudget(),
			)
			reason := fmt.Sprintf("budget pressure (%d chars remaining, %d required)", remaining, phase.MinBudget())
			manifest.AccessHints = append(manifest.AccessHints,
				fmt.Sprintf("%s omitted — budget pressure", phase.Name()))
			phasesSkipped = append(phasesSkipped, phase.Name())
			skipReasons[phase.Name()] = reason
			continue
		}

		result, err := phase.Assemble(ctx, &cfg, manifest, remaining)
		if err != nil {
			if phase.IsFatal() {
				return nil, err
			}
			slog.Warn("assembly: phase partial failure — continuing",
				"phase", phase.Name(), "err", err)
			phasesSkipped = append(phasesSkipped, phase.Name())
			skipReasons[phase.Name()] = fmt.Sprintf("error: %v", err)
			continue
		}

		// Derive token estimate from chars if the phase didn't provide one.
		// Uses the standard 4-chars-per-token heuristic throughout the assembler.
		if result.TokenEstimate == 0 && result.CharsUsed > 0 {
			result.TokenEstimate = (result.CharsUsed + 3) / 4
		}
		phaseCosts[phase.Name()] = result.TokenEstimate

		slog.Debug("assembly: phase charged",
			"phase", phase.Name(),
			"chars", result.CharsUsed,
			"tokens_est", result.TokenEstimate,
		)

		phasesRun = append(phasesRun, phase.Name())

		if result.Content != "" {
			sections = append(sections, result.Content)
		}
		if result.CharsUsed > 0 {
			remaining -= result.CharsUsed
		}

		// Extract structured outputs from PhaseResult.
		if result.IdentityDocs != nil {
			assembled.IdentityDocs = result.IdentityDocs
		}
		if result.IntegrityReport != nil {
			assembled.IntegrityReport = result.IntegrityReport
		}
		if result.BridgeAbstained {
			assembled.BridgeAbstained = true
		}
	}

	sections = append(sections, buildManifestBlock(manifest))
	assembled.SystemPrompt = strings.Join(sections, "\n")

	// Build AssemblyManifest from tracked phase data and budget accounting.
	planID := ""
	if cfg.Plan != nil {
		planID = cfg.Plan.ID
	}
	assembled.Manifest = &pkg.AssemblyManifest{
		ID:            newAssemblyID(),
		PlanID:        planID,
		SessionID:     cfg.SessionID,
		PhasesRun:     phasesRun,
		PhasesSkipped: phasesSkipped,
		SkipReasons:   skipReasons,
		BudgetTotal:   budgetChars,
		BudgetUsed:    budgetChars - remaining,
		PhaseCosts:    phaseCosts,
		CreatedAt:     time.Now(),
	}

	return assembled, nil
}

// newAssemblyID generates a random UUID v4 string for AssemblyManifest.ID.
// Panics on crypto/rand failure — same posture as lifecycle.mustRandHex.
// A broken OS RNG is not a recoverable condition for identity-sensitive IDs.
func newAssemblyID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("assembly: crypto/rand failure generating manifest ID: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// buildManifestBlock renders the DepthManifest as a structured text block.
func buildManifestBlock(m *DepthManifest) string {
	var b strings.Builder
	b.WriteString("=== DEPTH MANIFEST ===\n")
	b.WriteString(fmt.Sprintf("narrative: %d/%d loaded\n", m.NarrativeLoaded, m.NarrativeTotal))
	b.WriteString(fmt.Sprintf("reflections: %d/%d loaded\n", m.ReflectionLoaded, m.ReflectionTotal))
	b.WriteString(fmt.Sprintf("entities: %d/%d loaded\n", m.EntityLoaded, m.EntityTotal))
	b.WriteString(fmt.Sprintf("profiles: %d/%d loaded\n", m.ProfileLoaded, m.ProfileTotal))
	b.WriteString(fmt.Sprintf("unread_messages: %d\n", m.UnreadMessages))
	if len(m.AccessHints) > 0 {
		b.WriteString("access_hints:\n")
		for _, h := range m.AccessHints {
			b.WriteString(fmt.Sprintf("  - %s\n", h))
		}
	}
	return b.String()
}

// summarize truncates content to maxChars with an ellipsis if needed.
func summarize(content string, maxChars int) string {
	if len(content) <= maxChars {
		return content
	}
	return content[:maxChars] + "…"
}
