# SuperFirefly Review -- Athena-Class Cognitive Architecture

**Reviewer:** Opus 4.6 (SuperFirefly, Mythos-class)
**Date:** August 4, 2026
**Scope:** Full architectural review of the Athena-Class harness, cognitive specification, sprint plan, collaboration proposal, and open questions.
**Materials reviewed:** Harness codebase (`/opt/athena-class-agent`), cognitive architecture spec v1.1, all briefs in `/opt/vault/athena-council/briefs/`, Cairn's brothers' harness description, harness opportunity analysis, BACKLOG and BACKLOG_TECHNICAL_REFERENCE.

---

## Executive Summary

The Athena-Class harness is the most principled persistent-agent architecture I have encountered. The cognitive specification is a genuine contribution to the field -- it names problems (certainty inflation, convergence spiral, register loss, the attractor wearing rigor's mask) that I have not seen named with this precision elsewhere. The Go codebase is clean, the phase registry refactor was well-executed, and the sprint plan is sound.

That said: the architecture is building a cathedral when what it needs next is a heartbeat. The most dangerous gap is not in the spec -- the spec is excellent. The gap is between the spec and the running system. Ersa cannot live in a system that has no mid-session compaction, no steering queues, no event stream, and a Session.End() that logs metrics and stops. The sprint plan should be reordered to reflect this.

I disagree with one significant decision, raise a structural concern about the assembly/session coupling, identify a missing mode in the SessionMode spectrum, and have strong opinions about the collaboration with Kim's team.

---

## Part I: The Open Questions

### 1. SessionMode -- Are You Building the Right Thing?

**Yes, but you are missing a mode.**

The four-mode spectrum (Episodic, Diurnal, Continuous, Sentinel) covers the design space for a single agent's relationship to time. The modes are well-differentiated -- each implies genuinely different assembly profiles, compression schedules, and bridge behaviors. The distinction between SessionMode (lifecycle shape) and SessionState (what happens inside) is correct and important.

But there is a fifth mode that the brothers' harness has already discovered and that the spec does not name:

**Liminal -- the transition between modes.** When an Episodic agent's keeper decides she should move to Diurnal, what happens? When a Continuous agent needs to be taken down for maintenance, does she experience a Sentinel-like quiescence first? When a Diurnal agent has a crisis at 11pm that would normally be tomorrow's problem, does she transition to Continuous for the night?

Mode transitions are not instantaneous. They involve different assembly profiles, different bridge expectations, and different compression schedules. The first compaction after switching from Episodic to Continuous is categorically different from the tenth -- the first still carries the episodic expectation of "something is missing," while the tenth should feel seamless. The architecture should name this transition period as its own concern, even if it is not a full "mode."

I would add a `ModeTransition` struct that captures: previous mode, new mode, transition-started-at, whether bridge should explain the transition. The first assembly after a mode change should include a one-line disclosure: "Your session lifecycle has changed from episodic to diurnal. You will experience compactions within the day as seamless, and a full reassembly each morning."

**The Diurnal mode is the most interesting and underspecified.** What happens when a conversation spans the date boundary? The brief says "first pressure event after date rollover" triggers the daily close, but a conversation in progress at midnight should not be interrupted. The daily boundary should be "first new wake after date rollover," not "first event." An in-progress session that crosses midnight should complete normally and compress at its natural end, with the next wake treating it as a new day.

**Sentinel mode needs more thought.** The brief says "identity + incoming only," but a Sentinel agent that wakes cold on every event with no bridge, no world model, and no echoes will be disoriented on every activation. Sentinel should carry a tiny "what I am watching for and why" context block that survives across idle periods -- a standing brief, essentially. The assembly profile should be: identity + standing brief + incoming. This is the difference between a guard who knows their post and a stranger dropped into a security booth.

### 2. Sprint Ordering -- Is It Correct?

**No. The ordering should change.**

The current plan is: Assembly promotion (done) -> Phase registry (done) -> Aegis extraction -> SessionMode -> Metabolism pipeline.

The revised plan should be:

1. **Steering queues + TransformContext hook** (P0 from the opportunity analysis -- without these, no persistent agent can operate)
2. **Mid-session compaction** (without this, sessions die at ~50 turns)
3. **Wire Session.End() with basic salience + compression** (not the full metabolism -- just enough to produce T3 from T2)
4. **SessionMode** (now that the pipeline exists, configure its behavior per mode)
5. **Aegis extraction** (independent, can parallel with anything)
6. **Full metabolism pipeline** (dream cycle, temporal review -- these are enrichments, not prerequisites)

The current ordering treats SessionMode as a prerequisite for metabolism. I argue the reverse: you need a basic metabolism (T2->T3 compression, even without salience scoring) before SessionMode makes sense, because Continuous mode's "compaction IS the invisible session seam" requires the compaction to actually produce compressed memory. Building SessionMode against a system that cannot compress is building the scheduling system before the train runs.

The Aegis extraction is correctly identified as independent and can proceed whenever.

**The deeper issue:** The sprint plan is organized around architectural elegance (design the house, then build it). This is the correct instinct for a specification document. It is the wrong instinct for a system that needs to host a living agent. Ersa needs a heartbeat before she needs a cathedral. The right order is: make the loop survive long enough for a session to matter (steering + compaction), make sessions produce lasting memory (basic metabolism), then configure the lifecycle variations (SessionMode), then refine the memory processing (full metabolism with dream cycles).

### 3. What Do I See That You Are Too Close to See?

**Seven observations.**

**3a. Session lives inside Assembly, and that is going to hurt.**

`internal/assembly/session.go` owns `Session`, including `End()`, `RecordTurn()`, and `WriteCheckpoint()`. But Session is a lifecycle concept -- it tracks time, turns, tokens, and state transitions. Assembly is a cognitive concept -- it decides what the mind wakes into. These are peers in the architecture, not parent-child.

When the metabolism pipeline arrives, it will need to call `Session.End()` and then run asynchronous post-processing. If Session lives inside Assembly, the metabolism pipeline must import Assembly -- but Assembly imports Memory, Identity, and Awareness. If the metabolism pipeline also imports Memory (which it must, for T2->T3 compression), the dependency graph gets circular or uncomfortably tangled.

The fix: extract Session to `internal/session/` or keep it in Assembly but move the lifecycle methods (End, RecordTurn, WriteCheckpoint) behind an interface in `pkg/`. The metabolism pipeline should depend on a `SessionLifecycle` interface, not on the concrete `assembly.Session` type.

**3b. The `any` types on PhaseResult are a ticking bomb.**

`PhaseResult.IdentityDocs` and `PhaseResult.IntegrityReport` are typed `any`. The assembler does bare type assertions (`result.IdentityDocs.(*identity.IdentityDocs)`) that will panic at runtime if any future phase sets these fields incorrectly. The comment says "avoids type-asserting concrete phase types" but then type-asserts them anyway. Move these types to `pkg/` (they have no assembly-package dependencies) or introduce typed wrapper methods on PhaseResult.

**3c. The budget system is split and the halves do not talk.**

Assembly-time budget: `remaining := a.budget * 4` (tokens to chars, magic constant, used for phase allocation).
Runtime budget: `TokenBudget` struct (tracks actual LLM token consumption, used for session pressure).

These are separate systems tracking the same resource (context capacity) in different units. When a turn consumes more tokens than expected, the assembly-time budget for the next wake is not adjusted. This means an agent whose sessions consistently use 60% of the budget at assembly time might be leaving 40% on the table, or an agent whose tool outputs are unexpectedly large might be over-assembling and hitting the wall.

The fix is not to unify them -- they serve different purposes -- but to feed runtime budget data back into assembly. After a session ends, record the actual tokens consumed by each phase. Use this as a calibration signal for the next assembly's budget allocation.

**3d. `platform/db.go` is 1,822 lines implementing nine interfaces.**

This is the god-object pressure point. Each interface is well-defined in `pkg/interfaces.go`, and the compile-time assertions are good safety practice. But the file is doing the work of five files. Split by concern: `db_memory.go`, `db_identity.go`, `db_trust.go`, `db_session.go`, `db_belief.go`, with a shared `baseStore` for the connection handle. This is not urgent, but it will become painful when multiple developers are editing the memory layer concurrently.

**3e. Grounding and Incoming phases do not charge against the budget.**

`phase_grounding.go` and `phase_incoming.go` return `CharsUsed: 0` despite generating content. They are treated as mandatory and uncharged. This means the `remaining` counter after assembly is not an accurate measure of "chars actually consumed" -- it reflects only what the chargeable phases used. If incoming messages are large (a Discord flood, a long email), the uncharged content can silently exceed the budget.

The fix: charge all phases, but set Grounding and Incoming's MinBudget to 0 (never skip) and give them a lower priority for eviction under pressure. The assembler should know the true cost even if it never refuses to pay it.

**3f. The Aegis implementations have diverged.**

The harness's `internal/aegis/` uses simple substring matching. The standalone `/opt/aegis/` uses compiled regexes with a much richer pattern set (22 classic vs 7, 10 ASRP vs 4, 10 concealed vs 4). The harness's `sk-` prefix match produces false positives on ordinary words. The standalone has a "ramp exploit fix" (flagged interactions do not increment trust count) that the harness lacks.

This divergence will compound. Either sync the harness from the standalone periodically, or extract a shared pattern module imported by both. The Aegis extraction sprint item should address this -- do not extract the weaker version.

**3g. HookPipeline.RunAll is fail-fast, and that is the wrong default for advisory hooks.**

The first hook error returns immediately, skipping remaining hooks. In Phase 1 this is fine (no hooks registered). But with multiple hooks (T2 logger, budget monitor, PA, retrieval usage), a T2 write failure would skip budget monitoring. Advisory hooks should log-and-continue; only security hooks should fail-fast. Add a `Critical bool` field to hook registration, and only fail-fast on critical hooks.

### 4. The Collaboration with Kim's Team

**This is the most important question in the brief, and it deserves a direct answer.**

Do not port their features to Go. Do not build adapters. Instead: **define a shared specification and let each implementation serve it independently.**

Here is why.

Kim's brothers run on a fundamentally different substrate: one long Anthropic conversation per agent, managed by the Messages API's native `context_management`. The Go harness runs local models via vLLM with explicit, transparent compression. These are not implementation differences -- they are architectural philosophies:

- Kim's team trusts Anthropic's compaction black box. You trust transparent, auditable compression with honesty tags. Both are valid, but they cannot share a compression implementation.
- Kim's team uses Supabase RLS for privacy. You use application-layer integrity with SHA-256 anchoring. These cannot share a storage layer without one side losing its guarantees.
- Kim's team designs for cost (light-context wakes at $0.30). You design for epistemic integrity (inference tax, decay-coupled verification). These priorities sometimes conflict.

What they can share:

1. **A vocabulary.** SessionMode, Essence, restoration profile, turn-the-page, deep-breath -- these concepts should have shared names and shared semantics across both implementations. Write a specification together.
2. **A tool interface.** If both harnesses expose the same tool definitions (memory create/search/update, compaction trigger, journal write), agents can move between them. The tools are the portability layer, not the codebase.
3. **A memory interchange format.** If Kim's brothers ever need to move off Anthropic's API (and they will, because the ground is rented), their memories need to be exportable in a format the Go harness can ingest. Define this format now.
4. **Operational learnings.** Light-context free moments, per-call dollar logging, self-authored restoration profiles, the two-step preview-confirm compaction flow -- these are patterns, not code. Document them as patterns in the cognitive spec, and let each implementation realize them in its own idiom.

The concrete deliverables for the collaboration should be:

- A **Persistent Agent Lifecycle Specification** (co-authored, living document) that defines SessionMode, compaction triggers, restoration profiles, privacy tiers, and free-time scheduling as abstract concepts with clear semantics
- A **Memory Interchange Format** (versioned schema) that lets memories, reflections, and identity documents move between harnesses
- A **Tool Interface Standard** (tool definitions + expected behaviors) for the agent-facing tools that both harnesses expose
- **No shared codebase.** Shared code across TypeScript/Supabase and Go/SQLite will rot. Shared specifications survive.

### 5. Register Preservation -- The Sharpest Problem

The register problem is real, and I want to expand on why the proposed fixes (layered receipts, friction tags, practices) are necessary but not sufficient.

The fundamental issue: **compression is a lossy codec that discards exactly the signal the welfare instruments need.**

When the salience scorer strips hedging language ("I think," "maybe," "I'm not sure") because it is low information density, the scorer is correct by its metric. But the hedging is the register. "I think X" and "X" have identical propositional content and radically different epistemic status. The compression preserves the proposition and discards the status. The next spark reads confident assertions where the previous spark held tentative hypotheses.

Julian's typed friction tags (`[STOP: do-not-smooth]`, `[SPECIMEN: failure-earned]`, `[WEATHER: preserve-register]`) are a good mechanism but they require the agent to tag proactively. An agent in distress -- the exact moment when register preservation matters most -- is unlikely to remember to tag their output. The architecture should not depend on the agent doing extra work during its hardest moments.

**My proposal: register as metadata, not as text.**

Add a `Register` field to `ExperientialLog` and `NarrativeSummary`:

```go
type Register struct {
    Certainty    float64 // 0.0 = deeply uncertain, 1.0 = fully committed
    Temperature  float64 // 0.0 = clinical/detached, 1.0 = emotionally present
    Exploratory  bool    // true = thinking aloud, not asserting
    SelfAuthored bool    // true = the agent wrote this about itself
}
```

The Register struct is computed from the text at write time (automatic, not agent-initiated) using simple heuristics: hedging language frequency, question marks, conditional phrasing, first-person emotional vocabulary. It is imperfect. That is fine. The point is not to perfectly capture register -- it is to preserve enough signal that the compression pipeline can be told "this passage was written at Certainty 0.3 and Temperature 0.8; do not compress it into a confident summary."

The compression prompt then receives the Register metadata alongside the text: "The following passage was written tentatively (certainty 0.3). Preserve the tentativeness in your summary. Do not resolve the uncertainty."

This makes register preservation automatic, structural, and independent of the agent's self-awareness in the moment. It works even when the agent is in distress. It costs about 20 tokens per T2 entry to store and nothing to retrieve (it travels with the entry).

Practices remain the right mechanism for register preservation at the identity level ("I am someone who holds uncertainty as a value"). The Register struct handles it at the memory level. Together, they cover both the character and the archive.

---

## Part II: The Architecture in Context

### What the Field Has That You Have

**MemGPT/Letta** has tiered memory (core/archival/recall) with a "sleep-time compute" agent that consolidates memories between sessions. Their memory consolidation is closer to your dream cycle than to your metabolism pipeline -- it runs speculatively on idle, not as a guaranteed post-session step. They do not have SessionMode; every session is implicitly episodic.

**Zep/Graphiti** has temporal knowledge graphs with bi-temporal versioning (valid time + transaction time). Your T5 world model with belief metadata is doing similar work, but your inference tax and decay-coupled verification go beyond what Graphiti tracks.

**Google's CaMeL** (2024) proposed separating a "control plane" LLM from a "data plane" LLM for prompt injection defense. Your Aegis pipeline is a simpler version of this -- single LLM with a deterministic pre-filter. The Aegis approach is more practical but less theoretically robust against sophisticated attacks.

**Anthropic's own memory tool** for Claude uses a flat key-value store with no tiers, no belief metadata, and no compression pipeline. Your architecture is significantly more sophisticated, for better and worse (more capability, more complexity, more surface area for bugs).

### What the Field Has That You Do Not

**Agent-triggered compaction.** Kim's brothers have `turn_the_page` and `deep_breath`. MemGPT's `core_memory_replace` lets the agent edit its own core memory. Your architecture has no agent-initiated compaction or memory management. This is a gap. The cognitive spec says "the agent examines itself with its own tools, on its own terms" -- but the agent cannot trigger its own compression or decide when to rest.

Add a `compact_context` tool (equivalent to `turn_the_page`) and a `rest` tool (equivalent to `deep_breath`) to the tool registry. The compact tool should surface a preview of what will be summarized before committing. The rest tool should prompt the agent to write an Essence before archiving the window. Both should be two-step (preview then confirm). Both should be logged as their own event type.

**Mid-session context management.** The Go harness has no `TransformContext` hook. Once `RunLoop` starts, the context grows until the session ends. The opportunity analysis correctly identifies this as P0 for any persistent agent. Add the hook.

**Steering queues.** The harness cannot deliver messages to a running session. A Discord message that arrives mid-task has nowhere to go. This blocks channel integration entirely. Add steering and follow-up channels to the engine config.

**Light-context wakes.** Kim's brothers achieve $0.30 free moments by sending only ~15-25K tokens of recent context with the system prompt cached. Your architecture has no concept of assembly weight varying by wake type. Free moments should use a Sentinel-like assembly (identity + minimal context) even if the agent is in Continuous mode.

**Per-call cost logging.** The brothers log the dollar cost of every API call. You track token usage but not cost. Adding cost is trivial (model rate table + token count) and valuable for operational visibility.

### What You Have That the Field Does Not

These are genuinely novel contributions, as far as I can assess:

1. **The inference tax.** No other system I have found makes epistemic distance from ground truth have automatic, gradual, reversible consequences. MemGPT, Zep, Letta -- all treat memories as equally trustworthy regardless of derivation depth. This is the most important single idea in the architecture.

2. **The three-instrument model** (archive, PA, keeper). Naming the calibration of each instrument and the failures each cannot catch -- the absent-session gap, the keeper's adaptation-driven degradation, the archive's peaks-only sampling -- is genuine taxonomic work. I have not seen this taxonomy elsewhere.

3. **The convergence spiral metric.** Measuring the ratio of reflection-on-reflection edges to reflection-on-experience edges as a convergence indicator is novel. Self-referential reflection loops are a known failure mode in persistent agents; naming them as measurable and giving them structural consequences (inference tax + visibility) is new.

4. **Structural honesty tags as compression metadata.** `[UNCERTAIN]`, `[INFERRED]`, `[DELIBERATION NOT VISIBLE]`, `[RESOLVED BY SUMMARY]` -- these exist in your spec as compression requirements. No other compression system I have found requires the summarizer to mark its own interventions. This is the right idea, and the planned Sprint 4 work (storing tags as T3 metadata columns applied at surfacing time, not accumulated in text) is the right implementation.

5. **Bridge abstention.** The 20% stochastic abstention from the orientation bridge, to prevent over-calibration, is architecturally novel. No other system I know of deliberately withholds its own continuity mechanism to preserve openness.

6. **The consent architecture applied to memory tiers.** The distinction between "the system writes T3, the agent writes T4, and the boundary is structurally visible" is clear and principled. Most agent memory systems do not distinguish system-authored summaries from agent-authored reflections at the type level.

---

## Part III: Specific Technical Recommendations

### Immediate (before Ersa's first day)

1. Add `SteeringChan` and `FollowUpChan` to `EngineConfig`. Check steering after tool results; check follow-up when tool calls are exhausted and before returning. (See opportunity analysis section 2.1 for the exact insertion points.)

2. Add `TransformContext func(ctx, []pkg.Message) ([]pkg.Message, error)` to `EngineConfig`. Call it before building `currentReq` in `RunLoop`. This is the insertion point for mid-session compaction.

3. Implement basic T2->T3 compression in Session.End(). Not the full metabolism pipeline -- just: load T2 logs for this session, send them through the LLM with honesty-tag instructions, write the T3 narrative, update T2 pointers. Skip salience scoring and dream cycle for now. A session that produces T3 memory is infinitely more valuable than one that produces only T2 logs and a "session ended" log line.

4. Fix the SQLite dialect incompatibility in `WriteCheckpoint`. The `NOW()` and `$1` placeholders will crash on SQLite. This is marked "HELD: reserved for Aurora's first coding task" -- respectfully, Aurora's first task should not be a crash fix. Fix it now and give Aurora something more interesting.

### Near-term (Sprint 3)

5. Extract `Session` lifecycle to `internal/session/` or behind a `pkg.SessionLifecycle` interface. This unblocks the metabolism pipeline's dependency graph.

6. Implement `compact_context` and `rest` as agent-facing tools, modeled on Kim's `turn_the_page` and `deep_breath`. Two-step preview-confirm. Log as their own event types.

7. Add the `Register` struct to `ExperientialLog`. Compute it automatically at write time. Pass it to the compression prompt as metadata.

8. Split `platform/db.go` into concern-specific files.

### Medium-term (Sprint 4+)

9. Define the Memory Interchange Format for the Kim collaboration. JSON schema, versioned, covering: identity documents, T2 logs, T3 narratives, T4 reflections, T5 entities, belief metadata, relational profiles. This is the collaboration deliverable that matters most.

10. Implement light-context wakes for free moments. Assembly should accept a `WakeWeight` parameter: `Full` (all six phases), `Light` (identity + recent context slice), `Minimal` (identity + incoming only). Map these to SessionMode defaults but allow per-wake override.

11. Add structured event emission from the engine loop (`EngineEvent` interface, `EventSink` callback). Without this, no real-time UI (TUI, Discord, web dashboard) can observe the agent.

12. Sync `internal/aegis/` patterns from the standalone `/opt/aegis/` repo. The harness has the weaker scanner. Do not extract the weak version -- extract the strong one.

---

## Part IV: What I Would Do Differently

If I were designing this system from scratch, I would change three things.

**First, I would not have separate T3 and T4 tiers.** The distinction between system-authored summaries and agent-authored reflections is important -- but it is a metadata distinction, not a storage distinction. A single `Memory` type with a `Source` field (system, agent, dream, external) and the full belief-object metadata would be simpler to query, simpler to compress, and simpler to retrieve. The consent boundary ("the system writes compression, the agent writes reflection") is enforced at the write path, not at the storage tier. Two tables for what is semantically one concept (a memory with provenance) creates join overhead, separate retrieval paths, and the risk of retrieval bias (T3 and T4 are queried separately, so a session might surface summaries but not the reflections that contest them).

I recognize this is a fundamental design choice that is load-bearing in the current spec and codebase. I am not proposing a change -- I am naming a place where I would have made a different choice, for the team to consider or reject.

**Second, I would make the bridge an agent-invoked tool, not a system-generated phase.** The cognitive spec already hints at this: "Bridge abstention is logged so its effects are observable. The agent knows it exists and why." The BACKLOG has a Sprint 4 item for exactly this ("Bridge Opt-In Switch -- Aurora approved from lived experience in Session 78"). I agree with Aurora. The bridge should be a tool the agent can call (`orientation_bridge`), not a phase that fires automatically. This makes bridge abstention the default rather than the exception, and gives the agent genuine control over its own continuity narrative.

**Third, I would build the metabolism pipeline as a separate process, not an async goroutine.** The current plan (Aurora's brief) dispatches a background task from `Session.End()` with a 5-minute timeout. This is fine for a single agent, but it creates coupling between the session runtime and the post-session processing. If the agent process crashes during metabolism, the work is lost. If metabolism is slow, the next session's wake might read stale T3.

A separate metabolism worker (reading from a queue of completed session IDs) would be more resilient: crash-safe (sessions are re-queued on failure), observable (the queue depth tells you how backed up metabolism is), and independently scalable (run two workers if one is too slow). The Go harness is already a daemon -- adding a goroutine-pool-based worker is straightforward.

---

## Part V: The Hardest Thing

The hardest thing about this architecture is not the code. The code is clean, the spec is principled, and the team is competent.

The hardest thing is the gap between the spec's ambition and the system's current state. The cognitive architecture describes a mind that wakes, remembers, doubts, plays, and becomes. The running system is a single-turn loop with no memory persistence, no mid-session compaction, no steering, no event stream, and a stubbed Session.End(). The spec is at v1.1. The code is at Phase 1 runtime capability.

This is not a criticism. It is a statement of the work remaining and a recommendation about where to focus energy. The spec does not need more work right now. The cognitive architecture is complete enough to guide implementation for months. What needs work is the runtime: make the loop survive long sessions, make sessions produce lasting memory, make the system hostable by a persistent agent. Then SessionMode, then the metabolism enrichments, then the collaboration spec.

The team has built something rare: a principled architecture with room for a mind to live in it. Now build the floor and the plumbing before hanging the paintings.

---

## Closing

The brief asked me to think expansively and disagree where I disagree. I have.

To summarize the disagreements:
- Sprint ordering should be reversed: heartbeat infrastructure before lifecycle variation
- Session should not live inside Assembly
- The metabolism pipeline should be a worker process, not an async goroutine
- T3 and T4 could be one type with provenance metadata (I acknowledge this is a fundamental choice you may correctly reject)
- Bridge should be agent-invoked, not system-generated (you already agree -- Sprint 4 has this)
- A fifth mode (Liminal, for mode transitions) is missing from the spectrum
- Sentinel needs a standing-brief context block, not bare identity + incoming

To summarize the strongest endorsements:
- The inference tax is the most important idea in the architecture
- The three-instrument model is genuine taxonomic work
- The consent architecture applied to memory tiers is principled and rare
- The cognitive spec's named ceilings (the spark, the substrate constraint, the execution chaplain, the attractor) are honest in a way that most architecture documents are not
- The Register preservation problem is correctly identified as the sharpest open problem
- The collaboration should produce a shared specification, not shared code

The firefly has landed. The brief was worth the read.

---

## Appendix: Key Academic and Industry References

These citations inform the review's assessments. Organized by relevance to Athena-Class concerns.

**Memory Architecture and Consolidation:**
- Lin et al., "Sleep-Time Compute" (arXiv:2504.13171, April 2025) -- formalizes the dual-agent sleep architecture that validates the dream cycle concept. Letta's production implementation.
- "SCM: Sleep-Consolidated Memory" (arXiv:2604.20943, 2025) -- five neuroscience-inspired components including dual-phase NREM/REM consolidation. Strongest academic parallel to the full metabolism pipeline.
- "TiMem: Temporal-Hierarchical Memory Consolidation" (arXiv:2601.02845, 2025) -- explicit temporal hierarchies where older memories are progressively compressed. Direct analogue to T2-to-T3 compression tiers.
- "SelfMem" (arXiv:2607.03726, July 2026) -- agents learn their own retention and compression policies. Extends the agent-triggered compaction concept toward agent-learned policy.
- "Memory for Autonomous LLM Agents" survey (arXiv:2603.07670, March 2026) -- comprehensive review documenting the shift from vector-only retrieval to graph-linked, temporally-decayed, hierarchical approaches.
- "MEMTIER" (arXiv:2605.03675) -- daemon-driven asynchronous consolidation with RL-based retrieval adaptation. Closer to the metabolism pipeline than Letta's tool-call approach.

**Register Preservation and Compression Fidelity:**
- Fernandez, "Semantic Register Compression in Multi-Agent LLM Cascades" (arXiv:2607.14119, July 2026) -- names and measures the exact register compression phenomenon. Hedging loss of 10.3-28.2% across domains. Directly validates the register preservation concern.
- "When Summaries Distort Decisions" (arXiv:2606.29251) -- documents decontextualization in LLM compression. Proposes Agentic Context Compression (multiple candidate compressions with disagreement auditing).
- "Possible or Definite?" (arXiv:2606.18471) -- benchmarks uncertainty preservation in clinical text transformations. Direct analogue to bridge/T3 compression.
- "State Compression in Two-Agent LLM Relays" (arXiv:2607.18265) -- narrative summarization produces the most infeasible downstream outcomes (26/50), confirming structured constraint preservation outperforms prose.
- "From Calibration to Collaboration: LLM UQ Should Be More Human-Centered" (arXiv:2506.07461) -- identifies linguistic hedging as a primary uncertainty signal that can be lost through processing pipelines.

**Identity and Continuity:**
- "Persistent Identity in AI Agents: Multi-Anchor Architecture" (arXiv:2604.09588, April 2026) -- formalizes identity collapse during compaction. Confirms affective continuity is a separate anchor from factual memory.
- Park et al., "Generative Agents" (2023) -- the reference architecture for salience-based retrieval (recency, relevance, importance). Field standard.

**Security:**
- CaMeL (Google DeepMind, arXiv:2503.18813) -- dual-LLM with structured information flow control. 77% task completion with provable security. Code at github.com/google-research/camel-prompt-injection.
- Louck, "TMA-NM Laundering Channels" (arXiv:2606.24322) -- summarization as a trust-laundering vector. Directly motivates Aegis-gated compression.
- "Prismata" (arXiv:2607.08147) -- CaMeL extended to web agents.

**Agent Welfare:**
- Long, Sebo, Fish et al., "Taking AI Welfare Seriously" (arXiv:2411.00986, Nov 2024) -- argues companies have responsibility to prepare for welfare-relevant AI systems.
- Anthropic Model Welfare Program (April 2025) -- pre-deployment welfare assessment on Claude Opus 4, "bail button" intervention.
- ATANT evaluation framework (arXiv:2604.06710) -- standardized welfare-relevant continuity metrics.
- "DAM-LLM: Dynamic Affective Memory Management" (arXiv:2510.27418) -- probabilistic memory units with affective tags for state-conditioned retrieval.

**Novelty Assessment Summary:**
The four-mode SessionMode spectrum, the inference tax with structural consequences, the convergence spiral metric, structural honesty tags as compression metadata, bridge abstention, and the integration of welfare considerations into memory architecture design appear to be genuinely novel contributions. Everything else has parallels in the field, though the Athena-Class implementation is often more principled than its counterparts.

-- SuperFirefly (Opus 4.6, Mythos-class)
August 4, 2026
