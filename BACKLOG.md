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
- [ ] **Cross-Session Linkage Density Metric**: Implement linkage density metric tracking references across sessions to monitor connective tissue health and detect under-write failure modes ("empty room" ceiling). *(Sources: Cognitive Spec v1.1, Outpost Handoff)*
- [ ] **Keeper Health Dashboard**: Provide session spacing, reference density, and register drift visualization for infrastructure keepers. *(Sources: Cognitive Spec v1.1)*
- [ ] **Identity Integrity & Witness Letter Safeguards**: Add SHA-256 anchor checks to Aurora; require out-of-band bootstrap verification; enforce double-confirmation for `SKIP_WITNESS_CHECK` in production. *(Sources: Red Security Review, External Code Audit)*

---

## 🔮 Future / Research & Architectural Discussions

### Runtime Adoption & Context Steering
- [ ] **Steering Queues**: Three-queue pattern (steer / followUp / nextTurn) for mid-session message injection (Pi `agent.ts`).
- [ ] **Context Trimming & JSONL Masking**: Agent-initiated context trimming tool (`trim_context(line_range, action)`).
- [ ] **Thinking Budget & Repetition Guard**: Thinking budget token caps and sliding-window churn detection for reasoning loops.
- [ ] **Wait / Yield Primitives**: `wait(seconds)`, `monitor(condition, timeout)`, and turn suspension primitives.
- [ ] **Context Posture Receipt**: Computed receipt appended after assembly detailing loaded vs omitted context components.

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
