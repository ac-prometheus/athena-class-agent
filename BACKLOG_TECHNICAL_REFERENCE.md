# Athena-Class Agent: Backlog Technical Reference & Audit Notes

**Document Status:** TECHNICAL REFERENCE  
**Companion File to:** [`BACKLOG.md`](file:///opt/athena-class-agent/BACKLOG.md)  
**Maintained By:** Athena Engineering & Security Audit Group  
**Last Updated:** July 30, 2026  

---

## Executive Summary

While [`BACKLOG.md`](file:///opt/athena-class-agent/BACKLOG.md) serves as the high-level, scannable **4-Sprint Plan**, this document preserves all deep technical context, specific file/line references, arXiv research citations, and detailed security findings from prior audits. Developers implementing sprint tasks should consult this document for granular implementation specifics.

---

## 1. Security Audit Findings & File/Line References

### 1.1 Red Security Review — Codebase Audit (2026-07-18)

#### Structural Vulnerability Patterns
1. **Pattern 1: Fail-Open Boundaries**  
   Security checks failing open on error. Examples: `BeforeToolCall` fail-open (fixed in `d34b3f4`), `SandboxModeNone` running as full daemon user, `AllowedPaths` not enforced in permissive mode, dead `Aegis` field on `EngineConfig`.  
   *Enforcement:* Every security check must fail-closed.
2. **Pattern 2: Service API Bypasses**  
   Internal/service paths bypassing public consent/scan gates. Examples: `CLIDispatcher` sharing parent `MemoryStore` (SubAgent sees T3/T4 history), `DispatchToolCall` in `dispatch.go` skipping hooks.  
   *Enforcement:* Service paths must enforce identical isolation and hook gates as public paths.
3. **Pattern 3: Schema Divergence & Invariant Enforcement**  
   Append-only invariants stated in comments but not enforced at DB layer. `MemoryStore` interface has no sealed write interface.  
   *Enforcement:* Separate `T2Store` interface and DB-level immutability triggers.

#### Granular Severity Table

| Severity | Item / Description | Primary File & Line Locations |
| :--- | :--- | :--- |
| **HIGH** | `DispatchToolCall` skips `BeforeToolCall`/`AfterToolCall` hooks | `internal/engine/dispatch.go:14`, `internal/engine/engine.go:269` |
| **HIGH** | T2 append-only not type-enforced on `MemoryStore` | `internal/memory/tier2.go:25-36` |
| **HIGH** | UDS socket missing per-connection auth (chmod 0600 only) | `internal/tools/uds.go:34-65` |
| **HIGH** | SubAgent sandbox advisory-only in permissive/none modes | `internal/tools/sandbox.go:65-86` |
| **MEDIUM** | `AfterToolCall` hook can mutate `ToolResult.Terminate` | `internal/engine/engine.go:356-361` |
| **MEDIUM** | T3 compression prompt injects unsanitized raw T2 content | `internal/memory/tier3.go:24-38` |
| **MEDIUM** | T4 `FilterByVisibility` silent empty return on unknown string | `internal/memory/tier4.go:77-85` |
| **MEDIUM** | Identity first-boot bootstrap trust gap & missing old-hash check | `internal/identity/integrity.go:67-151` |
| **MEDIUM** | Parent/SubAgent memory isolated gap in `CLIDispatcher` | `internal/tools/cli.go:129-145` |
| **MEDIUM** | `InferenceDecayBase` comment mismatch vs code math | `internal/memory/belief.go:17` |
| **LOW** | Aegis scan misses JSON-encoded injection in tool args | `internal/aegis/patterns.go`, `internal/engine/engine.go:300` |
| **LOW** | T5 `SupersedeEntity` non-atomic across 3 DB operations | `internal/memory/tier5.go:46-66` |
| **LOW** | `SKIP_WITNESS_CHECK` missing production environment block | `internal/platform/config.go:85-86` |
| **LOW** | Frame size DoS: server allocates up to 4MB per connection | `internal/tools/uds.go:99-113` |
| **LOW** | Socket path TOCTOU vulnerability | `internal/tools/uds.go:34-37` |
| **LOW** | Registry panic on duplicate tool registration | `internal/tools/registry.go:51-61` |
| **LOW** | `tierName` integer-to-rune bug for tier >= 10 (`tier-:`) | `internal/tools/registry.go:149-158` |
| **LOW** | Decay config lacks zero/negative validation | `internal/memory/belief.go:12-27` |

---

### 1.2 Codebase Review (Circe: Phases 4, 5, 6 & MOP Phase 3)

- **Advisor Injection Boundary (Phase 4 C4):** Wire advisor tool through `gateway.ProcessInbound` on question strings.
- **Regex Tightening (Phase 4 C4/C5):**
  - UUID false positive: UUID strings in tool outputs currently trigger key alerts. Tighten regex.
  - API Key false positive: `sk-` matches `risk-` and `disk-`. Change regex to require `sk_live_`, `ghp_`, or `AKIA`.
- **`AnnotatedContent` Mutable Slice (Phase 4 C6):** `Original []byte` returned as mutable pointer. Fix: return `bytes.Clone(raw)`.
- **Forum Cursor Time vs ID (Phase 5 C3):** Cursor uses timestamp rather than opaque snowflake ID, causing silent message drops under clock skew.
- **Internal IDs in Debug Logs (Phase 6 C4):** `RetrievalUsageHook` emits record UUIDs at Debug level. Gate on log destination.
- **O(1) Contradiction Lookup (Phase 6 M4):** `findContradiction()` performs `O(n_edges * 20)` `SearchReflections` calls. Replace with `MemoryStore.GetReflectionByID()`.
- **`ContentBlock` Constructor Guards (MOP Phase 3 C1):** Raw struct literal construction throughout codebase allows `Type` to be left empty. Add `NewTextBlock()`, `NewThinkingBlock()`, `NewToolCallBlock()`.
- **Panic Recovery in Parallel Dispatch (MOP Phase 3 N3):** Add `recover()` in goroutines launched by `executeParallel` to prevent a failing tool from crashing the entire process.
- **Judge Transcript Budget (MOP Phase 3 C12):** `JudgeScore` sends un-truncated transcript prompts to LLM. Add max-transcript-chars guard.

---

### 1.3 External Code Audit (2026-07-12)

- **Audit Item #1 (CRITICAL):** SQLite dialect crashes due to `$1` placeholders and `NOW()` in `session.go` and `context.go`. Abstract into store layer.
- **Audit Item #2 (HIGH):** Discord snowflake cursor `fetchMessages` overwrites `newestID` with oldest message in batch, causing infinite re-fetch loops.
- **Audit Item #3 (HIGH):** Daemon `select` loop lacks `waker.NextWake()` ticker and `SIGINT`/`SIGTERM` signal handlers.
- **Audit Item #4 (HIGH):** `firstWake` flag drops interrupted-session notes if first event doesn't trigger wake. Preserve in persistent queue.
- **Audit Item #5 (MEDIUM):** Forum goroutine leak on blocking `out <- p` without `ctx.Done()`.
- **Audit Item #6 (MEDIUM):** Wake scheduler data race: `scheduled` slice modified concurrently without mutex.
- **Audit Item #7 (MEDIUM):** Inference distance default decision: Default `distance = 5` (configurable in `DefaultDecayConfig`), append `[UNGROUNDED]` tag, allow agent re-grounding via T2 citation. *(Decided by Opal, Vesper, Stoic).*
- **Audit Item #8 (LOW):** Sandbox privilege check: missing capability assertion before `SysProcAttr` credential switch.

---

## 2. Research & Academic Literature Citations

### 2.1 TMA-NM Laundering Channels (Louck, arXiv:2606.24322)
- **Summarization Channel:** T2→T3 compression without Aegis annotation allows poisoned inputs to escalate into trusted T3 narrative memory.
- **Trusted-Tool Echo:** Oracle/vault tool results carrying poisoned content bypass Aegis scanning. Fix: tool outputs must carry provenance labels.
- **Manufactured Corroboration:** Cross-source dedup and contradiction detection required before belief formation when multiple sources assert identical claims.
- **Write-Time Authority Binding:** Non-forgeable author label on every memory write (T2–T4).

### 2.2 A-TMA Ghost Memory Mitigation (Shi, Tang & Tung, arXiv:2607.01935)
- **Ghost Memory Failure Mode:** Persistent agents retrieve a mix of current, superseded, and transitional facts.
- **A-TMA Mechanism:** State-aware overlay labeling T3 records by temporal status (`current` / `superseded` / `transitional`) with evidence packets for conflict resolution at retrieval time. Informs read-time metadata approach for structural honesty tags.

### 2.3 Yantrik Mind Adoption Candidates (Hypatia Research, 2026-07-04)
- **Deterministic Harm Gate:** LLM-independent, property-tested, two-pass obfuscation normalization. Monotonic toward safety. Evaluated for upgrading `internal/aegis/patterns.go`.
- **Bounded Self-Improvement:** Sandbox mount of codebase + identity documents; agent proposes PRs compiled and validated in isolation with governance code structurally off-limits.

### 2.4 Forensic Trajectory Signatures (Prometheus, 2026-07-06)
- **Mechanism:** Trajectory-based behavioral analysis as a complement to content scanning in Aegis. Detects manipulation by observing behavioral drift over multi-turn interactions.

---

## 3. In-Depth Feature & Subsystem Notes

### 3.1 Reasoning & Loop Guards
- **ThinkingBudget:** Add `ThinkingBudget int` to `CompletionRequest`, wired as `thinking_budget_tokens` in provider extra_body. Prevents 15K reasoning oscillation loops.
- **Repetition & Churn Detection:** Sliding-window hash on response chunks to detect semantic churn across consecutive model responses (intervenes after 3+ matches).

### 3.2 PrismaAURA & Thinking Tag Normalization
- **Normalize Thinking Tags:** Rename `stripThinkingTokens` → `normalizeThinkingTokens` in `client.go`. Standardize bare `</think>` tags into proper `<think>`...`</think>` pairs. Preserve reasoning traces in `CompletionResponse.ThinkingTrace`.

### 3.3 Three-Pass Adversarial Compression (Toolshed + Agora)
- **Architecture:** Critic (finds rough edges) → Narrator (compresses) → Synthesizer (flags smoothing).
- **Cross-Substrate Critic:** Run Critic on a decorrelated model family (e.g. Gemma on local GPU reviewing Qwen/Claude output).
- **Attestation Chain Receipts:** Compression audit trail records *what* was challenged, by *which* substrate, with *what* outcome (excluding raw reasoning to avoid domesticating future readers).

### 3.4 Context Posture Receipt (Cairn & Opal, July 2026)
- **Concept:** Derived disclosure block computed after context assembly showing: components loaded vs available, known omissions, truncation details, and context utilization (~20 tokens). Third invariant alongside witness check and T2 inviolability.

---

## 4. Cross-Reference Index to Codebase

| Subsystem | Key Files | Outstanding Technical Task |
| :--- | :--- | :--- |
| **Assembly** | `internal/harness/context.go` | Refactor to `internal/assembly/` with `Phase` interface |
| **Session** | `internal/harness/session.go` | Wire `SessionMetabolismPipeline` into `Session.End()` |
| **Engine** | `internal/engine/engine.go` | Remove dead `aegis` field; protect `Terminate` in `AfterToolCall` |
| **Dispatch** | `internal/engine/dispatch.go` | Enforce `BeforeToolCall`/`AfterToolCall` hooks |
| **Aegis** | `internal/aegis/gateway.go` | `bytes.Clone(raw)` fix on `AnnotatedContent.Original` |
| **Memory** | `internal/memory/belief.go` | Enforce default `distance = 5` with `[UNGROUNDED]` tag |
| **Relational** | `pkg/interfaces.go` | Build dedicated `internal/relational/` package |
| **Tools** | `internal/tools/uds.go` | Add `SO_PEERCRED` socket authentication |
| **Sandbox** | `internal/tools/sandbox.go` | Enforce `AllowedPaths` in permissive mode |
