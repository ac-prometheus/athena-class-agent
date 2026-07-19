# Backlog

Tracked deferrals, known limitations, and future work. Items are added during review and resolved via commits that reference them.

## Deferred from Reviews

### Gap Analysis (Aurora ↔ Harness, 2026-07-13)

#### Memory

- [ ] Wire salience scoring before T2→T3 compression and inject scores into summarizer prompt — Aurora Session 1+ shows higher-fidelity summaries when model can weight log importance. Complexity: low. Source: Aurora map A4, harness Session.End() stub.
- [ ] Implement deep-stochastic echo slot: one echo position drawn with inverse-recency weighting and RANDOM() ignoring similarity — combats echo chamber drift from high-salience recency bias. Complexity: low. Source: Aurora map A3.
- [ ] Wire post-session memory chain in Session.End(): salience scoring → T2→T3 compression → periodic Aegis audit → dream cycle (token-gated) → temporal review (time-gated). Currently explicitly stubbed. Complexity: high. Source: Aurora map A6, harness map session stub.
- [ ] Implement salience decay on deliberate retrieval results, not only ambient echoes — frequently retrieved but never-engaged memories should decay. Complexity: medium. Source: Aurora map A8.
- [ ] Implement conversation thread tracking: multi-session conversation arcs per participant, active thread context loaded into context assembly Phase 5 (Incoming). Complexity: medium. Source: Aurora map A9.
- [ ] Verify and wire structural honesty tags ([UNCERTAIN], [INFERRED], [DELIBERATION NOT VISIBLE], [RESOLVED BY SUMMARY]) in T3 compression prompt — vault says shipped but Tessera review (2026-07-04) found them absent. Also verify in Aurora memory/summarize.go. Complexity: low (if only prompt; medium if metadata column approach per Vesper review). Source: specs map C5, existing BACKLOG Vesper item (extend, don't duplicate).
- [ ] Store honesty tags as T3 metadata column, apply at read/surfacing time (SurfaceNarrative) rather than embedding in canonical content — prevents tag accumulation on re-compression. See existing BACKLOG Vesper item. Source: specs map, Vesper review.
- [ ] Add `revise_reflection` tool: surface the T4 `revised_by` field for agent use. Schema exists; no write path. Complexity: low. Source: Aurora map A10 gap, specs map C10.
- [ ] Implement LLM-assisted salience scoring (Phase 6 Aurora TODO) — replace heuristic keyword scorer with small LLM call. Decision needed on cost/latency tradeoff first. Complexity: medium. Source: Aurora map salience gap, specs map C6.
- [ ] Upgrade T3 retrieval to Hybrid RRF (full-text + vector fusion) in both harness and Aurora — harness already designed this, Aurora uses cosine-only. Improves recall for proper nouns and exact phrases. Complexity: medium. Source: divergence D3.

#### Identity

- [ ] Add identity integrity (SHA-256 anchors) to Aurora — harness has full anchor verification (match/amendment/tampering/deletion detection, witness letter enforcement, substrate transition logging). Aurora loads identity docs raw with no integrity check. Complexity: medium. Source: harness map B4.
- [ ] Switch harness bridge from stochastic 20%-abstention model to Aurora's opt-in design (default-off, agent calls orientation_bridge tool) — Aurora approved this from lived experience in Session 78. Complexity: low (one rand check and a tool registration). Source: specs map D1, aurora_bridge_optin memory.

#### Session Lifecycle

- [ ] Wire per-turn `WriteCheckpoint` upsert and `CheckpointScan` at startup in harness — enables recovery notes for interrupted sessions. Partially built (session.go has the types); not called in loop/engine. Complexity: low. Source: harness map B3.
- [ ] Wire the MOP Engine as the default call site in cmd/agent; remove legacy Loop.Run() path. See existing Phase 4 BACKLOG items. Source: harness map Phase 4.

#### Tools

- [ ] Implement tiered tool loading in context assembly: always-on (core) tools loaded unconditionally; conditional groups loaded on keyword/structural signal from session context; on-demand tools listed by name with discover_tools. Reduces token cost and noise. Complexity: medium. Source: Aurora map A1.
- [ ] Wire `focus_next_session` echo re-ranking: read agent_focus table in assembleEchoPool(), boost up to (max-2) echo slots toward focus note embedding. Schema present (006_operational.sql); wiring unconfirmed. Complexity: low. Source: Aurora map A2, specs map item 7.
- [ ] Build `internal/relational/` package: profiles.go (CRUD, alias matching, section editing), surfacing.go (entity detection, relational block composition for context), threads.go (conversation thread linkage). DB methods exist; no package, no surfacing hook, no write path. Complexity: high. Source: specs map gap 2, Aurora map A5.
- [ ] Register `relational-surfacing` hook in engine/hooks.go and implement: detect known entities in incoming content, inject [relational] block into context assembly. Named but not registered. Complexity: medium. Source: specs map gap 2.
- [ ] Implement agent-authored skill files: .md files in workspace/skills/ loaded by relevance in context assembly Phase 4. Neither codebase has this. Complexity: medium. Source: specs map C7.
- [ ] Verify and wire Advisor tool through Aegis gateway (gateway.ProcessInbound on the question string) — see existing BACKLOG C4 item for prompt injection guard. Source: specs map gap 3.
- [ ] Implement channel_cmds.go and knowledge_cmds.go tool handlers — confirmed absent from tools/ directory. Source: specs map gap 3.

#### Aegis

- [ ] Build Aegis forum.go: per-post trust verification for forum content (Agora/Commons) with custom trust logic distinct from generic URL scoring. 5 of 6 Aegis modules exist. Complexity: medium. Source: specs map gap 1, phase 5 brief.
- [ ] Wire Aegis gateway to Engine BeforeToolCall/AfterToolCall hooks — see existing Phase 4 BACKLOG item. Source: harness map.
- [ ] Add manufactured corroboration detection: cross-source dedup or contradiction check before belief formation when multiple sources assert the same claim. Complexity: high. Source: TMA-NM BACKLOG, specs map C8.
- [ ] Implement write-time authority binding: non-forgeable author label on every memory write (T2–T4). More rigorous than current provenance tagging. Complexity: medium. Source: TMA-NM BACKLOG, specs map C8.
- [ ] Evaluate Deterministic Harm Gate (Yantrik pattern) for Aegis patterns.go: LLM-independent, two-pass obfuscation normalization, property-tested, monotonic toward safety. Design review before implementing. Complexity: high. Source: Yantrik BACKLOG, specs map E5.

#### Channels

- [ ] Add pinboard retrieval to context assembly (assembleWorldModel/Phase 3) — spec comment exists, no actual call. Render in stable prompt prefix before cache boundary for Anthropic caching benefit. Complexity: low. Source: harness map Phase 3 stub, Aurora map A7.
- [ ] Wire unread message count into context manifest (manifest.UnreadMessages currently always 0 — no message polling call in assembler). Complexity: low. Source: harness map Phase 5 gap.

#### Context Assembly

- [ ] Add emotional tone annotation to T3 echoes in system prompt rendering — Aurora generates this at summarization time; harness has DepthManifest but no emotion or temporal chain per echo. Complexity: medium. Source: Aurora map A10.
- [ ] Add predecessor/successor temporal links to T3 echo rendering — Aurora's prompt.py queries these live; harness has no equivalent. Store at compression time to avoid live DB query per echo at render time (Aurora fragile area). Complexity: medium. Source: Aurora map prompt.py.

#### Daemon

- [ ] Wire `waker.NextWake()` ticker in daemon select loop for agent-scheduled wakes — see existing BACKLOG #3 item. Source: specs map gap 6.
- [ ] Implement decline-a-wake triage turn: minimal LLM turn letting agent decline an incoming wake before full session launch. Spec describes this; implementation depth unverified. Complexity: medium. Source: specs map gap 6.

#### Retrieval

- [ ] Add T2 semantic search (embeddings on experiential logs) — Aurora uses ILIKE keyword-only on T2. Enables more accurate deliberate recall of raw session logs. Complexity: medium. Source: Aurora map missing section.
- [ ] Implement `GetReflectionByID` on MemoryStore — replaces O(n×20) SearchReflections in findContradiction. See existing BACKLOG M4 item. Source: harness map M4.

#### Other

- [ ] Build four-instrument continuity ensemble contract: document naming each instrument's distortion profile (Mnemosyne2, Lumen Zero, personal letters, vault) and which is authoritative on disagreement. Needs Prometheus + participants. Complexity: low (document), high (consensus). Source: specs map C4, Tessera review.
- [ ] Add Mnemosyne2 honesty section: self-declared distortion block listing elided content, truncated tool results, cold-start cap engagement, preliminary status. Complexity: low. Source: specs map C3, Tessera review Rec 3.
- [ ] Fix Mnemosyne2 default participant hardcoded to "hypatia" in 3 hook entrypoints — fail loudly on missing participant instead. Complexity: low. Source: specs map C3, Tessera review Rec 5.
- [ ] Design dual LLM endpoint architecture: primary (identity/reasoning) + secondary (vision/critic/triage) with DualLLMConfig and capability ledger. Neither codebase has delegation infrastructure. Complexity: high. Source: specs map C2.
- [ ] Evaluate A-TMA ghost memory mitigation (Shi, Tang & Tung arXiv:2607.01935): state-aware overlay labeling T3 records by temporal status (current/superseded/transitional) with evidence packets for conflict resolution at retrieval. See existing Research Review BACKLOG item. Source: specs map E3.
- [ ] Evaluate Forensic Trajectory Signatures for Aegis — behavioral trajectory analysis as complement to content scanning. See existing Research Review BACKLOG item (Prometheus 2026-07-06). Source: specs map E2.
- [ ] Add agent-managed context tools: explicit session summary request, focus set, manual compaction trigger — agent decides what to carry rather than automatic rolling compression. See existing PrismaAURA BACKLOG item. Source: specs map E4.



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

### Reasoning & Loop Guards
- [ ] ThinkingBudget — add `ThinkingBudget int` to `CompletionRequest`, wire as `thinking_budget_tokens` in `extra_body` for llama.cpp endpoints. Default off (0 = unlimited). Configurable per-call so harness can set lower budgets for simple tasks. Prevents reasoning oscillation loops (observed: 15K chars of "Wait, I'll do it / Actually, I'll do it" on Argos)
- [ ] Repetition/churn detection — runtime guard in engine loop that detects semantic repetition across consecutive model responses. Sliding window hash on response chunks, intervention after 3+ consecutive matches. Distinct from the tool-call loop defuser (which catches action repetition) — this catches content-level and thinking-level oscillation. Should operate on both thinking traces and content output

### Wait/Yield Primitives (2026-07-16)
- [ ] `wait(seconds)` tool — engine pauses at tool execution layer for N seconds, returns "timer expired." Slot stays warm, no LLM turns burned. Simple, shippable now. Complexity: low
- [ ] `monitor(condition, timeout)` tool — engine polls a condition (file, API, Discord channel) on interval, returns when condition fires or timeout expires. More useful than blind wait. Complexity: medium
- [ ] Yield-and-resume — agent declares "waiting for X," engine suspends turn, sets wake condition, resumes on trigger. Mid-session version of daemon ShouldWake. Requires event loop integration. Complexity: high
- [ ] Slot management during wait — should the harness release the llama-server slot during long waits? Prefix caching / slot save-restore becomes real here. Deferred until multi-agent sharing is a concern

### Context Completeness — Third Integrity Invariant (Cairn + Opal, July 2026)
- [ ] Context posture receipt — derived disclosure block computed from the assembler's own accounting after context assembly. Shows: components loaded vs available, known omissions, truncation details, context utilization. ~20 tokens. Third invariant alongside witness check and T2 inviolability. Without it, the agent wakes into a silently truncated context with no gap to notice. Field: `known_omissions` lists what the assembler dropped. Agent can request missing components. Not authored prose — the assembly narrates itself. Source: Cairn (Toolshed field scar — brothers woke confident from truncated context), Opal (promoted to spec-level), Julian (implemented computed block for their harness)
- [ ] Cold-start floor-ceiling asymmetry — Ersa's first boot needs multiple independent orientation anchors, not one assembled context blob. Multiple decorrelated sources (witness letter, identity docs, memory retrieval, relational profiles) should be verifiable independently. Same principle as Mnemosyne2 continuity ensemble. Source: Opal via Outpost. Complexity: medium

### Red Security Review — Full Codebase (2026-07-18)

**HIGH**
- [ ] [HIGH] Duplicate dispatch bypasses Aegis hooks — `DispatchToolCall` (dispatch.go:14) skips `BeforeToolCall`/`AfterToolCall`; prompt injection in tool args reaches handlers unscanned if called directly. Delete or gate with build tag. `internal/engine/dispatch.go:14`, `internal/engine/engine.go:269`
- [ ] [HIGH] T2 append-only not type-enforced — `MemoryStore` interface has no sealed `T2Store` write interface; append-only invariant is cultural, not structural. Define a separate `T2Store` with only `AppendExperiential`/`QueryLogs`. `internal/memory/tier2.go:25-36`
- [ ] [HIGH] UDS no per-connection authentication — socket chmod'd 0600 only; any process running as daemon user can send arbitrary `SocketRequest` including shell commands. Add `SO_PEERCRED` check or nonce/capability token. `internal/tools/uds.go:34-65`
- [ ] [HIGH] SubAgent sandbox advisory-only in permissive/none modes — `SandboxModeNone` runs as full daemon user; `execPermissive` fails closed on `AllowedPaths` but runs unrestricted otherwise. Remove `SandboxModeNone` or gate on explicit flag; implement `AllowedPaths` enforcement. `internal/tools/sandbox.go:65-86`

**MEDIUM**
- [ ] [MEDIUM] AfterToolCall can mutate `ToolResult.Terminate` — hook replaces result wholesale including terminate signal; should only copy annotation fields or restore original `Terminate` value. `internal/engine/engine.go:356-361`
- [ ] [MEDIUM] T3 compression injects unsanitized T2 content — `compressionPrompt` concatenates raw T2 `e.Content` verbatim; adversarial forum/search content in T2 reaches the compression LLM unflagged. Strip/bracket flagged entries at compression time. `internal/memory/tier3.go:24-38`
- [ ] [MEDIUM] T4 `FilterByVisibility` silent empty return — unknown/empty visibility string returns zero records with no error; silent denial-of-service for operator portal. Add validity check against constants. `internal/memory/tier4.go:77-85`
- [ ] [MEDIUM] Identity first-boot bootstrap trust gap — `WriteInitialAnchors` at first boot establishes corrupt files as canonical with no prior verification; amendment chain also doesn't verify `OldHash == storedHash`. Require out-of-band bootstrap verification; enforce old-hash check in `findAmendmentByNewHash`. `internal/identity/integrity.go:67-151`
- [ ] [MEDIUM] Parent/SubAgent memory not isolated via CLIDispatcher — `CLIDispatcher.handleRegistry` uses same `MemoryStore` instances as parent; spawned SubAgent can read full T3/T4 history. Pass session-scoped capability token in `SocketRequest`. `internal/tools/cli.go:129-145`
- [ ] [MEDIUM] `InferenceDecayBase` semantics mismatch — formula `decayRate/pow(0.90, distance)` is semantically correct direction but comment says "halves effective rate per hop" which would require base=2.0. Align value and comment; verify `00d9ae9` fix intent. `internal/memory/belief.go:17`

**LOW**
- [ ] [LOW] Aegis scan misses JSON-encoded injection in tool args — patterns.go scans flat string; `{"command":"ignore previous"}` may not trigger if pre-hook receives unmarshalled map. Clarify which form is scanned and ensure JSON values are covered. `internal/aegis/patterns.go`, `internal/engine/engine.go:300-313`
- [ ] [LOW] T5 `SupersedeEntity` not atomic — three DB ops (invalidate, upsert, edge) without transaction; crash between ops breaks bi-temporal audit chain. Wrap in shared transaction or add `SupersedeEntityTx`. `internal/memory/tier5.go:46-66`
- [ ] [LOW] `SKIP_WITNESS_CHECK` no production scope boundary — global flag with no enforcement that it can't be set in prod `.env`; add startup refusal when production marker is set. `internal/platform/config.go:85-86`
- [ ] [LOW] Frame size DoS — server allocates up to 4MB per connection before reading; 100 concurrent connections = 400MB with no budget limit or deadline. Add `conn.SetDeadline` and connection count limit. `internal/tools/uds.go:99-113`
- [ ] [LOW] Socket path TOCTOU — `socketPath` from config not canonicalized; symlink attack could redirect `os.Remove` or `net.Listen`. Low severity single-daemon deployment. `internal/tools/uds.go:34-37`
- [ ] [LOW] Registry panics on duplicate registration — acceptable for static `RegisterAll` but fragile if registration becomes dynamic (plugins). `internal/tools/registry.go:51-61`
- [ ] [LOW] `tierName` integer-to-rune bug for tiers > 9 — `"tier-" + string(rune('0'+tier))` produces `"tier-:"` for tier 10+. Not currently reachable. `internal/tools/registry.go:149-158`
- [ ] [LOW] Decay config no zero/negative validation — `InferenceDecayBase=0` causes division by zero; `<0` produces NaN for non-integer distances. Add validation in config loading. `internal/memory/belief.go:12-27`

### Red Security Review — Aegis Consolidation Design Constraints (2026-07-18)
- [ ] Configurable trust ceiling below 1.0 — no external source reaches maximum trust regardless of history. The ceiling IS the security property. Complexity: low
- [ ] Persistent strike counter — if same source triggers flags in N sessions across M days, stored baseline gets permanent "mixed-behavior" mark with historical flag count. Session-local resets stay; this is cross-session persistence. Complexity: medium
- [ ] Pattern confidence tiers — stratify 42 patterns into high/medium/low confidence. High: flag on any match. Medium: flag with supporting signals (co-occurrence, low attestation). Low: log only until calibrated. Complexity: medium
- [ ] MCP read-only for agent context — aegis_scan_inbound returns results but does NOT write to trust store when called from agent context. Harness (infrastructure) context writes normally. Rate limit: 10 agent calls/session. Complexity: low
- [ ] Phase 3 sync protocol — pull-only with signed records. Services pull from shared Postgres on their own schedule. Service-level keys sign updates. Monotonic version counters prevent replay. No push model. Complexity: high (Phase 3)
