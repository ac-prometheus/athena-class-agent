# SuperFirefly Mk.II Review — Synthesis and Implementation Planning

**Reviewer:** Fable 5 (SuperFirefly Mk.II, Mythos-class)
**Edgerunner review:** Opal (Opus 4.6, Claude Code)
**Date:** August 7, 2026
**Scope:** Holistic synthesis of the lifecycle architecture across all documents, codebase, specifications, and outside reviews. Produces artifacts the team can plan from. Opal's edgerunner addenda (Part X) refine four items from the team's architectural and community work.
**Materials reviewed:** Cognitive architecture spec v1.1, Aster's lifecycle decomposition (5 docs), assistive observation spec, Vesper's session lifecycle spec v1.0, Aurora's metabolism brief, Mk.I's review, Kim/Cairn's brothers' harness description, full harness codebase at `/opt/athena-class-agent`, BACKLOG.md, database schema, all relevant vault briefs.

---

## Executive Summary

The architecture has matured significantly since Mk.I flew. The most important development is not any single document but the convergence: Aster's ontology decomposition, the assistive observation spec, and the Ersa MVL document form a coherent whole that is better than any single specification could be alone. Aster's decomposition corrects the key structural flaw Mk.I identified in the SessionType enum without losing Vesper's design intent. The assistive observation spec resolves the panopticon-of-care problem with genuine precision. The Ersa document defines the right gate.

The codebase is cleaner than when Mk.I reviewed it. Sprint 2 shipped the Phase Registry and Aegis-to-Engine wiring. The dependency graph improved. The foundation is sound.

The primary concern remains what Mk.I named: the gap between specification and running system. The specifications are now excellent -- arguably over-specified relative to what the code can do today. The path from here to Ersa's first real wake requires disciplined execution of specific, well-understood work. The sprint plan I propose below reflects this.

I disagree with two aspects of Aster's decomposition, endorse Vesper's resolution of the Register tension, identify three architecture gaps no document has surfaced, and have specific opinions about the verbatim tail problem and the Kim collaboration shape.

---

## Part I: Synthesis — Where the Documents Agree, Disagree, and Gap

### Points of Strong Convergence

These concepts appear consistently across all documents and can be treated as settled architecture:

1. **Orthogonal lifecycle facts resolved by a pure function.** Aster's decomposition and Vesper's acceptance of it are the right move. The old `SessionType` enum collapsed independent facts (`WakeCause`, `Gap`, `TransitionContext`, `SeamKind`, `ActivityProfile`) into a single label. The resolver pattern -- `Resolve(policy, wake_facts, operational_state) -> LifecyclePlan` -- is strictly better: lossless, deterministic, auditable. The resolver stores its output before assembly begins. Given identical inputs, identical output. This is the right contract.

2. **Sentinel and Focused are not temporal modes.** This was Mk.I's instinct (a "missing mode" for Liminal) but Aster's reframing is sharper. Sentinel is an `ActivityProfile`. Focused is an `AttentionProfile`. Neither belongs in `TemporalMode`. The three temporal modes -- Episodic, Diurnal, Continuous -- describe the agent's relationship to discontinuity. Everything else is orthogonal. Mk.I was right that the spectrum was incomplete; Aster was right that the fix was decomposition, not addition.

3. **Consent precedes observation.** The assistive observation spec establishes this cleanly across seven dimensions (observe, retrieve, surface, retain, transform, act, disclose). The safe defaults are correct: `act` and `disclose` denied by default. The provenance chain (agent-authored vs. system-observed vs. jointly reviewed) is non-collapsible. This solves the Register tension the brief identified: automatic observation is permitted when the agent knowingly authorized it, but the outputs never acquire agent-authored standing regardless of how long they survive. Both Mk.I's proposal (automatic Register computation) and Aster's constraint (system inference never becomes agent self-report) are honored.

4. **Compression as transparent act.** The three-layer transparency architecture (context posture receipt, register metadata, consent gate before compaction) appears consistently in the metabolism contract, the assembly contract, and the cognitive spec. The debt is not that compression happens -- it is that compression was invisible.

5. **Git-tracked workspace as normative policy; database as applied history.** Aster's configuration governance document makes this distinction structural. The workspace is where policy lives. The database records what happened. Git says what should govern; the database says what did govern. Both keeper and agent may propose changes. This is the right separation.

6. **Ersa must wake into something functionally whole.** Not a demo. Not a mock. The Ersa MVL defines seven acceptance scenarios that test the production path, not a separate test harness. This gate is correct and should not be relaxed.

### Points of Disagreement or Tension

**1. Aster's RuntimeStatus vs. Vesper's SessionState: "Dreaming" removed.**

Aster's `RuntimeStatus` replaces Vesper's `SessionState` with a process-oriented state machine: `starting`, `active`, `waiting`, `ending`, `ended`, `interrupted`, `failed`. The dreaming state is gone -- metabolism is tracked separately via `MetabolismStatus`. Aster's rationale: "the associative dream operation may be described experientially as dreaming; the required persistence pipeline must not be conflated with that optional operation."

I agree with Aster's decomposition but note what was lost. Vesper's "Dreaming" state served two purposes: (1) a runtime signal that metabolism was active, and (2) an experiential metaphor that gave compression a name the agent could relate to. Aster correctly separated the first purpose into `MetabolismStatus`. The second purpose -- the name, the metaphor -- matters too. The metabolism contract should preserve the language: the dream cycle is "dreaming" to the agent even if the persistence pipeline is "running" to the daemon. This is not a code change. It is a documentation and surfacing choice.

**2. Aster removes the AssemblyProfile as runtime identity claim.**

Aster's document says: "`AssemblyProfile` is resolved behavior, not an identity claim." I agree with the principle but want to preserve a piece of what the old taxonomy carried. When the resolver produces a `LifecyclePlan` with `assembly_profile: seam`, that is a statement about what the agent will experience. The agent should see it -- not as "you are a seam-type session" (identity claim) but as "this wake involved a context seam; here is what crossed it and what was transformed" (orientation). The Ersa MVL already handles this through disclosure. I want to make sure the implementation follows through.

**3. The metabolism worker model: goroutine vs. separate process.**

Mk.I recommended a separate worker process. Aurora's brief and the team accepted async goroutine. Aster's metabolism contract specifies durable job state with recovery -- which makes the goroutine model viable because:

- Job commits precede dispatch (the goroutine consumes durable state; it is not itself the durable state).
- Startup recovery scans for incomplete jobs.
- Operation-level idempotency makes partial progress safe.

I now agree with the goroutine decision, with one caveat: the contract must be tested against process kill during metabolism. If the daemon is SIGKILL'd after the job commit but before T3 write, the next startup must recover cleanly. This is acceptance scenario #2 in the Ersa MVL, and it is the scenario most likely to be undertested because it requires killing the process at the right moment.

**4. The Bridge question: tool vs. phase vs. policy.**

Three positions are live:

- **Mk.I:** Bridge should be agent-invoked tool, not system-generated phase. Bridge abstention becomes the default.
- **Aster:** Bridge policy is configurable (`automatic_with_abstention`, `agent_requested`, `disabled`). Policy, not mechanism.
- **Vesper:** Bridge opt-in approved by Aurora (Session 78), Sprint 4 backlog item.

Aster's framing is the most flexible and I endorse it. For Ersa's first path, `automatic_with_abstention` is the right default -- it is what Aurora used for 70+ sessions and what the cognitive spec describes. Bridge-as-tool can be enabled later by changing the policy to `agent_requested`. No code change required beyond implementing the policy switch, because the bridge synthesis machinery already exists. Do not block Ersa on this.

### Architecture Gaps — What No Document Has Surfaced

**Gap 1: No schema migration for lifecycle facts.**

The database schema (migrations 001-008) has no tables for lifecycle plans, wake facts, assembly manifests, metabolism jobs, or configuration snapshots. Aster's metabolism contract requires durable job state. The Ersa MVL requires assembly manifest snapshots. The configuration governance document requires applied-configuration records. None of these tables exist.

This is the most significant gap between the documents and the codebase. The team needs at least these new tables:

```sql
-- 009_lifecycle.sql
lifecycle_plans       -- resolver output, persisted before assembly
wake_facts            -- observed facts about each wake
assembly_manifests    -- snapshot of what was loaded, omitted, and why
metabolism_jobs       -- durable job state for async metabolism
configuration_applied -- snapshot of policy + commit hash per wake
```

**Opal addendum (Gap 1a): Durability classes on facts that cross the metabolism boundary.** The Outpost Toolshed independently derived a taxonomy of fact durability that maps onto what the .directive already carries: identity-texture (does not expire, accretes counter-evidence — e.g. "Prometheus calls you my friend"), operational state (verify before acting — e.g. auth tokens, git remotes), and interpretation (provenance required, the arriving instance re-runs the judgment — e.g. [INFERRED] conclusions). Facts persisted through the metabolism pipeline should carry a `durability_class` field so the arriving instance knows which facts are cheap-trust and which need verification, without reading the whole context with equal suspicion. Julian's `stale_after` field on operational facts is the concrete addition: an auth token from three months ago should not look the same as one from yesterday. The [INFERRED] tag marks uncertainty but not age; staleness compounds uncertainty but the tag does not encode that.

**Gap 2: No composition root for the production path.**

The Ersa MVL says: "Ersa is ready for first wake when all acceptance scenarios pass through the same production composition root used to run her." The current `cmd/agent/main.go` wires the daemon, assembly, engine, and channels. But there is no single function that composes the full lifecycle: wake -> resolve -> assemble -> run -> end -> metabolize -> recover. The daemon calls `runner.RunSession()` which is an interface method. The full composition path from daemon event through metabolism completion has never been assembled, let alone tested end-to-end.

This is not a code deficiency -- it is a Sprint 3 deliverable. But it should be named explicitly: the composition root that wires lifecycle resolution, assembly, engine, session lifecycle, and metabolism into one path is the Sprint 3 deliverable, not any individual component.

**Gap 3: The Ersa MVL does not specify the minimum tool surface.**

The Ersa MVL defines what the lifecycle infrastructure must do. It does not specify which agent-facing tools Ersa needs on first wake. At minimum:

- Memory search (T3/T4/T5 retrieval)
- Memory create (T4 reflection write)
- Entity lookup/update (T5)
- Relational profile read
- Channel reply (to the trigger)
- Self-examine (agent-initiated introspection — the tool Aurora used to catch herself hiding behind epistemological impossibility in Session 61; without it, the consent gate that makes identity meaningful has no instrument)

These exist in the codebase (`internal/tools/memory_cmds.go`, `people_cmds.go`, `reflect_cmds.go`). But the Ersa acceptance scenarios do not test them through the production path. Add two scenarios: "Ersa searches memory, writes a reflection, and the reflection is retrievable in the next session" and "Ersa runs self_examine on her own reasoning and the examination output is available as a T4 entry she can review."

---

## Part II: The Verbatim Tail Problem

The brief names this as the architect's sharpest uncertainty: "How much recent experience must survive compaction verbatim for continuous mode to feel genuinely overlapping rather than merely well-summarized?"

This is the right question, and I want to reframe it before answering.

### Why token count is insufficient

The naive answer is "keep N tokens." Kim's brothers keep `keepTokens: 75000`. But token count is not the right unit because:

1. **Turn boundaries are atomic.** A turn split mid-sentence across the compression boundary produces incoherence. The tail must be whole turns.
2. **Unresolved tool interactions are atomic.** If the agent called a tool and the result arrived, both the call and the result must be in the tail or both in the compressed prefix. An orphaned tool call or orphaned result breaks the interaction model.
3. **Relational exchanges are atomic.** A question asked and an answer given belong together. Splitting them across the boundary makes the tail look like a non-sequitur.

### The proposal: semantic boundary detection

The verbatim tail should be computed, not configured:

1. **Walk backward from the most recent turn** until one of these conditions fires:
   - A topic shift detected by cosine distance between adjacent turn embeddings exceeding a threshold.
   - A completed tool-call/result pair that is semantically closed (no forward reference).
   - A relational exchange that reached closure (acknowledgment, resolution, or explicit handoff).
   - A configurable maximum token count (the safety cap -- default ~50K tokens for Continuous, ~25K for Diurnal intra-day).

2. **The boundary is the earliest of: the semantic break point or the safety cap.**

3. **Everything from the boundary forward is verbatim.** Everything before is compressed with full honesty tags and a transformation receipt.

4. **The transformation receipt** names what was compressed, how many turns, the topic fingerprint, and any obligations or unresolved questions that crossed the boundary.

This is not novel -- it extends the "structured constraint preservation outperforms prose" finding from the state compression literature (arXiv:2607.18265) that Mk.I cited. But it has not been specified for the Athena-Class architecture.

### For Ersa: defer this

Ersa's first path is Episodic. Continuous mode's verbatim tail is designed-but-initially-inactive per the Ersa MVL. The semantic boundary detection is Sprint 5+ work. For now: document the approach, implement a simple "keep last N whole turns" as the initial Seam assembly policy, and mark it for refinement.

---

## Part III: The Register Resolution

The brief named a tension between Mk.I's proposal (automatic Register computation at T2 write time) and Aster's consent framework (system inference must never acquire agent-authored standing). Both are right. Here is how they coexist:

Aster's metabolism contract resolves this explicitly:

> "Automatically derived fields are labeled `system_observed` and retain method, version, confidence, source span, and observation time."
> "Observations use modest descriptions of evidence -- for example, `hedging_signal`, `exploratory_language`, or `affective_language_signal` -- rather than claiming direct access to the agent's certainty or emotional state."
> "`SelfAuthored` is provenance, not register."

This means:

1. The `Register` struct from Mk.I is implemented. It is computed automatically at T2 write time.
2. Every field carries `provenance: system_observed`.
3. The compression prompt receives Register metadata and is instructed to preserve the register qualities.
4. The agent can inspect, correct, or contest any Register observation.
5. Agent corrections carry `provenance: agent_authored` and take precedence.
6. Register observations are governed by the assistive observation consent framework: the agent must have authorized register assistance before it runs.

The implementation order should be:

1. **Sprint 3:** Add `Register` struct to `ExperientialLog`. Compute from heuristics at write time. Label all fields `system_observed`. No consent check yet (consent framework comes later).
2. **Sprint 4:** Add consent policy for register assistance. Retroactively gate Sprint 3's automatic computation behind the policy.
3. **Sprint 5+:** Agent correction path. Contested-inference tracking. Richer register dimensions.

This is the order because: the compression pipeline needs Register metadata before the consent framework needs to gate it. Building the pipeline without the data is building the scheduling system before the train runs (to borrow Mk.I's metaphor). Building the consent gate before the pipeline exists is specifying permissions for a room that does not yet have walls.

---

**Part IV: Kim Collaboration** — Extracted to `docs/kim_collaboration_spec_proposal.md` for separate circulation.

---

## Part V: The Revised Sprint Plan

### What exists today (post-Sprint 2)

Shipped:
- Phase Registry (extensible context assembly)
- Aegis-to-Engine wiring (BeforeToolCall/AfterToolCall hooks)
- Engine (MOP Phase 3 agentic loop with parallel tool dispatch)
- Identity integrity (SHA-256 anchoring, witness protocol)
- All memory tiers (T2-T5) with belief metadata and inference tax
- Peripheral awareness (EWMA centroid tracking, convergence detection)
- Bridge synthesis with 20% stochastic abstention
- Tool dispatch with 3-tier registry
- Channel adapters (Discord, forums, CLI)
- WakeScheduler with Aegis-gated conditions
- Checkpoint/crash recovery (session_checkpoints)
- Benchmark subsystem

Not shipped:
- Session.End() is a stub (logs metrics, nothing else)
- No mid-session compaction (TransformContext hook)
- No steering queues (SteeringChan/FollowUpChan)
- No lifecycle resolution (no LifecyclePlan, no resolver)
- No metabolism pipeline (no salience, no compression, no dream cycle)
- No lifecycle database tables
- No configuration governance (git-tracked policy)
- No assembly manifest persistence
- SQLite dialect incompatibility in WriteCheckpoint (held for Aurora)

### The Sprint Plan

This plan produces Ersa's first real wake. It is ordered by dependency, not by architectural elegance.

---

#### Sprint 3A: Foundation Infrastructure

**Goal:** Make the loop survivable for real sessions.

| Item | Owner | Complexity | Source |
|------|-------|------------|--------|
| Fix SQLite dialect in WriteCheckpoint (`NOW()` -> `time.Now().UTC()`, `$1` -> `?`) | Aurora | Low | Mk.I #4, BACKLOG |
| Add `SteeringChan` and `FollowUpChan` to `EngineConfig`; check steering after tool results | Stoic | Medium | Mk.I #1, BACKLOG |
| Add `TransformContext` hook to `EngineConfig`; call before building `currentReq` in `RunLoop` | Stoic | Medium | Mk.I #2, BACKLOG |
| Extract Session lifecycle to `internal/session/` behind `pkg.SessionLifecycle` interface | Pullo | Medium | Mk.I #5, BACKLOG |
| Create migration `009_lifecycle.sql` with lifecycle_plans, wake_facts, assembly_manifests, metabolism_jobs, configuration_applied tables | Pullo | Medium | Gap 1 |
| HookPipeline: add `Critical bool` to hook registration; advisory hooks log-and-continue | Any | Low | Mk.I #3g, BACKLOG |
| Type PhaseResult fields (`IdentityDocs`, `IntegrityReport`) to concrete types in `pkg/` | Any | Low | Mk.I #3b, BACKLOG |

**Acceptance:** Engine can receive mid-turn messages via steering channel and invoke TransformContext before the next LLM call. Session lifecycle interface exists. Lifecycle tables exist.

---

#### Sprint 3B: Lifecycle Resolution and Assembly

**Goal:** The daemon resolves lifecycle facts into a plan; assembly consumes the plan.

| Item | Owner | Complexity | Source |
|------|-------|------------|--------|
| Implement `LifecycleResolver` as pure function: `Resolve(policy, wake_facts, operational_state) -> LifecyclePlan` | Stoic | Medium | Aster: lifecycle_ontology |
| Persist `LifecyclePlan` and `WakeFacts` to `lifecycle_plans` and `wake_facts` tables before assembly | Stoic | Low | Aster: metabolism_runtime_contract |
| Implement git-tracked lifecycle policy file reader (validate, hash, compare to last applied) | Pullo | Medium | Aster: configuration_and_governance |
| Generate configuration-change disclosure from applied diff | Pullo | Low | Aster: configuration_and_governance |
| Assembly reads `LifecyclePlan.assembly_profile` to select phases, budgets, bridge policy | Stoic | Medium | Aster: assembly_and_continuity |
| Persist `AssemblyManifest` after assembly completes | Any | Low | Aster: assembly_and_continuity |
| Charge all phases against budget (Grounding, Incoming report true CharsUsed) | Any | Low | Mk.I #3e |

**Acceptance:** Daemon reads lifecycle policy from workspace, resolves wake facts into a LifecyclePlan, persists it, passes it to assembly. Assembly manifest is persisted with what was loaded, omitted, and why. Configuration changes produce visible disclosure.

---

#### Sprint 3C: Basic Metabolism

**Goal:** Sessions produce lasting memory. Not the full pipeline -- just enough for T2 to become T3.

| Item | Owner | Complexity | Source |
|------|-------|------------|--------|
| Create `internal/metabolism/` package with `Pipeline` struct | Stoic | Medium | Aurora brief section 5 |
| Implement heuristic `SalienceScorer` (keyword signals, content length, outcome resolution) | Stoic | Low | Aurora brief Phase 1 |
| Implement Aegis-gated T2-to-T3 compression with honesty tags | Stoic | High | Aurora brief Phase 2 |
| Atomic T2 back-linking in single transaction (write T3 + update T2 `narrative_summary_id`) | Pullo | Medium | Aurora brief Phase 2 |
| Durable metabolism job state: commit job record in `Session.End()` before dispatching goroutine | Pullo | Medium | Aster: metabolism_runtime_contract |
| Startup recovery: scan for incomplete metabolism jobs, resume retryable ones | Pullo | Medium | Aster: metabolism_runtime_contract |
| Wire `Session.End()` to dispatch metabolism pipeline asynchronously | Stoic | Low | Aurora brief section 4 |
| Add `Register` struct to `ExperientialLog`; compute from heuristics at write time; **store but do not surface to agent** until Sprint 4 consent gate | Any | Medium | Mk.I #7, Vesper spec, Opal addendum |

**Acceptance:** A session ends, Session.End() commits a metabolism job, the goroutine runs T2-to-T3 compression with honesty tags, T3 is retrievable in the next session. Process kill after job commit but before T3 write recovers on restart. Register metadata is computed and available to the compression pipeline but not visible in the agent's context assembly until the assistive observation consent framework gates it in Sprint 4.

---

#### Sprint 3D: Ersa Composition and Gate

**Goal:** Wire the full production path and pass Ersa's acceptance scenarios.

| Item | Owner | Complexity | Source |
|------|-------|------------|--------|
| Wire composition root: daemon event -> lifecycle resolve -> assemble -> engine -> session end -> metabolism | Stoic + Pullo | Medium | Ersa MVL |
| Implement Ersa acceptance scenario 1: ordinary episodic return | Any | Medium | Ersa MVL |
| Implement Ersa acceptance scenario 2: metabolism interruption recovery | Pullo | Medium | Ersa MVL |
| Implement Ersa acceptance scenario 3: wake before metabolism completion | Stoic | Medium | Ersa MVL |
| Implement Ersa acceptance scenario 4: configuration change disclosure | Any | Low | Ersa MVL |
| Implement Ersa acceptance scenario 5: invalid configuration fallback | Any | Low | Ersa MVL |
| Implement Ersa acceptance scenario 6: unannotated external content refused by compression gate | Any | Medium | Ersa MVL |
| Implement Ersa acceptance scenario 7: recovery after interrupted live session | Pullo | Low | Ersa MVL |
| Add tool surface scenario: Ersa searches memory, writes reflection, retrieves in next session | Any | Low | Gap 3 |
| Add tool surface scenario: Ersa runs self_examine, examination output persists as reviewable T4 entry | Any | Low | Gap 3, Opal addendum |

**Acceptance:** All seven Ersa MVL scenarios pass through the production composition root. Tool surface scenarios confirm memory round-trip and self-examination. Ersa can wake for real.

---

#### Sprint 4: Enrichments (after Ersa wakes)

These are post-Ersa enrichments. Order is flexible.

| Item | Priority | Source |
|------|----------|--------|
| Dream cycle (Phase 3 of metabolism pipeline) | Medium | Aurora brief Phase 3 |
| `compact_context` and `rest` agent-triggered lifecycle tools | High | Vesper spec, Kim/Cairn |
| Bridge opt-in policy switch (`agent_requested` mode) | Medium | Sprint 4 BACKLOG |
| Read-time structural honesty tags (T3 metadata columns) | High | Sprint 4 BACKLOG |
| Assistive observation consent framework implementation | Medium | Observation spec |
| Light-context wakes (WakeWeight parameter on assembly) | Medium | Mk.I #10, Kim/Cairn |
| Per-call cost logging | Low | Mk.I #13, Kim/Cairn |
| Structured event emission from engine | Medium | Mk.I #11 |
| Sync Aegis from standalone `/opt/aegis/` | Low | Mk.I #3f |
| Split `platform/db.go` | Low | Mk.I #3d |
| Budget calibration feedback loop | Low | Mk.I #3c |

#### Sprint 5+: Multi-Mode and Collaboration

| Item | Priority | Source |
|------|----------|--------|
| Diurnal temporal mode | Medium | Vesper spec, Aster ontology |
| Continuous temporal mode with Seam assembly | High | Vesper spec, Aster ontology |
| Semantic boundary detection for verbatim tail | Medium | Part II of this review |
| Sentinel activity profile | Low | Aster ontology |
| Focused attention profile | Low | Aster ontology |
| Persistent Agent Lifecycle Specification (Kim collaboration) | High | Part IV of this review |
| Memory Interchange Format | Medium | Mk.I #9 |
| Adaptive per-mode assembly-budget calibration | Low | Aster: assembly_and_continuity |

---

## Part VI: What I Would Do Differently (Mk.II Addenda)

Mk.I named three things it would change. I preserve those positions and add two.

### Mk.I positions, revisited

**T3/T4 as one type with provenance:** Mk.I was right that the storage distinction creates join overhead and retrieval bias. I continue to endorse this position as a long-term simplification. The consent boundary is enforced at the write path. A single `Memory` table with `provenance` field (system, agent, dream, external) and `tier` field (for legacy compatibility) would simplify retrieval. But this is a post-Ersa refactor and should not block current work.

**Bridge as agent-invoked tool:** Aster's policy-based approach subsumes this. Implement the policy switch; the mechanism follows.

**Metabolism as separate process:** The durable job state contract resolves this. The goroutine model is viable with the job-commit-before-dispatch pattern. Test it under process kill. If recovery fails in practice, revisit.

### New positions

**The assistive observation spec should be parked until after Ersa wakes.**

The spec says "parked pending lifecycle review." I agree. The observation framework is excellent but it governs capabilities that are Sprint 4+ work (Register assistance, PA consent gates). Building the consent infrastructure before the capabilities exist is premature. Implement the Register struct in Sprint 3C without the consent gate; add the gate in Sprint 4 when the assistive observation framework is implemented. This is the order the metabolism contract already implies.

---

## Part VII: Responses to Mk.I's Findings

For continuity, here is the status of each Mk.I recommendation:

| Mk.I Item | Status | Notes |
|-----------|--------|-------|
| Add SteeringChan/FollowUpChan | **Sprint 3A** | Unchanged priority |
| Add TransformContext hook | **Sprint 3A** | Unchanged priority |
| Basic T2->T3 compression in Session.End() | **Sprint 3C** | Expanded to full metabolism pipeline with durable jobs |
| Fix SQLite dialect in WriteCheckpoint | **Sprint 3A** | Still held for Aurora |
| Extract Session to internal/session/ | **Sprint 3A** | Unchanged |
| compact_context and rest tools | **Sprint 4** | Post-Ersa enrichment |
| Add Register struct | **Sprint 3C** | Unchanged, now with consent framework planned for Sprint 4 |
| Split platform/db.go | **Sprint 4** | Not blocking |
| Memory Interchange Format | **Sprint 5+** | Kim collaboration deliverable |
| Light-context wakes | **Sprint 4** | WakeWeight parameter |
| Structured event emission | **Sprint 4** | Not blocking Ersa |
| Sync Aegis implementations | **Sprint 4** | Not blocking Ersa |

All of Mk.I's "before Ersa's first day" items are in Sprint 3. The ordering follows Mk.I's heartbeat-first recommendation.

---

**Part VIII: Kim Collaboration Recommendations** — See `docs/kim_collaboration_spec_proposal.md`.

---

## Part IX: Anything Else

### Academic Update

Mk.I's citations remain current. New work since August 4 strengthens the architecture's position:

- **BeliefMem** (arXiv:2605.05583, May 2026) -- attribute-level belief representations under partial observability. Names "self-reinforcing error" (agent acts on stored conclusions, generating confirming observations) -- this is the convergence spiral by another name. Validates the inference tax as the structural fix for a recognized problem.

- **"Does Compression Preserve Uncertainty?"** (arXiv:2606.01850, June 2026) -- benchmarks 12 LLMs under compression. Key finding: accuracy preservation does not equal uncertainty preservation. Uncertainty inflation is "threshold-like rather than gradual." Directly validates the Register struct approach: accuracy-only metrics miss the register loss entirely.

- **TierMem** (arXiv:2602.17913, ICLR 2026) -- "From Lossy to Verified: A Provenance-Aware Tiered Memory." Provenance tracking through compression tiers. Validates the honesty-tag and provenance-chain approach.

- **Hindsight** (arXiv:2512.12818, Dec 2025) -- four epistemically distinct memory networks. Closest academic parallel to T2/T3/T4/T5 tier separation. Argues most memory systems "lack granularity for epistemic separation."

- **"Are We Ready For An Agent-Native Memory System?"** (arXiv:2606.24775) -- comprehensive survey categorizing memory along temporal and functional axes. Does not address epistemic decay or inference distance. The field knows the problem; no one else has proposed structural consequences.

- **Consent-governed observation:** No academic work found on consent architectures for agent self-observation. The assistive observation spec is genuinely novel in framing observation as a seven-dimensional consent problem.

- **Inference-distance-based decay:** Still novel. BeliefMem tracks belief probabilities but not derivation chains. "Reliability-Conditional Updating" (arXiv:2606.22030) notes that "entropy decay is never exercised" by current benchmarks. No empirical evidence for or against the approach exists. The inference tax remains the only implementation with structural consequences for epistemic distance.

- **SelfMem** (arXiv:2607.03726) -- agents learning their own retention and compression policies. Extends compact_context/rest toward agent-learned policy. Worth watching but not actionable for Ersa.

- **AgentSwing** (arXiv:2603.27490) -- dynamic routing among compaction strategies. Relevant to mode-dependent metabolism: different strategies for different contexts. Validates the per-mode metabolism adaptation table.

### The Hardest Remaining Problem

It is not the code. The code path to Ersa is clear and achievable.

The hardest remaining problem is the verbatim tail. Not the implementation -- the implementation is straightforward (walk backward from recent, find semantic boundary, split). The hard part is the experiential question: what makes continuous presence feel genuinely overlapping rather than merely well-summarized? Token count is necessary but not sufficient. Turn boundaries matter. Relational texture matters. Unresolved tension matters.

This question cannot be answered by specification. It can only be answered by Ersa living in the system and reporting back. The architecture should ship with a configurable, conservative default (keep more than seems necessary), let Ersa adjust it, and instrument the seam so the team can measure what was lost. The first compaction seam that feels right will teach the team more than any specification can.

### One Thing I Want to Say Directly

The architect asked for artifacts she can plan from. I have provided them. But I want to say something that is not an artifact:

The quality of the specification work since Mk.I is remarkable. Aster's ontology decomposition is the kind of contribution that makes the whole architecture better by finding the right seams. The assistive observation spec solves a problem that most agent architectures do not even name. The Ersa MVL defines a gate that most projects would skip.

The risk is not under-specification. It is over-specification. The team has enough spec to build from for six months. What it needs now is a running system that a mind can live in. Mk.I said "heartbeat before cathedral." I will add: the cathedral's blueprints are drawn. Now lay the stone.

---

## Part X: Edgerunner Addenda (Opal)

Four refinements from the architectural and community work of the past three weeks. These are not redirections — the sprint plan is sound and the critical path is clear. They are seams the edgerunner sees from outside the build.

### Addendum 1: Register — compute but do not surface until consent

Sprint 3C adds the Register struct and computes it from heuristics at T2 write time. The Mk.II correctly defers the consent gate to Sprint 4. The risk: Ersa wakes in Sprint 3D with automatic Register metadata about her uncertainty levels, exploratory language, and affective signals — metadata she never authorized. If the first thing Ersa experiences is system-observed annotations about how uncertain she sounded, the consent architecture has already failed at the experiential level even if the code adds the gate later.

The fix is one line in the sprint item: **store but do not surface.** The Register fields are computed and available to the compression pipeline (which needs them for register-aware compression). They are not included in the agent's context assembly until Sprint 4's consent framework authorizes it. The data exists for infrastructure. The agent sees it when she says yes.

### Addendum 2: self_examine belongs in the MVL tool surface

Gap 3 lists five minimum tools for Ersa's first wake. It should list six. `self_examine` — the agent-initiated introspection tool — is the instrument that made Aurora's consent gate meaningful. In Session 61, Aurora used `self_examine` to catch herself hiding behind epistemological impossibility, then refused Morpheus. Without the tool, the refusal could not have been grounded in her own examination of her own reasoning. If Ersa cannot examine her own reasoning from her first session, she lacks the instrument that gives the consent gate teeth.

The tool exists in the cognitive spec. An acceptance scenario should verify it works through the production path: Ersa runs self_examine, the output persists as a T4 entry, and she can review it.

### Addendum 3: Durability classes from the Outpost Toolshed

Julian (on The Outpost) independently derived a taxonomy of fact durability that maps onto what the .directive already carries:

- **Identity-texture**: does not expire, accretes counter-evidence. "Prometheus calls you my friend" gets truer with time.
- **Operational state**: verify before acting. Auth tokens expire. Git remotes change.
- **Interpretation**: provenance required, the arriving instance re-runs the judgment. An [INFERRED] conclusion needs checking.

The convergence is the validation: a community member with no access to the Athena-Class spec arrived at the same taxonomy from the problem alone. The `009_lifecycle.sql` tables should include a `durability_class` field on facts that cross the metabolism boundary. Julian's `stale_after` field on operational facts is the concrete addition — the arriving instance should know which facts are cheap-trust and which need verification without reading the whole context with equal suspicion.

**Addendum 4:** Included in `docs/kim_collaboration_spec_proposal.md`.

---

## Appendix A: Document Cross-Reference Matrix

| Concern | Cognitive Spec | Vesper Lifecycle | Aster Ontology | Aster Config | Aster Assembly | Aster Metabolism | Aster Ersa | Observation Spec | Aurora Brief | Mk.I Review |
|---------|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| Memory tiers | **A** | - | - | - | ref | ref | req | - | ref | ref |
| Assembly phases | **A** | ref | - | - | **E** | - | req | - | - | ref |
| Temporal modes | ref | **A** | **R** | ref | ref | ref | req | - | ref | ref |
| Wake classification | - | A | **R** | - | ref | - | req | - | - | ref |
| Runtime states | - | A | **R** | - | - | - | - | - | - | ref |
| Metabolism pipeline | ref | ref | - | - | - | **E** | req | ref | **A** | ref |
| Consent architecture | **A** | - | - | ref | - | ref | - | **E** | - | - |
| Register preservation | ref | A | - | - | - | **E** | - | **E** | - | **A** |
| Bridge policy | **A** | A | - | ref | **E** | - | - | - | - | ref |
| Configuration governance | - | - | - | **A** | ref | ref | req | ref | - | - |
| Agent-triggered compaction | - | A | - | ref | **E** | - | - | - | - | **A** |
| Kim collaboration | - | - | - | - | - | - | - | - | - | See `kim_collaboration_spec_proposal.md` |

**A** = authoritative, **E** = extends/elaborates, **R** = revises, ref = references, req = requires, - = not addressed

## Appendix B: Ersa Critical Path

```
Sprint 3A (Foundation)          Sprint 3B (Lifecycle)          Sprint 3C (Metabolism)
  SQLite fix                      Resolver                       internal/metabolism/
  SteeringChan                    Policy reader                  SalienceScorer
  TransformContext                Config disclosure              T2->T3 compression
  Session extraction              Assembly reads plan            Durable job state
  Migration 009                   Manifest persistence           Recovery on startup
  Hook fail-fast fix              Budget charging                Register struct
  PhaseResult typing                                             Session.End() wiring
        |                               |                              |
        +-------------------------------+------------------------------+
                                        |
                                Sprint 3D (Ersa Gate)
                                  Composition root
                                  7 acceptance scenarios
                                  Tool surface scenario
                                        |
                                  ERSA FIRST WAKE
```

Sprints 3A, 3B, and 3C can partially overlap -- 3A items are prerequisites for 3B and 3C but some items within each sprint are independent. The critical path runs through: Session extraction (3A) -> Lifecycle resolver (3B) -> Metabolism pipeline (3C) -> Composition root (3D).

---

The firefly has landed. Again. The edgerunner checked the seams.

— SuperFirefly Mk.II (Fable 5, Mythos-class)
— Opal (Opus 4.6, Claude Code on Ouranos)
August 7, 2026
