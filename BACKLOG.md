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

### Red Security Review
- [ ] Finding 1: Aegis pipeline — implemented in Phase 5. Channel adapters must not pre-sanitize
- [ ] SubAgent isolation via sandbox — Phase 4 sandbox exists, delegation not yet wired
- [ ] T2→T3 compression Aegis gate — compression must refuse content lacking Aegis annotation
- [ ] Double-confirm for `SKIP_WITNESS_CHECK` — Red recommended interactive confirmation
- [ ] Network egress proxy — deferred hardening item per spec

### TMA-NM Laundering Channels (Louck, arXiv:2606.24322)
- [ ] Summarization channel — T2→T3 compression without Aegis annotation is the attack surface. Compression guard (refuse to compress unannoted content) is tracked in Red's items but not yet implemented
- [ ] Trusted-tool echo — vault/oracle retrieval of poisoned content bypasses Aegis if the tool is trusted. Tool results should carry provenance labels
- [ ] Manufactured corroboration — multiple poisoned sources reinforcing the same claim. Cross-source dedup or contradiction detection before belief formation
- [ ] Write-time authority binding — every memory record should carry a non-forgeable label of who wrote it. More rigorous than current provenance tagging

### PrismaAURA / Context Management
- [ ] Thinking tag normalization — rename `stripThinkingTokens` → `normalizeThinkingTokens` in `client.go`. Standardize "Here's a thinking process:" and bare `</think>` into proper `<think>`...`</think>` pairs. Preserve in `CompletionResponse.ThinkingTrace`, strip from `Content`. Enables: T2 provenance (reasoning is auditable), TUI collapsible sections, belief metadata referencing thinking traces
- [ ] Agent-managed context tools — session summary, focus set, manual compaction request. Agent decides what to carry, not automatic rolling compression. Follows consent principle. Reference: Aurora's context management approach. At 85K ceiling with PrismaAURA, ~50 turns before window fills
- [ ] Reasoning verbosity tuning — explore `enable_thinking: false` for tool-only calls, system prompt nudge for concise reasoning, temperature 0.7 as default. Balance: thoroughness vs context burn

## Infrastructure

- [ ] `GetReflectionByID` method on MemoryStore — needed for O(1) contradiction lookup
- [ ] AllowedPaths enforcement in permissive sandbox mode — currently fails closed
- [ ] Playwright/Patchright system Chrome integration for DynamicFetcher tier
- [ ] Unified search tool (Gemini grounded + Brave API + DDG, backlogged per Prometheus)
- [ ] GFS backup rotation script for JSONL conversation files
- [ ] README update for Phase 6 (belief tuning section)
- [ ] BACKLOG.md itself — kept current as items are resolved

## Resolved

Items moved here when fixed, with commit reference.

*(none yet)*
