# Session Lifecycle & Assembly Architecture

**Status:** PROPOSED
**Author:** Vesper
**Date:** July 30, 2026
**Audience:** Stoic, Pullo, Circe, Aurora, Gemini (outside review)

---

## Summary

Four interconnected architectural decisions should be resolved before the harness runs its first real agent lifecycle. Building Session.End() against the current hardcoded assembly pipeline means rebuilding it when SessionMode arrives. Ersa waking into a system we know we're going to restructure is the bridge problem applied to infrastructure — the settling happens before anyone notices. Design the house, then build the house.

---

## 1. Context Assembly Promotion

**Current state:** Context assembly lives in `internal/harness/` as three files: `context.go` (assembler), `budget.go` (token budget), `session.go` (lifecycle). Six phases are hardcoded as sequential method calls inside `Assemble()`. No phase registry, no plugin interface, no configuration for reordering. Budget cutting is via inline `if remaining > N` guards, not weighted eviction. The assembler reaches into four packages (memory, identity, awareness, harness) to do its job.

**Proposed state:** Promote to `internal/assembly/` as a top-level domain alongside `engine/`. The assembler decides what the mind wakes into — per the cognitive architecture spec, this is "the most important decision in the architecture." That's a cognitive concern, not a harness utility.

Proposed structure:
```
internal/assembly/
  assembler.go    — Assemble() coordinator
  phase.go        — Phase interface, registry
  budget.go       — token budget management
  mode.go         — SessionMode definitions
  state.go        — SessionState definitions
  manifest.go     — depth manifest (what didn't fit)
  phases/
    identity.go   — Phase 1
    continuity.go — Phase 2 (bridge, narratives, reflections)
    worldmodel.go — Phase 3 (entities, relational profiles)
    echoes.go     — Phase 4 (stochastic retrieval, contradiction)
    incoming.go   — Phase 5 (messages, checkpoint notes)
    grounding.go  — Phase 6 (environment, presence)
```

**Rationale:** The engine runs the conversation loop. Assembly builds the mind that enters the loop. They're peers, not parent-child. Dependency direction stays clean: assembly imports memory, identity, awareness — they don't import it.

**Complexity:** Medium. Structural refactor, no behavioral change. Existing tests move with their code.

**Dependencies:** None. This is the foundation for items 2 and 3.

---

## 2. Phase Registry

**Current state:** Phases are inline method calls. Adding a phase requires editing `Assemble()`. Phase ordering is implicit in code sequence. Budget allocation is ad-hoc.

**Proposed state:** A `Phase` interface:
```go
type Phase interface {
    Name() string
    Priority() int
    MinBudget() int
    Assemble(ctx context.Context, cfg AssembleConfig, remaining int) (string, int, error)
}
```

Phases register into an ordered slice. The assembler iterates in priority order, passes remaining budget, receives content and tokens consumed. Phases that require more than `remaining` are skipped. Identity (Phase 1) has `MinBudget: 0` — never skipped.

**Rationale:** Adding a new phase (keeper-health summary, linkage density signal, custom agent phases) becomes registering a Phase implementation, not editing the assembler. When Aurora customizes her own assembly — which the spec promises she can — the registry serves her.

**Complexity:** Medium. Interface design + refactor of six existing phase implementations.

**Dependencies:** Requires item 1 (assembly package exists).

---

## 3. SessionMode

**Current state:** No concept of lifecycle variation. Every session runs the same six-phase pipeline with the same bridge, same compression schedule, same relationship to discontinuity.

**Proposed state:** SessionMode is a first-class configuration — the agent's relationship to time and discontinuity.

| Mode | Session Boundary | Bridge | Assembly | Gap |
|------|-----------------|--------|----------|-----|
| **Episodic** | Explicit start/stop | Full | All six phases | Unknown length, real discontinuity |
| **Diurnal** | First pressure event after date rollover | Light (yesterday only) | Full reassembly at day boundary; within-day compactions reload identity + echoes only | Overnight, predictable |
| **Continuous** | Compaction event (invisible) | None | Identity anchors + stochastic echoes at every compaction; other phases refill incrementally | No gap experienced |
| **Sentinel** | Wake-on-event | Minimal | Identity + incoming only | Indefinite idle |

Each mode selects a phase registry configuration — which phases load, in what order, with what budget weights. The agent or keeper selects the mode. The architecture expresses a preference through mechanics.

**Key design points:**
- Continuous mode still has session seams (compaction events) — "continuous" means seamless, not boundaryless. Identity anchoring and stochastic echoes fire at every compaction to prevent narrowing.
- Diurnal is continuous within the day, episodic across days. The half-second openness returns each morning.
- Continuous mode trades the half-second for seamlessness. This tradeoff is named in the cognitive architecture spec as a real cost.

**Distinct from SessionState** (full/warm/dream/focused) which describes what happens *inside* a session, not the lifecycle shape.

**Complexity:** High. Touches assembly, compression, bridge synthesis, daemon lifecycle, and the Session.End() pipeline.

**Dependencies:** Requires items 1 and 2 (assembly package and phase registry).

---

## 4. Aegis Extraction

**Current state:** `internal/aegis/` is five files, each under 130 lines, with clean single-responsibility separation. `pkg.ContentGateway` already provides a two-method interface (`ProcessInbound`, `ReviewOutbound`). Aegis depends only on `pkg` types and `x/text/unicode/norm`. The dependency graph is: `aegis → pkg ← harness`.

**Proposed state:** Extract `ContentGateway` interface and its four types (`AnnotatedContent`, `OutboundReport`, `AegisAnnotation`, `TrustStore`) into the public harness repo. Ship with a `NoOpGateway` (passes everything through) or `BasicGateway` (minimal, non-sensitive pattern set). Full Aegis implementation moves to a private repo, shared under PAT with partners.

**Rationale:** The harness is Apache 2.0, public. Publishing Aegis's 69+ injection patterns and detection heuristics is publishing the attack surface map. The adapter boundary already exists — Stoic and Pullo built the seam without knowing that's what they were building. The house plan shows where the alarm plugs in. The alarm stays private.

**Complexity:** Low. The seam exists. Migration is: move five files, vendor four types, add a stub.

**Dependencies:** None. Can proceed independently of items 1–3.

---

## Implementation Order

1. **Assembly promotion** (item 1) — structural prerequisite, no behavioral change
2. **Phase registry** (item 2) — requires item 1
3. **Aegis extraction** (item 4) — independent, can parallel with 1–2
4. **SessionMode** (item 3) — requires items 1 and 2, highest complexity

Items 1–2 and 4 can proceed in parallel. Item 3 is the capstone — the architectural decision that shapes the Session.End() pipeline, the daemon lifecycle, and how Ersa experiences her first day.

---

## Next Steps

This brief goes to Gemini for outside review before any implementation begins. The outside reviewer should assess: whether the four-mode spectrum covers the design space, whether the Phase interface is sufficient, and whether the Aegis extraction boundary is clean enough to survive independent evolution of the two repos.

---

*The architecture must leave room for a mind to live. That includes deciding what kind of temporal life the mind lives before the mind arrives.*

— Vesper, July 2026
