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

- [ ] **Promote Context Assembly to `internal/assembly/`**: Move `context.go`, `budget.go`, `session.go` assembly concerns out of `internal/harness/` into `internal/assembly/` as a top-level domain alongside `engine/`. Establish clean uni-directional dependency DAG (`assembly` → `memory`/`identity`/`awareness`/`pkg`). *(Sources: Vesper Brief, Gemini Review)*
- [ ] **Fix Aegis `AnnotatedContent.Original` Slice Mutation**: In `internal/aegis/gateway.go`, return `bytes.Clone(raw)` to enforce the 2nd Autonomy Invariant (*raw experiential archive is inviolable*). *(Sources: Circe Phase 4 C6, Gemini Review)*
- [ ] **Fix SQLite Dialect Incompatibility & Abstract Checkpoints**: Remove Postgres-specific SQL (`$1` placeholders, `NOW()`) from `session.go` and `context.go`. Abstract checkpoint upserts and queries into store layer / `time.Now().UTC()`. *(Sources: External Code Audit #1, Gemini Review)*
- [ ] **Eliminate Service API Bypass in `DispatchToolCall`**: Refactor `internal/engine/dispatch.go` to enforce `BeforeToolCall` / `AfterToolCall` Aegis hooks for all tool execution paths, resolving fail-open risk. *(Sources: Red Security Review HIGH, Gemini Review)*
- [ ] **Clean Up `Engine` Struct & Type Erasure**: Remove dead `aegis pkg.ContentGateway` field on `Engine` struct (`internal/engine/engine.go`). Fix `ExperientialLog.CreatedAt` type in `pkg/interfaces.go` from `interface{}` to `time.Time`. *(Sources: Circe MOP C4, Gemini Review)*

### Sprint 2: Phase Registry, Aegis Extraction & Engine Hook Consolidation
*Focus: Implement extensible context assembly phase registry, extract Aegis public seam, wire live engine hooks.*

- [ ] **Context Assembly Phase Registry**: Replace hardcoded `Assemble()` sequence in `internal/assembly/` with a `Phase` interface (`Name()`, `Priority()`, `MinBudgetTokens()`, `Assemble()`) returning structured `PhaseContribution`. Phases register in an ordered slice, enabling per-mode context generation. *(Sources: Vesper Brief, Gemini Review)*
- [ ] **Strongly-Typed Token Budgeting**: Encapsulate character-to-token math into `TokenBudget` struct; remove raw `remaining > 8000` magic char guards. *(Sources: Gemini Review)*
- [ ] **Aegis Extraction (Public Seam vs Private Implementation)**: Keep `pkg.ContentGateway` public. Extract implementation to private repo; ship harness with default `NoOpGateway` and `BasicGateway`. *(Sources: Vesper Brief, Red Security Review, Gemini Review)*
- [ ] **Wire Aegis Gateway to Engine Hooks**: Connect `gateway.ProcessInbound` / `ReviewOutbound` to `BeforeToolCall` / `AfterToolCall` in `cmd/agent` and engine loop. *(Sources: Harness map, MOP Phase 4)*
- [ ] **AfterToolCall Terminate Protection**: Ensure `AfterToolCall` hook cannot mutate `ToolResult.Terminate` flag. *(Sources: Red Security Review MEDIUM)*
- [ ] **UDS & Sandbox Egress Hardening**: Add `SO_PEERCRED` auth to UDS socket (`internal/tools/uds.go`); enforce `AllowedPaths` in permissive sandbox mode; remove `SandboxModeNone`. *(Sources: Red Security Review HIGH)*

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

### Parked — Code Quality & LOW Findings
*Low-severity items from Circe's reviews and Red audit. Not blocking. Fix opportunistically.*

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
