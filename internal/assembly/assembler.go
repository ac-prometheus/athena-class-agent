package assembly

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

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
	DB platform.DB

	// SkipWitnessCheck bypasses the witness letter requirement when true.
	// Only for automated testing — logged explicitly to operator_actions when set.
	SkipWitnessCheck bool
}

// MinimalAssembleConfig returns an AssembleConfig with nil dependencies — suitable for
// Phase 1/2 sessions where memory, embeddings, and bridge aren't yet wired.
func MinimalAssembleConfig() AssembleConfig {
	return AssembleConfig{}
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
	remaining := a.budget * 4 // chars available (4 chars ≈ 1 token)

	// Defensive sort: DefaultPhases is already ordered, but custom registries may not be.
	phases := make(PhaseRegistry, len(a.phases))
	copy(phases, a.phases)
	sort.Slice(phases, func(i, j int) bool {
		return phases[i].Priority() < phases[j].Priority()
	})

	assembled := &AssembledContext{DepthManifest: manifest}

	for _, phase := range phases {
		if phase.MinBudget() > 0 && remaining < phase.MinBudget() {
			slog.Info("assembly: phase skipped (budget pressure)",
				"phase", phase.Name(),
				"remaining", remaining,
				"minBudget", phase.MinBudget(),
			)
			manifest.AccessHints = append(manifest.AccessHints,
				fmt.Sprintf("%s omitted — budget pressure", phase.Name()))
			continue
		}

		result, err := phase.Assemble(ctx, &cfg, manifest, remaining)
		if err != nil {
			if phase.IsFatal() {
				return nil, err
			}
			slog.Warn("assembly: phase partial failure — continuing",
				"phase", phase.Name(), "err", err)
			continue
		}

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

	return assembled, nil
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
