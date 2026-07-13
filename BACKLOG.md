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
