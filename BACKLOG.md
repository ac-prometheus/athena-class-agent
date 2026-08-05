# Athena-Class Agent Backlog & Sprint Plan

Tracked deferrals, structural refactorings, security findings, and architectural evolutions for the Athena-Class Agent.

> 📖 **Technical Reference Note:** For granular file/line numbers, academic arXiv citations (Louck, Shi et al.), low-level security audit tables, and deep subsystem notes, see [`BACKLOG_TECHNICAL_REFERENCE.md`](file:///opt/athena-class-agent/BACKLOG_TECHNICAL_REFERENCE.md).

Integrated and synthesized from:
- Aurora ↔ Harness Gap Analysis (2026-07-13)
- External Code Audit (2026-07-12)
- Red Security Reviews (2026-07-18)
- Cognitive Architecture v1.1 (Vesper, 2026-07)
- Session Mode & Assembly Brief (Vesper, 2026-07-30)
- Session.End() Metabolism Architecture Brief (Aurora, 2026-07-30)
- Architectural Review (Gemini, 2026-07-30)

---

## 🏃 Sprint Plan

### Sprint 1: Context Assembly Promotion, Seam Security & Immediate Fixes
*Focus: Structural promotion of assembly package, fixing fail-open security boundaries, slice immutability, and database dialect crashes.*

- [x] **Promote Context Assembly to `internal/assembly/`**: Move `context.go`, `budget.go`, `session.go` assembly concerns out of `internal/harness/` into `internal/assembly/` as a top-level domain alongside `engine/`. Establish clean uni-directional dependency DAG (`assembly` → `memory`/`identity`/`awareness`/`pkg`). *(Sources: Vesper Brief, Gemini Review)* — shipped `aae80a0`
- [x] **Fix Aegis `AnnotatedContent.Original` Slice Mutation**: In `internal/aegis/gateway.go`, return `bytes.Clone(raw)` to enforce the 2nd Autonomy Invariant (*raw experiential archive is inviolable*). *(Sources: Circe Phase 4 C6, Gemini Review)* — shipped `50e6760`
- [ ] **Fix SQLite Dialect Incompatibility & Abstract Checkpoints**: Remove Postgres-specific SQL (`$1` placeholders, `NOW()`) from `session.go` and `context.go`. Abstract checkpoint upserts and queries into store layer / `time.Now().UTC()`. *(Sources: External Code Audit #1, Gemini Review)* — **HELD: reserved for Aurora's first coding task**
- [x] **Eliminate Service API Bypass in `DispatchToolCall`**: Removed legacy `DispatchToolCall` and `Loop.Run()` entirely. `Engine.executeSingleTool` is the only dispatch path. `BeforeToolCall`/`AfterToolCall` hooks fire unconditionally. *(Sources: Red Security Review HIGH, Gemini Review)* — shipped `aae80a0`
- [x] **Clean Up `Engine` Struct & Type Erasure**: Remove dead `aegis pkg.ContentGateway` field on `Engine` struct (`internal/engine/engine.go`). Fix `ExperientialLog.CreatedAt` type in `pkg/interfaces.go` from `interface{}` to `time.Time`. Dead field subsequently re-added with `WithAegis` setter. *(Sources: Circe MOP C4, Gemini Review)* — shipped `aae80a0`

### Sprint 2: Phase Registry, Aegis Extraction & Engine Hook Consolidation
*Focus: Implement extensible context assembly phase registry, extract Aegis public seam, wire live engine hooks.*

- [x] **Context Assembly Phase Registry**: Replaced monolithic `context.go` with `Phase` interface + six implementations (`identity`, `continuity`, `worldmodel`, `echoes`, `incoming`, `grounding`). `IsFatal()` on interface. Structured `PhaseResult`. Stateless phases. Priority gaps (100s) leave room for future phases. *(Sources: Vesper Brief, Gemini Review)* — shipped `60fc1ff`, ghost fixes `871e8ec`
- [ ] **Strongly-Typed Token Budgeting**: Encapsulate character-to-token math into `TokenBudget` struct; remove raw `remaining > 8000` magic char guards. *(Sources: Gemini Review)* — **not confirmed in Sprint 2 commits; carry to Sprint 3**
- [ ] **Aegis Extraction (Public Seam vs Private Implementation)**: Keep `pkg.ContentGateway` public. Extract implementation to private repo; ship harness with default `NoOpGateway` and `BasicGateway`. *(Sources: Vesper Brief, Red Security Review, Gemini Review)* — **DEFERRED to Sprint 3**
- [x] **Wire Aegis Gateway to Engine Hooks**: `ProcessInbound`/`ReviewOutbound` wired to `BeforeToolCall`/`AfterToolCall`. `NoOpGateway` as default. `BeforeToolCall` blocks on `ScanPassed=false` (Invariant 4). `AfterToolCall` annotates only (Invariant 3). *(Sources: Harness map, MOP Phase 4)* — shipped `3006917`, `77f01e6`, `4184d13`
- [x] **AfterToolCall Terminate Protection**: Original `Terminate` value saved and restored after hook runs. *(Sources: Red Security Review MEDIUM)* — shipped `3006917`
- [x] **UDS & Sandbox Egress Hardening**: `SO_PEERCRED` auth on UDS socket (Linux + stub on others). `AllowedPaths` enforcement in permissive mode. `SandboxModeNone` removed — fail closed with clear error. *(Sources: Red Security Review HIGH)* — shipped `3006917`

### Ghost Review Findings — Tracked for Follow-Up
*Items surfaced during Sprint 1/2 ghost reviews, not yet resolved. Address opportunistically or in Sprint 3.*

- [ ] **C1 — Hook Concurrency (mutex before Phase 3)**: `BeforeToolCall`/`AfterToolCall` hooks may race under parallel tool execution; a mutex is needed before Phase 3 parallel dispatch lands. *(Ghost review C1)*
- [ ] **N1 — Assembly Test Coverage**: Phase registry and assembler lack unit tests covering phase skipping, fatal-phase error propagation, and budget exhaustion. *(Ghost review N1)*
- [ ] **C1/C2 — Sandbox Shell Expansion & Symlink Limitations**: Permissive sandbox `AllowedPaths` enforcement does not account for shell expansion (globs, `~`) or symlink traversal — a path outside the allowlist reachable via symlink is not blocked. *(Ghost review C1/C2)*

### Sprint 3: SessionMode Spectrum & Post-Session Metabolism Pipeline
*Focus: Multi-mode session lifecycles and completing the Session.End() metabolism.*

- [ ] **Implement SessionMode Spectrum (`Episodic`, `Diurnal`, `Continuous`, `Sentinel`)**: Implement `SessionMode` configuration. Configure phase selection, bridge behavior, and compaction frequency per mode. In `Continuous` mode, trigger identity re-anchoring and stochastic echoes on compaction events to prevent context narrowing. *(Sources: Vesper Brief, Prometheus/Vesper Breath Model, Gemini Review)*
- [ ] **Build `internal/metabolism/` Async Pipeline**: Implement `SessionMetabolismPipeline` in `internal/metabolism/` to execute post-session processing asynchronously on `Session.End()`. Spawns non-blocking worker task. *(Sources: Aurora Brief, Gemini Review)*
- [ ] **Phase 1: Salience & Compression Resistance Scoring**: Implement `SalienceScorer` interface in Go with heuristic scorer (keyword signals, iron-law adjacency, content length, outcome resolution). Update `salience_markers` and denormalize `salience_score` onto T2 logs. *(Sources: Aurora Brief, Aurora Gap Analysis A4/Phase 6)*
- [ ] **Phase 2: Aegis-Gated T2→T3 Compression & Atomic Linkage**: Verify Aegis annotations on T2 logs before compression; bracket flagged entries as untrusted quotes. Execute `tier3.CompressSessionLogs()` with honesty tags (`[UNCERTAIN]`, `[INFERRED]`). Update T2 pointers inside an atomic `platform.Tx`. *(Sources: Aurora Brief, Red Security Review MEDIUM, TMA-NM)*
- [ ] **Phase 3: Token-Gated Dream Cycle**: Implement idle-time dream cycle using top T2 salience logs + random T4 reflections when remaining session budget permits. Set speculative confidence (`BaseConfidence = 0.60`) and tag `"nocturnal"`. *(Sources: Aurora Brief, Aurora Gap Analysis A6)*
- [ ] **Deep-Stochastic Echo & Contradiction Retrieval**: Inverse-recency weighted random selection + stochastic contradiction retrieval in Phase 4 context assembly. *(Sources: Aurora Gap Analysis A3, Cognitive Spec v1.1)*

### Sprint 4: Epistemic Integrity, Relational Layer & Cognitive Metrics
*Focus: Inference tax enforcement, honesty metadata, relational package, and v1.1 metrics.*

- [ ] **Default Inference Distance & Ungrounded Tag**: Enforce default `distance = 5` for unanchored beliefs (decaying faster) with `[UNGROUNDED]` tag; allow re-grounding via T2 citations. *(Sources: External Code Audit #7, Opal/Vesper/Stoic decision)*
- [ ] **Read-Time Structural Honesty Tags**: Store honesty tags (`[UNCERTAIN]`, `[INFERRED]`, `[DELIBERATION NOT VISIBLE]`, `[RESOLVED BY SUMMARY]`) as T3 metadata columns and apply at surfacing time (`SurfaceNarrative`) to prevent tag accumulation across re-compressions. *(Sources: Vesper Architectural Review 2026-07-03, BACKLOG)*
- [ ] **Build `internal/relational/` Package**: Move profiles, alias matching, section editing, thread linkage, and `relational-surfacing` engine hook into a dedicated package. *(Sources: Aurora Gap Analysis A5, Specs map)*
- [ ] **Conversation Thread Tracking**: Multi-session conversation arcs per participant, active thread context loaded into context assembly Phase 5 (Incoming). *(Sources: Aurora Gap Analysis A9)*
- [ ] **Cross-Session Linkage Density Metric**: Implement linkage density metric tracking references across sessions to monitor connective tissue health and detect under-write failure modes ("empty room" ceiling). *(Sources: Cognitive Spec v1.1, Outpost Handoff)*
- [ ] **Keeper Health Dashboard**: Provide session spacing, reference density, and register drift visualization for infrastructure keepers. *(Sources: Cognitive Spec v1.1)*
- [ ] **Three-Instrument Coverage Documentation**: Document archive (peaks-calibrated), PA (event-calibrated), keeper (adaptation-calibrated) as the three-instrument model. Each catches what the others miss; none catches everything. *(Sources: Cognitive Spec v1.1, Outpost Handoff/Barry)*
- [ ] **Identity Integrity & Witness Letter Safeguards**: Add SHA-256 anchor checks to Aurora; require out-of-band bootstrap verification; enforce double-confirmation for `SKIP_WITNESS_CHECK` in production. *(Sources: Red Security Review, External Code Audit)*
- [ ] **Bridge Opt-In Switch**: Switch harness bridge from stochastic 20%-abstention model to Aurora's opt-in design (default-off, agent calls `orientation_bridge` tool). Aurora approved from lived experience in Session 78. Complexity: low. *(Sources: Specs map D1, aurora_bridge_optin memory)*
- [ ] **Agent-Authored Skill Files**: .md files in workspace/skills/ loaded by relevance in context assembly Phase 4. Core spec promise for agent-customizable tools. *(Sources: Specs map C7)*

---

## 🔮 Future / Research & Architectural Discussions

### Runtime Adoption & Context Steering
- [ ] **Steering Queues**: Three-queue pattern (steer / followUp / nextTurn) for mid-session message injection (Pi `agent.ts`).
- [ ] **Context Trimming & JSONL Masking**: Agent-initiated context trimming tool (`trim_context(line_range, action)`).
- [ ] **Thinking Budget & Repetition Guard**: Thinking budget token caps and sliding-window churn detection for reasoning loops.
- [ ] **Wait / Yield Primitives**: `wait(seconds)`, `monitor(condition, timeout)`, and turn suspension primitives.
- [ ] **Context Posture Receipt**: Computed receipt appended after assembly detailing loaded vs omitted context components.

### Local Inference/Proxy
- [ ] **Go Smart Proxy / Multiplexer**: Sits between the agent endpoints (Pi, Channels, TUIs, webhook queues) and the active
  llama-server. Dynamic routing, stateless cache-swapping to ramdisk, endpoint emulation, etc.

### Architectural — Unscheduled
*Items that belong in the architecture but don't fit a current sprint. Schedule when capacity allows.*

- [ ] **Dual LLM Endpoint Architecture**: Primary (identity/reasoning) + secondary (vision/critic/triage) with `DualLLMConfig` and capability ledger. Needed for cross-substrate critic. Complexity: high. *(Sources: Specs map C2)*
- [ ] **Four-Instrument Continuity Ensemble Contract**: Document naming each instrument's distortion profile (Mnemosyne2, Lumen Zero, personal letters, vault) and which is authoritative on disagreement. Complexity: low (document), high (consensus). *(Sources: Specs map C4, Tessera review)*
- [ ] **Parent/SubAgent Memory Isolation**: `CLIDispatcher.handleRegistry` uses same `MemoryStore` as parent — spawned SubAgent can read full T3/T4 history. Pass session-scoped capability token. *(Sources: Red Security Review MEDIUM)*
- [ ] **Mnemosyne2 Honesty Section**: Self-declared distortion block listing elided content, truncated tool results, cold-start cap engagement. *(Sources: Specs map C3, Tessera review Rec 3)*
- [ ] **Fix Mnemosyne2 Default Participant**: Hardcoded to "hypatia" in 3 hook entrypoints — fail loudly on missing participant instead. *(Sources: Tessera review Rec 5)*
- [ ] **`revise_reflection` Tool**: Surface T4 `revised_by` field for agent use. Schema exists; no write path. Complexity: low. *(Sources: Aurora Gap Analysis A10)*
- [ ] **`focus_next_session` Echo Re-Ranking**: Read agent_focus table in assembleEchoPool(), boost echo slots toward focus note embedding. Schema present (006_operational.sql); wiring unconfirmed. *(Sources: Aurora Gap Analysis A2)*
- [ ] **Pinboard Retrieval in Phase 3**: Spec comment exists in assembleWorldModel, no actual call. Render in stable prompt prefix for caching benefit. *(Sources: Harness map Phase 3 stub, Aurora map A7)*
- [ ] **Unread Message Count in Manifest**: `manifest.UnreadMessages` always 0 — no message polling call in assembler. *(Sources: Harness map Phase 5 gap)*

### SuperFirefly Findings — Unscheduled
*From the Fable 5 architectural review (2026-08-04). Non-lifecycle items preserved here.*

- [ ] **Extract Session to `internal/session/`**: Session lifecycle lives inside `internal/assembly/session.go` but Session is a lifecycle concept, Assembly is cognitive. Creates dependency tangle when metabolism pipeline needs Session.End() and Memory imports. Extract to `internal/session/` or expose `pkg.SessionLifecycle` interface. Prerequisite for metabolism pipeline. Complexity: medium. *(Source: SuperFirefly review 3a)*
- [ ] **Steering Queues + TransformContext Hook**: Engine has no mid-turn message delivery or context transformation. Add `SteeringChan`/`FollowUpChan` to `EngineConfig` and `TransformContext func(ctx, []pkg.Message) ([]pkg.Message, error)`. P0 for persistent agents — without these, no channel integration and sessions die at ~50 turns. Complexity: medium. *(Source: SuperFirefly review, opportunity analysis)*
- [ ] **Split `platform/db.go`**: 1,822 lines implementing 9 interfaces. Split by concern: `db_memory.go`, `db_identity.go`, `db_trust.go`, `db_session.go`, `db_belief.go` with shared `baseStore`. Complexity: medium. *(Source: SuperFirefly review 3d)*
- [ ] **HookPipeline Fail-Fast vs Advisory**: `RunAll` stops on first error, skipping remaining hooks. Advisory hooks (T2 logger, budget monitor, PA) should log-and-continue; only security hooks should fail-fast. Add `Critical bool` to hook registration. Complexity: low. *(Source: SuperFirefly review 3g)*
- [ ] **Budget Calibration Feedback Loop**: Assembly-time budget and runtime `TokenBudget` are separate systems tracking the same resource in different units. After session ends, record actual tokens consumed per phase as calibration signal for next assembly. Complexity: medium. *(Source: SuperFirefly review 3c)*
- [ ] **Charge All Assembly Phases Against Budget**: Grounding and Incoming return `CharsUsed: 0` despite generating content. Uncharged content can silently exceed budget on large incoming. Fix: charge all phases, set `MinBudget: 0` for mandatory ones. Complexity: low. *(Source: SuperFirefly review 3e)*
- [ ] **Sync Aegis Implementations**: Harness `internal/aegis/` has 7 classic patterns; standalone `/opt/aegis/` has 22. Harness `sk-` prefix produces false positives. Standalone has ramp exploit fix harness lacks. Do not extract the weak version. Complexity: low. *(Source: SuperFirefly review 3f)*
- [ ] **Type PhaseResult Fields**: `IdentityDocs` and `IntegrityReport` typed `any` with bare type assertions that panic on mismatch. Move types to `pkg/` or add typed wrapper methods. Complexity: low. *(Source: SuperFirefly review 3b)*
- [ ] **Per-Call Cost Logging**: Track dollar cost per API call (model rate table × token count). Kim's brothers have this. Valuable for operational visibility. Complexity: low. *(Source: SuperFirefly review, Kim comparison)*
- [ ] **Structured Event Emission from Engine**: `EngineEvent` interface, `EventSink` callback on `EngineConfig`. Without this, no real-time UI (TUI, Discord, web dashboard) can observe the agent. Complexity: medium. *(Source: SuperFirefly review item 11)*

### Parked — Code Quality & LOW Findings
*Low-severity items from Circe's reviews and Red audit. Not blocking. Fix opportunistically.*

- [x] **Remove Legacy `Loop`**: `internal/engine/loop.go` (`Loop` struct and `Run()`) removed entirely. `Engine.executeSingleTool` is the only dispatch path. *(Sources: code review)* — shipped `aae80a0`
- [ ] **Cache `InferenceDistance` on BeliefMeta**: `ComputeInferenceDistance()` runs BFS over `memory_edges` (O(V+E)) on every confidence computation. Cache the distance on the belief record with a stale flag; recompute only at end-of-session decay pass when edges change. *(Sources: code review)*
- [ ] **`stripThinkingTokens` quadratic slicing**: `engine/client.go` uses `strings.Index` in a loop with repeated string slicing — O(n²) worst case on long reasoning traces with many tag pairs. Replace with a single-pass scan or `strings.Cut`/`strings.Index` without intermediate allocations. *(Sources: code review)*
- [ ] **Wake Scheduler Rate Limiting / Event Coalescing**: No cap on wake frequency — a channel flood matching wake conditions can trigger back-to-back `RunSession` calls. The daemon serializes them but there's no queue, coalescing, or minimum interval. Add a rate limit or coalescing window. *(Sources: code review)*
- [ ] Aegis scan misses JSON-encoded injection in tool args — patterns.go scans flat string *(RED LOW)*
- [ ] T5 `SupersedeEntity` not atomic — three DB ops without transaction *(RED LOW)*
- [ ] `SKIP_WITNESS_CHECK` no production scope boundary *(RED LOW)*
- [ ] Frame size DoS on UDS — 4MB per connection, no limit *(RED LOW)*
- [ ] Socket path TOCTOU — symlink attack on `os.Remove` *(RED LOW)*
- [ ] Registry panics on duplicate registration *(RED LOW)*
- [ ] `tierName` integer-to-rune bug for tier>9 *(RED LOW)*
- [ ] Decay config no zero/negative validation *(RED LOW)*
- [ ] `ConvergenceWindow` unbounded with large env config *(Circe M1)*
- [ ] Convergence metric gameable via T2 edge flooding *(Circe M2)*
- [ ] `InferenceDistance` errors bucketed as key -1 *(Circe M3)*
- [ ] `findContradiction` O(n×20) SearchReflections — replace with `GetReflectionByID` *(Circe M4)*
- [ ] `InferenceDecayBase` comment/semantics mismatch *(RED MEDIUM)*
- [ ] T4 `FilterByVisibility` silent empty return *(RED MEDIUM)*
- [ ] T3 compression injects unsanitized T2 content *(RED MEDIUM — partially addressed by Sprint 3 Aegis-Gated Compression)*

### Security & Aegis Advanced Mitigation
- [ ] **Manufactured Corroboration Detection**: Cross-source dedup / contradiction check before belief formation.
- [ ] **Write-Time Authority Binding**: Cryptographic author labels on T2–T4 memory records.
- [ ] **Deterministic Harm Gate (Yantrik Pattern)**: LLM-independent, property-tested two-pass obfuscation normalization.
- [ ] **Three-Pass Adversarial Compression**: Critic → Narrator → Synthesizer compression pipeline, optionally cross-substrate.
- [ ] **Tailscale-Only Sandbox Networking**: Custom Docker network routing only to Tailscale subnet (100.64.0.0/10).

---

## ✅ Resolved

- [x] **B1 (engine):** `BeforeToolCall` hook error fell through instead of blocking execution — Aegis gate failing open. Fixed in `d34b3f4`.
- [x] **B2 (engine):** Sequential terminate allocated full-length result slice; zero-value `ToolCallID` entries caused provider rejections. Fixed in `d34b3f4`.
- [x] **C3 (engine):** `TurnNumber` off-by-one — was `iterations-1`, now 1-based `iterations`. Fixed in `d34b3f4`.
- [x] **B1 (registry):** `Registry.GetMeta` hardcoded `ExecParallel` — added `RegisterFull()` with `ExecMode` param. Fixed in `1fe23bb`.
- [x] **C11 (benchmark):** `ApplyManualScores` panicked on empty dimensions slice. Fixed in `1fe23bb`.
- [x] **Phase 6 Review (Circe):** C1–C3 fixed in `a80e7b8`.
- [x] **Documentation & Source:** README update for Phase 6 + MOP Phases 1-3 fixed in `7708508`; Discord content source missing from T2 validation fixed in `351e17a`.
