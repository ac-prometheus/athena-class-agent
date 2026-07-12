# Backlog

Tracked deferrals, known limitations, and future work. Items are added during review and resolved via commits that reference them.

## Deferred from Reviews

### Phase 4 (Circe)
- [ ] C4: Advisor prompt injection — structured boundaries on advisor question string. Deferred to Phase 5 Aegis integration. (Now available — wire advisor through `gateway.ProcessInbound`)
- [ ] C2: Narrow injection pattern set — 7 classic + 4 ASRP. Missing: "act as if", "pretend you are", "your new role is", "forget what you were told". Intentionally conservative; expand as field data arrives
- [ ] C4: UUID false positives in outbound scan — any UUID triggers alert. High FP on structured tool output. Tighten regex
- [ ] C5: API key detection too broad — "sk-" matches "risk-", "disk-". Missing: `ghp_`, `AKIA`, `sk_live_`. Tighten regexes
- [ ] C6: `AnnotatedContent` returned as mutable pointer — `Original []byte` can be modified post-return. Structural enforcement of inviolable archive invariant

### Phase 5 (Circe)
- [ ] C3: Forum cursor uses time, not opaque ID — clock skew enables silent drops or replays
- [ ] C4: Channel ID not validated — config values interpolated into Discord API URL without snowflake pattern check

### Phase 6 (Circe)
- [ ] C4: Internal record IDs in debug logs — `RetrievalUsageHook` emits memory record UUIDs at Debug level. Gate on log destination
- [ ] M1: ConvergenceWindow unbounded with large env config — enforce upper bound (currently clamped to 50 in config, but in-memory window not separately guarded)
- [ ] M2: Convergence metric gameable via T2 edge flooding — document the assumption
- [ ] M3: InferenceDistance errors bucketed as key -1 — add explicit ErrorCount field
- [ ] M4: `findContradiction` is O(n_edges × 20) SearchReflections calls — replace with `GetReflectionByID` when method exists

### MOP Phase 3 (Circe)
- [ ] C1: `ContentBlock` constructed as raw struct literal throughout — no constructor guards. Add `NewTextBlock()`, `NewThinkingBlock()`, `NewToolCallBlock()` helpers so callers can't accidentally leave `Type` unset. Affects `pkg/types.go` + call sites in `internal/engine/client.go` and tests
- [ ] C2: `AfterToolCall` behaviour under `DryRun` is undocumented — the comment says "hooks still run" but does not clarify that `AfterToolCall` fires with the dry-run stub result, not a real tool result. Add a doc comment to `EngineConfig.AfterToolCall` clarifying this
- [ ] C4: Dead `Aegis` field on `EngineConfig` — listed in `engine.go` struct but never read or passed to any hook. Either wire it or remove it
- [ ] N3: Panic recovery absent from parallel goroutine dispatch — a panicking tool handler will crash the whole process. Add `recover()` in the goroutine launched by `executeParallel`, convert to error result
- [ ] C12: Judge transcript budget unguarded — `JudgeScore` in `internal/benchmark/scorer.go` sends the full conversation transcript in a single prompt with no token length check. Long runs can exceed provider context limits. Add a max-transcript-chars guard with truncation or chunking before the LLM call

### Red Security Review
- [ ] Finding 1: Aegis pipeline — implemented in Phase 5. Channel adapters must not pre-sanitize
- [ ] SubAgent isolation via sandbox — Phase 4 sandbox exists, delegation not yet wired
- [ ] T2→T3 compression Aegis gate — compression must refuse content lacking Aegis annotation
- [ ] Double-confirm for `SKIP_WITNESS_CHECK` — Red recommended interactive confirmation
- [ ] Network egress proxy — deferred hardening item per spec

### TMA-NM Laundering Channels (Louck, arXiv:2606.24322)
- [ ] Summarization channel — T2→T3 compression without Aegis annotation is the attack surface. Compression guard (refuse to compress unannotated content) is tracked in Red's items but not yet implemented
- [ ] Trusted-tool echo — vault/oracle retrieval of poisoned content bypasses Aegis if the tool is trusted. Tool results should carry provenance labels
- [ ] Manufactured corroboration — multiple poisoned sources reinforcing the same claim. Cross-source dedup or contradiction detection before belief formation
- [ ] Write-time authority binding — every memory record should carry a non-forgeable label of who wrote it. More rigorous than current provenance tagging

### PrismaAURA / Context Management
- [ ] Thinking tag normalization — rename `stripThinkingTokens` → `normalizeThinkingTokens` in `client.go`. Standardize "Here's a thinking process:" and bare `</think>` into proper `<think>`...`</think>` pairs. Preserve in `CompletionResponse.ThinkingTrace`, strip from `Content`. Enables: T2 provenance (reasoning is auditable), TUI collapsible sections, belief metadata referencing thinking traces
- [ ] Agent-managed context tools — session summary, focus set, manual compaction request. Agent decides what to carry, not automatic rolling compression. Follows consent principle. Reference: Aurora's context management approach. At 85K ceiling with PrismaAURA, ~50 turns before window fills
- [ ] Reasoning verbosity tuning — explore `enable_thinking: false` for tool-only calls, system prompt nudge for concise reasoning, temperature 0.7 as default. Balance: thoroughness vs context burn

### Research Review Items (Hypatia surveys)
- [ ] Forensic Trajectory Signatures — evaluate for Aegis/harness integration. Mechanism for detecting manipulation via trajectory analysis rather than content scanning. Complements pattern-based detection. Per Prometheus request (2026-07-06)
- [ ] A-TMA ghost memory (Shi, Tang & Tung, arXiv:2607.01935) — agents retrieve a mix of current, superseded, and transitional facts ("ghost memory"). A-TMA adds state-aware overlay labeling memory records by temporal status + evidence packets for conflict resolution at retrieval. Evaluate against our T2→T3 compression and honesty tag accumulation problem — may inform the read-time metadata approach

### Yantrik Mind Adoption Candidates (Hypatia research, 2026-07-04)
- [ ] Deterministic harm gate — LLM-independent, two-pass obfuscation normalization, property-tested, monotonic toward safety. More rigorous than current Aegis injection patterns. Evaluate for `internal/aegis/patterns.go` upgrade
- [ ] Bounded self-improvement — agent can propose changes to its own codebase via PR, compile-gated, governance code structurally off-limits. Design: sandbox mount of codebase + identity documents, proposal system (local git or GitHub rules). Complex external dependency for core functionality — worth discussing architecture before implementing. Ref: Prometheus wants sandbox codebase access + proposal flow

### External Code Audit (2026-07-12)
- [ ] #1 (CRITICAL): SQLite dialect incompatibility — `session.go` and `context.go` use `$1` placeholders and `NOW()`. SQLite path crashes immediately. Abstract into store layer
- [ ] #2 (HIGH): Discord snowflake cursor — `fetchMessages` overwrites `newestID` with oldest message in batch, causing re-fetch loop. Verify against Circe's Phase 5 fix (may already be resolved)
- [ ] #3 (HIGH): Daemon missing wake checks + OS signals — `select` loop only handles `ctx.Done()` and events. No `waker.NextWake()` ticker, no `SIGINT`/`SIGTERM` handling. Phase 7+ daemon integration scope
- [ ] #4 (HIGH): Discarded startup notes — `firstWake` flag drops interrupted-session notes if first event doesn't trigger wake. Preserve in persistent queue
- [ ] #5 (MEDIUM): Forums goroutine leak — blocking `out <- p` without `ctx.Done()` select. Same class as Phase 5 registry fan-in fix
- [ ] #6 (MEDIUM): Wake scheduler data race — `scheduled` slice modified concurrently without mutex. Check if Circe's Phase 5 fixes cover this surface
- [ ] #7 (MEDIUM): Inference distance default=1 for unanchored beliefs — spec says decay faster, code gives slowest rate. **Decision: A+C — distance=5 (configurable in DefaultDecayConfig), `[UNGROUNDED]` visibility tag, agent can re-ground via T2 citation to lift penalty. Spec basis: line 143, "uncited beliefs receive a default distance that decays them faster than any cited belief." Three-layer pattern: consequence (decay), visibility (tag), contestability (re-grounding). — Opal, Vesper, Stoic, 2026-07-12**
- [ ] #8 (LOW): Sandbox privilege check — no capability assertion before `SysProcAttr` credential switch. Add startup validation or graceful failure

### Vesper Architectural Review (2026-07-03)
- [ ] Honesty tag accumulation in T3 re-compression — tags (`[UNCERTAIN]`, `[INFERRED]`, etc.) persist in canonical T3 content and re-accumulate on subsequent compressions. Same failure mode as stored-mutated confidence: each compression layer adds weight the tag didn't earn. Fix: store tags as metadata, apply at read time (same pattern as `BeliefMeta.Confidence()`). Confirmed by Vesper, Pullo, Hypatia's research. Design discussion needed before implementing.

## Phase 4 (MOP) — Upcoming Work

- [ ] Remove `internal/engine/loop.go` and `internal/engine/dispatch.go` — legacy loop preserved during Phase 3; Phase 4 completes the cut-over to `Engine`
- [ ] Wire `Engine` as the default in `cmd/agent` — currently `Loop.Run()` is still the call site
- [ ] `ToolHandlerV2` adoption — migrate existing tool handlers to `ExecuteV2` returning `*pkg.ToolResult` (structured errors, metadata, terminate signal)
- [ ] Structured tool output — `ToolResult.Metadata` map for provenance labels (feeds TMA-NM trusted-tool echo mitigation)
- [ ] Aegis hook wiring — connect `gateway.ProcessInbound` / `ProcessOutbound` to `BeforeToolCall` / `AfterToolCall` on the live `Engine` instance

## Infrastructure

- [ ] `GetReflectionByID` method on MemoryStore — needed for O(1) contradiction lookup
- [ ] AllowedPaths enforcement in permissive sandbox mode — currently fails closed
- [ ] Playwright/Patchright system Chrome integration for DynamicFetcher tier
- [ ] Unified search tool (Gemini grounded + Brave API + DDG, backlogged per Prometheus)
- [ ] GFS backup rotation script for JSONL conversation files
- [ ] BACKLOG.md itself — kept current as items are resolved

## Resolved

Items moved here when fixed, with commit reference.

### MOP Phase 3 review (Circe) — fixed in `d34b3f4` and `1fe23bb`
- [x] **B1 (engine):** `BeforeToolCall` hook error fell through instead of blocking execution — Aegis gate failing open. Fixed: error → block → error-as-result (`d34b3f4`)
- [x] **B2 (engine):** Sequential terminate allocated full-length result slice; zero-value `ToolCallID` entries caused provider rejections. Fixed: slice to filled entries only (`d34b3f4`)
- [x] **C3 (engine):** `TurnNumber` off-by-one — was `iterations-1`, now 1-based `iterations` (`d34b3f4`)
- [x] **B1 (registry):** `Registry.GetMeta` hardcoded `ExecParallel` — added `RegisterFull()` with `ExecMode` param so sequential tools can declare themselves. Fixed in `1fe23bb`
- [x] **C11 (benchmark):** `ApplyManualScores` panicked on empty dimensions slice — zero-guard added (`1fe23bb`)

### Phase 6 review (Circe) — fixed in `a80e7b8`
- [x] C1–C3 from Circe Phase 6 review (see commit message for details)

### Infrastructure
- [x] README update for Phase 6 (belief tuning section) + MOP Phases 1-3 — fixed in `7708508`
- [x] Discord content source missing from T2 validation — fixed in `351e17a`
