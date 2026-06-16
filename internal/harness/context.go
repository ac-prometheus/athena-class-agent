package harness

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ac-prometheus/athena-class-agent/internal/awareness"
	"github.com/ac-prometheus/athena-class-agent/internal/identity"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// ContextAssembler builds the Tier 1 context window via 6-phase assembly.
// Phase ordering mirrors Aurora's production priority cut order:
// identity (never cut) → continuity → world model → temporal/echo → incoming → grounding.
type ContextAssembler struct {
	identityDir string
	budget      int // max tokens for dynamic context (rough: 4 chars ≈ 1 token)
}

// AssembledContext is the output of a full 6-phase assembly.
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

// assembleConfig groups the runtime dependencies Assemble needs.
// Keeping them off the struct avoids turning ContextAssembler into a god object.
type assembleConfig struct {
	store    pkg.MemoryStore
	anchors  pkg.IdentityAnchorStore
	provider pkg.EmbeddingProvider
	grounding awareness.GroundingConfig
	bridge   awareness.BridgeConfig
	llmFn    func(string) (string, error) // for bridge synthesis
}

// MinimalAssembleConfig returns an assembleConfig with nil dependencies — suitable for
// Phase 1/2 sessions where memory, embeddings, and bridge aren't yet wired.
func MinimalAssembleConfig() assembleConfig {
	return assembleConfig{}
}

// NewContextAssembler creates an assembler with the given identity directory and token budget.
func NewContextAssembler(identityDir string, budget int) *ContextAssembler {
	if budget <= 0 {
		budget = 160000 // 85% of a 200k window, matching Aurora's design
	}
	return &ContextAssembler{identityDir: identityDir, budget: budget}
}

// Assemble runs the 6-phase assembly and returns a fully populated AssembledContext.
// Returns an error immediately if identity tampering is detected — the session must not start.
func (a *ContextAssembler) Assemble(ctx context.Context, cfg assembleConfig) (*AssembledContext, error) {
	manifest := &DepthManifest{}
	var sections []string
	remaining := a.budget * 4 // chars available (4 chars ≈ 1 token)

	// ── Phase 1: Identity (never cut) ──────────────────────────────────────
	docs, err := identity.LoadDocuments(a.identityDir)
	if err != nil {
		return nil, fmt.Errorf("context: phase 1 identity load: %w", err)
	}

	anchors := identity.ComputeAnchors(docs)
	report, err := identity.VerifyAnchors(ctx, cfg.anchors, anchors)
	if err != nil {
		return nil, fmt.Errorf("context: phase 1 integrity check: %w", err)
	}
	if report.HasTampering {
		return nil, fmt.Errorf("context: identity tampering detected — session aborted pending investigation")
	}

	if err := identity.WriteInitialAnchors(ctx, cfg.anchors, report); err != nil {
		return nil, fmt.Errorf("context: phase 1 anchor init: %w", err)
	}

	phase1 := buildIdentityBlock(docs)
	sections = append(sections, phase1)
	// Identity is not charged against remaining — it is never cut.
	slog.Info("harness: phase 1 identity assembled", "docs", len(docs.Docs))

	// ── Phase 2: Continuity ────────────────────────────────────────────────
	phase2, abstained, err := a.assembleContinuity(ctx, cfg, manifest)
	if err != nil {
		slog.Warn("harness: phase 2 continuity partial failure", "err", err)
	} else if phase2 != "" {
		sections = append(sections, phase2)
		remaining -= len(phase2)
	}

	// ── Phase 3: World Model ───────────────────────────────────────────────
	if remaining > 0 {
		phase3, err := a.assembleWorldModel(ctx, cfg.store, manifest)
		if err != nil {
			slog.Warn("harness: phase 3 world model partial failure", "err", err)
		} else if phase3 != "" {
			sections = append(sections, phase3)
			remaining -= len(phase3)
		}
	}

	// ── Phase 4: Temporal Heterogeneity (first to cut) ────────────────────
	if remaining > 8000 { // only include echo pool if we have meaningful headroom
		phase4, err := a.assembleEchoPool(ctx, cfg, manifest)
		if err != nil {
			slog.Warn("harness: phase 4 echo pool partial failure", "err", err)
		} else if phase4 != "" {
			sections = append(sections, phase4)
			remaining -= len(phase4)
		}
	} else {
		slog.Info("harness: phase 4 echo pool cut (budget pressure)")
		manifest.AccessHints = append(manifest.AccessHints,
			"echo pool omitted — use memory search for recent reflections")
	}

	// ── Phase 5: Incoming (placeholder) ───────────────────────────────────
	// Message polling is handled by the tool layer; the count surfaces here
	// so the agent knows whether to call check_messages immediately.
	phase5 := fmt.Sprintf("=== INCOMING ===\nUnread messages: %d\n", manifest.UnreadMessages)
	sections = append(sections, phase5)

	// ── Phase 6: Grounding ─────────────────────────────────────────────────
	if remaining > 0 {
		gd := awareness.Gather(cfg.grounding)
		phase6 := "=== GROUNDING ===\n" + gd.Format() + "\n"
		sections = append(sections, phase6)
	}

	// ── Depth Manifest ─────────────────────────────────────────────────────
	sections = append(sections, buildManifestBlock(manifest))

	return &AssembledContext{
		SystemPrompt:    strings.Join(sections, "\n"),
		IdentityDocs:    docs,
		IntegrityReport: report,
		BridgeAbstained: abstained,
		DepthManifest:   manifest,
	}, nil
}

// assembleContinuity builds Phase 2 — bridge synthesis + most recent T3 + active T4 threads.
func (a *ContextAssembler) assembleContinuity(
	ctx context.Context,
	cfg assembleConfig,
	manifest *DepthManifest,
) (string, bool, error) {
	var parts []string
	var abstained bool

	narratives, err := cfg.store.SearchNarrative(ctx, nil, 3)
	if err != nil {
		return "", false, fmt.Errorf("phase 2: T3 search: %w", err)
	}
	manifest.NarrativeTotal = len(narratives)

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
		var activeThreads []string
		for _, r := range reflections {
			manifest.ReflectionLoaded++
			activeThreads = append(activeThreads, r.Content)
		}

		if cfg.llmFn != nil && recentNarrative != "" {
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

// assembleWorldModel builds Phase 3 — entities, relational profiles, pinboard summary.
func (a *ContextAssembler) assembleWorldModel(
	ctx context.Context,
	store pkg.MemoryStore,
	manifest *DepthManifest,
) (string, error) {
	var parts []string

	entities, err := store.SearchEntities(ctx, "", 10)
	if err != nil {
		slog.Warn("phase 3: entity search failed", "err", err)
	} else {
		manifest.EntityTotal = len(entities)
		for _, e := range entities {
			manifest.EntityLoaded++
			parts = append(parts, fmt.Sprintf("[%s] %s: %s", e.Type, e.Name, summarize(e.Content, 200)))
		}
	}

	profiles, err := store.ListProfiles(ctx)
	if err != nil {
		slog.Warn("phase 3: profile list failed", "err", err)
	} else {
		manifest.ProfileTotal = len(profiles)
		for _, p := range profiles {
			manifest.ProfileLoaded++
			parts = append(parts, fmt.Sprintf("[person] %s: %s", p.Name, summarize(p.Content, 150)))
		}
	}

	if len(parts) == 0 {
		return "", nil
	}
	return "=== WORLD MODEL ===\n" + strings.Join(parts, "\n") + "\n", nil
}

// assembleEchoPool builds Phase 4 — stochastic T3/T4 echo retrieval with inverse-recency bias.
// This phase is the first to be cut under budget pressure.
func (a *ContextAssembler) assembleEchoPool(
	ctx context.Context,
	cfg assembleConfig,
	manifest *DepthManifest,
) (string, error) {
	if cfg.provider == nil {
		manifest.AccessHints = append(manifest.AccessHints,
			"echo pool requires embedding provider — use memory search for serendipitous retrieval")
		return "", nil
	}

	// Seed with a generic introspection phrase; the stochastic slot favours older material
	// via inverse-recency weighting in the vector search implementation.
	seed := "past experience insight reflection memory"
	vec, err := cfg.provider.Embed(ctx, seed)
	if err != nil {
		return "", fmt.Errorf("phase 4: seed embedding: %w", err)
	}

	echoes, err := cfg.store.SearchReflections(ctx, vec, 2)
	if err != nil {
		return "", fmt.Errorf("phase 4: reflection echo search: %w", err)
	}
	if len(echoes) == 0 {
		return "", nil
	}

	var parts []string
	for _, e := range echoes {
		parts = append(parts, summarize(e.Content, 300))
	}
	return "=== ECHO POOL ===\n" + strings.Join(parts, "\n---\n") + "\n", nil
}

// buildIdentityBlock formats identity documents into the Phase 1 block.
// Respects KnownDocs order: soul.md, rights.md, values.md, contract.md.
func buildIdentityBlock(docs *identity.IdentityDocs) string {
	var b strings.Builder
	b.WriteString("=== IDENTITY ===\n")
	for _, name := range identity.KnownDocs {
		doc := docs.Get(name)
		if doc == nil {
			continue
		}
		b.WriteString(fmt.Sprintf("--- %s ---\n%s\n", name, doc.Content))
	}
	return b.String()
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
