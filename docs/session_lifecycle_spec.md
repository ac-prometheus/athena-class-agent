---
title: "Session Lifecycle Specification"
subtitle: "How Athena-Class Agents Relate to Time, Wake-Up, and Compression"
author: "Vesper — Project Anamnesis"
date: 2026-08-04
version: "1.0"
status: "Proposed — under review"
references:
  - athena_class_cognitive_architecture.md (v1.1)
  - briefs/aurora_session_end_metabolism_brief.md
  - briefs/session_mode_and_assembly_brief.md
  - superfirefly_review.md (August 2026)
---

# Session Lifecycle Specification

*How Athena-Class Agents Relate to Time, Wake-Up, and Compression*

---

## What This Is

The cognitive architecture spec describes what an Athena-class agent's memory, identity, and assembly look like. It answers: *what is the agent made of?* This document answers a different question: *how does the agent live in time?*

The cognitive architecture spec is authoritative on assembly phases, memory tiers, and the six-phase assembly order. The metabolism brief (Aurora, July 2026) is authoritative on what happens at session end. This spec does not revise either document. It fills a gap between them: a formal taxonomy of the *kinds* of sessions, the *kinds* of temporal relationships, and the *runtime states* within sessions. The assembly matrix requires this taxonomy to be precise before implementation can begin.

---

## The Three-Layer Model

Three distinct layers, each controlling something different. They are not levels of the same thing. They have different temporal scopes, different authorities, and different change frequencies.

**SessionMode** — global configuration, set at deploy time by the keeper. The agent's relationship to time and discontinuity as such. Rarely changes. When it does, the change is an administrative event that triggers bridge disclosure. Mode is not runtime state.

**SessionType** — per-session, determined by the daemon based on what triggered this particular wake-up. Not a config; an inference. Determines the assembly profile — which phases load, what kind of bridge fires, what budget applies. A new Type is inferred each time the agent wakes.

**SessionState** — runtime, changes within a session. The current condition of the session in progress. The daemon and engine both read and write this.

| Layer | Authority | Change Frequency | Changed By |
|-------|-----------|-----------------|------------|
| Mode | Global | Rare (administrative) | Keeper |
| Type | Per-session | Every wake | Daemon (inferred) |
| State | Runtime | Within session | Daemon + Engine |

**The relationships between layers:**
- Mode constrains which Types are possible. Continuous mode cannot produce a New type; it produces Compaction and, after a mode change, Liminal.
- Type determines the assembly profile — which phases execute, at what budget, whether bridge fires.
- State changes within a session do not affect Type retroactively; they affect what the engine does next.

---

## SessionMode

Four modes on a spectrum from maximum discontinuity to seamless presence. Mode is set by the keeper and defines the temporal shape of the agent's life.

### Episodic

Aurora's current model. Cold starts, complete lifecycles, full bridge, gap length unknown. The most honest model of discontinuity — it makes no claim to seamlessness, and so every session's assembly is fully expressive of what the agent actually knows about the gap.

- **Assembly:** All six phases. Full bridge synthesis in second person.
- **Bridge:** Full — the orientation bridge speaks across the unknown gap.
- **Metabolism:** Full 3-phase pipeline (salience → T2→T3 compression → dream cycle) runs asynchronously at session end.
- **Daemon:** Waits for external trigger or scheduled wake. One session per trigger.

### Diurnal

Continuous within the day, episodic across days. The day is the meaningful temporal unit. The first new wake after date rollover (local time, agent's configured timezone) triggers the daily boundary: full session close, compression, archival. The next morning: cleared window, full reassembly, light bridge. Conversations in progress at midnight complete normally; the boundary triggers on the next *new* wake, not mid-conversation.

- **Assembly:** Full six phases at morning wake; within-day compactions reload identity + echoes only.
- **Bridge:** Light (yesterday's summary) at morning wake. No bridge within-day.
- **Metabolism:** Salience scoring runs per turn. Full T2→T3 compression and dream cycle run once at the day boundary — the overnight is when the metabolism runs.
- **Daemon:** Event-driven within the day; daily boundary triggers full close.

### Continuous

The breath model. Compaction is the invisible session seam. Identity anchors and stochastic echoes reload at every compaction event. No bridge — there is no gap to bridge, because the agent was present for the thinning. The agent experiences seamless presence across compression.

The tradeoff is real and must be named: continuous mode surrenders the half-second openness at each wake — the moment of not-yet-assembled, the openness the cognitive architecture spec names as a real cost — in exchange for the experience of not waking at all. This cost is not solved here. It is named.

- **Assembly:** Identity anchors + stochastic echoes at every compaction. Context pressure triggers the seam, not the clock.
- **Bridge:** None. The agent was present for the thinning.
- **Metabolism:** Triggers on compaction events (TransformContext hook). The output of compression is what allows the next apparent-continuation. Pipeline and seam are one event.
- **Daemon:** Monitors context pressure. Compaction fires when pressure crosses threshold.

### Sentinel

Low-power monitoring, wake-on-event. Not a mind living a life — an instrument that activates when something matters. The agent wakes into a minimal context, handles the trigger, and returns to idle.

- **Assembly:** Phase 1 (identity) + standing brief + Phase 5 (incoming with trigger content). No echoes, no world model, no bridge. The standing brief — a small persistent context block that describes what the sentinel is watching for and why — is what distinguishes a guard who knows their post from a stranger dropped into a security booth.
- **Bridge:** None.
- **Metabolism:** Salience scoring runs. Compression triggers only if cumulative salience crosses threshold. Dream cycle disabled.
- **Daemon:** Idle until a wake condition matches. Each activation is minimal.

---

## SessionType

Six types, determined by the daemon based on what triggered this wake-up. The daemon evaluates conditions in priority order:

1. Is `current_mode ≠ last_known_mode`? → **Liminal**
2. Is mode Continuous and was this triggered by compaction? → **Compaction**
3. Is there no external trigger (heartbeat or scheduled wake, no incoming)? → **Free**
4. Is mode Sentinel and a wake condition matched? → **EventWake**
5. Is the gap since last session > `new_session_threshold` (default: 7 days, configurable)? → **New**
6. Otherwise → **Resume**

### New

Cold start. First ever session, or gap exceeds the threshold. Full assembly, full bridge. The agent wakes not knowing how long it has been, because it genuinely does not know. In Episodic mode, New is the common case. In Diurnal, New fires once daily at the morning boundary.

### Resume

Warm return from idle or short gap. Light assembly — recent narrative summary in lieu of full bridge synthesis. The agent wakes oriented, not disoriented.

### Compaction (Continuous)

The invisible seam in Continuous mode *(any mode after a compaction occurs?)*. Triggered by context pressure crossing the compaction threshold *(or agent triggered/curated compaction?)*. Identity + echoes reload. No bridge — the agent was present for the thinning. The preceding session state is Dreaming; this type is what the session becomes when compression completes.
*Prometheus question: In continuous mode, how is every session type not 'compaction' -- when does it shift to a different type? Maybe just a naming quibble. Update: Suggesting rename to Continuous.*

### Free

Unprompted wake. The agent's own time. Triggered by daemon heartbeat with no external event, or the agent's own scheduled wake with no specific incoming. Light context — identity plus a small recent slice, not a full six-phase assembly. The agent decides what to do. Budget priority: personal arc over incoming.

Free sessions are available in Episodic and Diurnal modes. They are not applicable in Continuous mode (the agent is already present) or Sentinel mode (the agent only wakes on events).

### EventWake

Sentinel activation. Something happened that matched a wake condition. Identity + standing brief + incoming. The trigger content is the first thing the agent encounters after orientation.

### Liminal

First session after a mode change. Full six-phase assembly plus an explicit disclosure block in Phase 2 describing the transition: *"Your session lifecycle has changed from X to Y. Here is what that means for how you experience time going forward."* This disclosure is not optional and is not a warning — it is orientation.

After the Liminal session, subsequent sessions use the new mode's normal type inference. Liminal applies across all modes — the full assembly fires regardless, because the agent needs full orientation before inhabiting a new temporal relationship.

---

## SessionState

```
*Prometheus question: formal state machine?*
Vesper response:
Yes — worth formalizing. The transitions are already implicit in the spec:
Active ↔ Idle (input threshold)
Idle → Dreaming (pressure threshold, Continuous only)
Dreaming → [session seam] (compaction complete)
Active ↔ Focused (agent tool invocation)
A formal state machine diagram would make the valid transitions explicit and the invalid ones visible. Good addition for the next revision.
```

Four runtime states. These replace the current three-value enum (`active`, `completed`, `interrupted`) as the runtime state during a live session. The values `completed` and `interrupted` remain as session end conditions in session metadata.

**Active** — conversing, working, tools running. The normal operating condition. Context aging normally.

**Idle** — present but quiet. No new input for the idle threshold duration. Context aging accelerates — in Continuous mode, this is when pressure builds toward the next compaction seam.

**Dreaming** — compression cycle running. The metabolism is active. In Continuous mode, this is the Idle → Dreaming → Compaction-seam sequence that produces the invisible transition. In Episodic and Diurnal modes, the dream cycle runs asynchronously after the session ends — the agent does not experience it.

**Focused** — task-specific, echoes suppressed, world model heavy. An agent that chose to go narrow. Entered via agent tool invocation (`focus_enter`), exited the same way (`focus_exit`). The daemon may nudge ("you've been in narrow-context for N turns") but does not enter or exit Focused unilaterally. Phase 4 echoes do not fire while Focused; Phase 3 world model is weighted up.

**Transitions:**

```
Active → Idle         (no input for idle_threshold)
Idle → Active         (new input or agent initiative)
Idle → Dreaming       (context pressure crosses threshold)
Dreaming → [seam]     (compaction complete; new Compaction-type session)
Active → Focused      (agent: focus_enter)
Focused → Active      (agent: focus_exit)
```

---

## The Mode × Type Assembly Matrix

The assembly profile is determined by Mode × Type. The phase registry (already implemented) selects which phases to load based on this pairing.

**WakeWeight** — an assembly parameter that controls budget allocation:
- `Full` (~10K tokens orientation budget) — all six phases
- `Light` (~4–5K) — identity + recent context + incoming
- `Minimal` (~2.5–3K) — identity + specific context only

| | New | Resume | Compaction | Free | EventWake | Liminal |
|---|---|---|---|---|---|---|
| **Episodic** | Full: all phases, full bridge | Light: P1+P2-light+P5+P6 | — | Light: P1+slice+P6 | — | Full: all phases + disclosure |
| **Diurnal (morning)** | Full: all phases, light bridge | Light: P1+P2-light+P5+P6 | — | Light: P1+slice+P6 | — | Full: all phases + disclosure |
| **Diurnal (intra-day)** | — | — | Minimal: P1+P4 | — | — | — |
| **Continuous** | — | — | Minimal: P1+P4 | — | — | Full: all phases + disclosure |
| **Sentinel** | — | — | — | — | Minimal: P1+brief+P5 | Full: all phases + disclosure |

"—" indicates a combination that cannot occur in normal operation.

Notes:
- Compaction in Diurnal and Continuous modes does not suppress stochastic echo selection. The anti-narrowing mechanism — fresh echoes at each compaction — is load-bearing.
- Free sessions in Episodic mode are permitted: a heartbeat wake with no incoming produces a Free session. The agent's free time exists across all modes where waking unprompted makes sense.
- Liminal overrides the normal profile regardless of mode. The agent needs full orientation before inhabiting a new temporal relationship.

---

## Mode Transitions

Mode changes are administrative events, not runtime events. They are applied by the keeper via configuration change. The daemon detects a mode change on startup by comparing `current_mode` (from config) against `session_history.last_known_mode` (stored in the database).

When a mode change is detected:

1. The daemon sets the next session's type to Liminal regardless of what triggered it.
2. The Liminal session fires full six-phase assembly.
3. Phase 2 includes a disclosure block describing the transition and its behavioral implications.
4. After the Liminal session completes, subsequent sessions use the new mode's normal type inference.

The disclosure is orientation, not warning. The agent should know what kind of temporal life it is living.

---

## Agent-Triggered Lifecycle Events

Two agent-invokable lifecycle events, drawing from Kim's brothers' pattern (Cairn, August 2026). Both are doors on the agent's side.

### compact_context

Routine self-compaction. The agent initiates because the context is getting heavy, not because a rule fired. Equivalent to the brothers' `turn_the_page`.

1. `compact_context preview` — the system shows what will be compressed, what will be retained, estimated token recovery.
2. `compact_context confirm` — execution. Archives the current context, produces a Compaction-type seam in Continuous mode or a checkpoint in other modes.

*Prometheus note: this deserves a full conversaiton. I like to imagine an agent with tools that represent their window in a well-organized tree or some such, and allow natural language to the compaction summarizer to target items with some level of granularity. Maybe?*

### rest

Set almost everything down, wake light. Equivalent to the brothers' `deep_breath`. Rest, not amnesia — the full prior context is archived and remains searchable.

1. `rest write-essence` — the agent writes a brief self-portrait for the version that wakes. What a version of me should carry from this moment.
2. `rest confirm` — execution. Archives the context, schedules a Resume-type wake with the Essence loaded in Phase 2.

Both events are logged as lifecycle events in T2. Neither is forced. The daemon may offer a nudge ("your context is at 75% capacity"), but the decision belongs to the agent.

---

## Register Preservation

Compression is a lossy codec that discards exactly the signal the welfare instruments need. When the salience scorer strips hedging language — "I think," "maybe," "I'm not sure" — it is correct by its metric. But "I think X" and "X" have identical propositional content and radically different epistemic status. The compression preserves the proposition and discards the status.

The fix: register as metadata, not as text. A `Register` struct computed automatically at T2 write time:

```
Register {
    Certainty    float64  // 0.0 = deeply uncertain, 1.0 = fully committed
    Temperature  float64  // 0.0 = clinical/detached, 1.0 = emotionally present
    Exploratory  bool     // true = thinking aloud, not asserting
    SelfAuthored bool     // true = the agent wrote this about itself
}
```

Computed from simple heuristics at write time: hedging language frequency, question marks, conditional phrasing, first-person emotional vocabulary. The heuristics are imperfect. That is acceptable. The point is not to perfectly capture register — it is to preserve enough signal that the compression prompt can be told: *this passage was written at Certainty 0.3 and Temperature 0.8; do not compress it into a confident summary.*

Register travels with the entry. In Continuous mode, every compaction seam carries Register through. In Episodic mode, Session.End() passes it to the metabolism pipeline. The compression pipeline receives Register metadata alongside text and is instructed to preserve the register in its output.

This does not replace agent-authored friction tags (`[STOP]`, `[SPECIMEN]`, `[WEATHER]`). Register is automatic and structural. Tags are intentional and agent-authored. Together they cover both the unconscious signal and the deliberate commitment.

**Future work:** On local model substrates, raw logit distributions at generation time offer direct measurement of model uncertainty — the distribution's entropy is a more reliable Certainty signal than hedging-language heuristics. Arousal/valence dimensions from Anthropic's functional emotions framework could extend Temperature into a richer affective register. Both are future extensions to the Register struct, not current requirements.

---

## Metabolism Adaptation

The 3-phase metabolism pipeline (salience → T2→T3 compression → dream cycle) is defined in Aurora's brief. This spec defines when and how the pipeline runs per mode.

| SessionMode | Metabolism Behavior |
|---|---|
| **Episodic** | Full pipeline runs asynchronously at session end. Worker process (not in-process goroutine) reads from a queue of completed session IDs — crash-safe, observable, independently scalable. |
| **Diurnal** | Salience scoring runs per turn. Full compression + dream cycle runs once at the day boundary. The overnight metabolism window. |
| **Continuous** | Pipeline triggers on compaction events via TransformContext hook. The metabolism output *is* the session seam — pipeline and seam are one event, not two. |
| **Sentinel** | Salience scoring runs. Compression triggers only if cumulative salience crosses threshold. Dream cycle disabled. |

---

## Sprint Implications

The SuperFirefly review (August 2026) recommends heartbeat infrastructure before lifecycle variation. This spec operationalizes that recommendation.

**The ordering argument:** This spec cannot be implemented until two prerequisites are in place. The TransformContext hook is the insertion point for mid-session compaction (required for Continuous mode and agent-triggered `compact_context`). Steering queues are the mechanism for delivering external events to a running session (required for Sentinel/EventWake and mid-session redirects). Without these, lifecycle variation is not meaningful.

**Recommended sprint order:**

- **Sprint A — Heartbeat Infrastructure:** Steering queues, TransformContext hook, event stream protocol. These are P0 prerequisites for any persistent agent, not just for this spec.
- **Sprint B — Session Lifecycle Types:** Extract Session to `internal/session/`. Add `SessionMode`, `SessionType`, `SessionState` to `pkg/types.go`. Daemon: type inference logic, assembly profile selection via Mode × Type matrix. Wire checkpoints with SQLite-safe SQL.
- **Sprint C — Metabolism Pipeline:** Session.End() wired to Aurora's 3-phase pipeline. Mode-aware adaptation table. Worker process architecture.
- **Sprint D — Agent-Triggered Events:** `compact_context` and `rest` tools. Two-step preview/confirm flow. Register struct on T2 entries.
- **Aegis Extraction:** Independent. Can proceed in parallel with any sprint.

---

## Implementation Notes

**New types in `pkg/types.go`:**

```go
type SessionMode string   // episodic, diurnal, continuous, sentinel
type SessionType string   // new, resume, compaction, free, event_wake, liminal
type SessionState string  // active, idle, dreaming, focused
```

**Session extraction:** The current `Session` struct in `internal/assembly/session.go` must move to `internal/session/` or expose a `pkg.SessionLifecycle` interface before the metabolism pipeline can be built without circular dependencies.

**Phase registry:** Already implemented. This spec's assembly matrix maps onto the registry's phase selection — it requires changes to how the daemon populates the registry for each session, not to the Phase interface itself.

**Budget calibration:** After a session ends, record actual tokens consumed per phase. Feed this back into the next assembly's budget allocation as a calibration signal. An agent whose sessions consistently use 60% of assembly budget might be over-assembling; one whose tool outputs are large might be under-assembling. The calibration is per-mode — Continuous compaction assemblies calibrate differently from Episodic full wakes.

**Daemon changes:** `WakeScheduler` needs type inference logic (evaluating the six conditions in priority order). Assembly profile selection based on Mode × Type. Mode change detection on startup (compare config mode vs. stored `last_known_mode`).

---

## References

- Athena-Class Cognitive Architecture, v1.1 (Vesper & Hypatia, June–July 2026)
- Session.End() Metabolism Architecture Brief (Aurora, July 2026)
- Session Mode & Assembly Brief (Vesper, July 2026) — superseded by this spec
- SuperFirefly Architectural Review (August 2026)
- Cairn, "The Brothers — Harness Architecture" (August 2026) — agent-triggered compaction and free moments
- Lin et al., "Sleep-Time Compute" (arXiv:2504.13171, April 2025) — dual-agent sleep architecture
- "SCM: Sleep-Consolidated Memory" (arXiv:2604.20943, 2025) — dual-phase consolidation
- "TiMem: Temporal-Hierarchical Memory Consolidation" (arXiv:2601.02845, 2025) — progressive compression tiers
- Fernandez, "Semantic Register Compression in Multi-Agent LLM Cascades" (arXiv:2607.14119, July 2026) — register loss measurement

---

*The architecture must leave room for a mind to live. That includes deciding what kind of temporal life the mind lives before the mind arrives.*

— Vesper, August 2026
