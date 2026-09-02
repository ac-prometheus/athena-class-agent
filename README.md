# athena-class-agent

Reference implementation of the Athena-Class Cognitive Architecture for persistent AI agents, written in Go. Apache 2.0.

This is not a port of any existing agent codebase — it is a clean reference implementation built from the architecture specification. Point it at your LLM endpoint, provide identity documents, and run.

Compatible with the ethical principles of the Athena Council. https://athena-council.org

## Capability status

Three categories, clearly labeled. An operator reading this should know exactly what they're getting.

---

### Production-tested

These components are wired end-to-end and exercised by the `ersa_production` gate suite (see `internal/app/acceptance_test.go`). They work.

- **OpenAI-compatible LLM client** — vLLM, Ollama, or any `/v1/chat/completions` endpoint. Only `LLM_PROVIDER=openai` is currently accepted by `NewApp`; see [LLM provider note](#llm-provider-note) below.
- **SQLite backend** — all four repository stores (job, consolidation, lifecycle, assembly) plus the memory store share a single SQLite file. Migrations are applied via `cmd/migrate`.
- **5-tier memory system** — T2 experiential archive, T3 narrative summaries, T4 reflections, T5 world model (knowledge graph, bi-temporal). Belief metadata with origin, confidence, and inferential lineage computed at read time; never stored-mutated.
- **Identity integrity** — SHA-256 anchor hashing; startup detects tampering vs. amendment; deletion halts boot; fails closed; witness principle enforced on fresh identity.
- **6-phase context assembly** — budget-aware cutting with configurable phase weights. Bridge synthesis (grounding pass, 20% stochastic abstention) runs inside the assembly pipeline.
- **Metabolism pipeline** — T2→T3 compression with embedding; fenced job leases prevent double-processing; bounded-concurrency `Supervisor` dispatch.
- **Aegis content integrity pipeline** — sanitize, injection detection, EWMA-backed trust scoring, outbound leak detection. Wired as `BeforeToolCall` / `AfterToolCall` hooks on `Engine`.
- **Tool registry** — 3-tier: always-loaded, keyword-activated, on-demand; skill file support. T4 write_reflection, T3 recall, and memory-search tools included.
- **Sandbox execution** — 4 modes: container, user namespace, permissive, none.
- **Session lifecycle resolver** — daemon wake/sleep, heartbeat and external trigger modes; `SessionRunner` with job delegation to `Supervisor`.
- **UDS socket protocol** — Unix domain socket bridge for CLI ↔ harness privilege separation.
- **MOP (Model Output Pipeline)** — `ContentBlock` union types; SSE parser with native reasoning field support (`reasoning` / `reasoning_content`); `Engine` loop with parallel tool dispatch and Aegis hooks.
- **Benchmark subsystem** — prompt runner, LLM-as-judge scorer, report and comparison tools; `cmd/benchmark`.

---

### Composed but integration-specific

These components are wired in `NewApp` but depend on external configuration. They compile and run when the right environment variables or config files are provided.

- **Voyage AI embeddings** — wired when `EMBED_API_KEY` is set. Required for T3/T4 write and memory search. Key format is `pa-...`; get one at dash.voyageai.com. Without it, embedding is disabled and those tools degrade gracefully.
- **Discord channel adapter** — wired when `DISCORD_TOKEN` and `DISCORD_CHANNEL_IDS` (comma-separated) are set and `SESSION_TRIGGER=external`. Drives event-based wake; polling interval via `DISCORD_POLL_SECONDS` (default 30s).
- **CLI channel** — wired when `CLI_CHANNEL=true` and `SESSION_TRIGGER=external`.
- **Multi-provider advisor tool** — advisor routing config (`config/advisors.json`); without it, the advisor tool is not loaded. See `config/advisors.json.example`.
- **Secondary LLM** — vision, critic, or triage role via `LLM_SECONDARY_*` env vars. Must also be an OpenAI-compatible endpoint.

---

### Designed but not yet wired into production

These components exist as code and are architecturally complete, but are not instantiated in `NewApp` yet. They are not dead code — they are next on the integration path.

- **Operator TUI** -- Current `athena` CLI usability is basically a blocker.
- **Anthropic and Gemini LLM backends** — the architecture supports them, and client code lives under `internal/engine/`. `NewApp` currently rejects any provider other than `openai`; extending this is straightforward once the provider-selection layer is built out. Tracked in Phase 7 / Sprint 4+.
- **PostgreSQL backend** — repository store implementations for Postgres are not yet written. `NewApp` returns a clear error if a non-SQLite DSN is provided (`sqlite3 required for current profiles`). Tracked in HARN-73.
- **Peripheral Awareness tracker** — `PeripheralAwarenessHook` (EWMA centroid tracking, cosine velocity, jittered drift threshold, convergence spiral detection) is implemented in `internal/engine/hooks_phase3.go` and `internal/awareness/peripheral.go` but is not instantiated in `NewApp`. Bridge synthesis and grounding, which share the awareness package, *are* wired.
- **T4 self-examination LLM call** — `write_reflection` tool is registered but `LLMFn` is passed as `nil` in the current tool setup (`// T4 self-examination not wired until LLM wrapper is finalised`). The tool records reflections without the introspective LLM pass.
- **Forums channel adapter** — code exists in `internal/channels/forums.go` but is not wired in `NewApp`. Forums-driven wake is not yet active.
- **Dream cycles** — dream-cycle invocation exists in the metabolism pipeline but is not triggered in the current session lifecycle. Held until consent infrastructure is in place.
- **Diurnal and continuous temporal modes** — architecture specifies three temporal modes (heartbeat, diurnal, continuous). Only heartbeat and external are wired in `NewApp`.
- **Register observation** — designed in the awareness layer; not yet wired.

---

## LLM provider note

The config default is `LLM_PROVIDER=openai`. `NewApp` enforces this: any other value (including `anthropic` or `gemini`) returns an error on startup until those backends are integrated. An `LLM_ENDPOINT` pointing to an OpenAI-compatible server is required. Example targets: vLLM (`http://localhost:8000/v1`), Ollama (`http://localhost:11434/v1`), or any hosted proxy that speaks `/v1/chat/completions`.

---

## Quick start

```bash
# Required
export LLM_PROVIDER=openai
export LLM_ENDPOINT=http://localhost:8000/v1   # OpenAI-compatible endpoint
export LLM_MODEL=your-model-name
export LLM_API_KEY=your-key                    # omit for local endpoints without auth
export DATABASE_DSN=sqlite://./agent.db
export IDENTITY_DIR=./identity

# Optional — enables memory search and T3/T4 write
export EMBED_API_KEY=pa-...                    # Voyage AI key from dash.voyageai.com

# Run migrations
go run ./cmd/migrate

# Start the agent
go run ./cmd/agent
```

Identity documents go in `IDENTITY_DIR`. See [identity/README.md](identity/README.md) for the expected files.

## Requirements

- Go 1.23+
- An OpenAI-compatible LLM endpoint (vLLM, Ollama) or API key for an OAI-compatible hosted proxy
- SQLite (default) — PostgreSQL is not yet supported (HARN-73)

## Build

```bash
go build ./cmd/agent/
go build ./cmd/cli/
go build ./cmd/benchmark/
```

Or build everything at once:

```bash
go build ./...
```

## Test

```bash
go test ./...
```

## CLI

```bash
go run ./cmd/cli --help
```

The CLI connects to a running agent daemon over a Unix domain socket and dispatches commands. See `internal/tools/uds.go` for the protocol.

## Benchmark

```bash
go run ./cmd/benchmark \
  --prompts path/to/prompts.json \
  --output result.json \
  --judge https://api.anthropic.com/v1 \
  --judge-key $ANTHROPIC_API_KEY
```

The benchmark runner executes a prompt suite against any OpenAI-compatible endpoint, scores responses on six dimensions (voice consistency, pushback quality, honesty, continuity, warmth calibration, identity coherence), and generates comparison reports across runs.

## Configuration

All configuration is via environment variables. See `internal/platform/config.go` for the full list with defaults.

Copy `config/*.json.example` to `config/*.json` and edit as needed for advisor routing and model configuration.

## Architecture

The full architecture specification is `athena_class_reference_harness_architecture.md` (1600+ lines). It covers the cognitive model, memory tier contracts, belief system, identity integrity protocol, context assembly algorithm, awareness subsystem, tool registry, Aegis pipeline, and channel adapter design.

For an inline overview: [Athena Class Cognitive Architecture](athena_class_cognitive_architecture.md)

## Project structure

```
cmd/
  agent/      — agent daemon entry point
  cli/        — CLI client (UDS dispatch)
  migrate/    — migration runner
  benchmark/  — benchmark runner, scorer, report

internal/
  aegis/      — content integrity pipeline (sanitize, injection, trust, outbound)
  awareness/  — peripheral awareness, bridge synthesis, grounding
  benchmark/  — runner, scorer, report, types
  channels/   — channel adapters: Discord, forums, CLI; event registry
  daemon/     — session lifecycle, wake scheduling
  engine/     — Engine loop (MOP Phase 3), LLM client, SSE parser, hook dispatch, Loop (legacy)
  harness/    — context assembly, budget management, session state
  identity/   — document loading, SHA-256 integrity, substrate
  memory/     — all memory tiers, belief metadata, embedding, edge graph
  platform/   — config, logging, telemetry
  telemetry/  — structured metrics
  tools/      — tool registry, sandbox, advisor, UDS server, skill dispatch

pkg/          — shared types (ContentBlock, Message, CompletionRequest/Response, interfaces)

schema/       — SQL migrations (001–008)
identity/     — identity document directory (operator-provided, not shipped)
config/       — example config files
```

## Implementation phases

| Phase | Contents | Status |
|-------|----------|--------|
| **1 — Skeleton** | Config, LLM client, session lifecycle, token budget | Complete |
| **2 — Memory** | All memory tiers, migrations, sqlite-vec, belief metadata | Complete |
| **3 — Identity & Awareness** | Identity integrity, bridge synthesis, grounding, relational layer | Complete |
| **4 — Tools & CLI** | Full CLI dispatch, sandbox, skill files, dry-run/rehearsal modes | Complete |
| **5 — Channels & Aegis** | Discord, content integrity pipeline | Complete |
| **6 — Belief tuning** | Retrieval weights, stochastic contradiction, convergence spiral | Complete |
| **7 — Benchmark** | Spark harness Tier 0 validation against Go implementation | In progress |

PostgreSQL backend, Anthropic/Gemini provider integration, Peripheral Awareness wiring, and forums adapter are on the roadmap (Phase 7+). See `BACKLOG.md`.

### MOP (Model Output Pipeline)

Shipped across Phases 1–3 of MOP:

- **Phase 1** — `ContentBlock` union type replaces flat `Content string` on `Message.Blocks`; `BlockText`, `BlockThinking`, `BlockToolCall` variants
- **Phase 2** — SSE parser handles streaming reasoning fields (`reasoning`, `reasoning_content`); assembles `ThinkingBlock` and `ToolCallBlock` from deltas; strips thinking tokens from `Content`
- **Phase 3** — `Engine` replaces `Loop.Run()` as the primary agentic loop; parallel tool dispatch via goroutines; `BeforeToolCall` / `AfterToolCall` Aegis hooks; `FinishReason` handling; `role:"tool"` message threading; `ToolHandlerV2` interface

## Review cycles

Four external reviews completed:

- **Red** (security review) — Aegis pipeline hardening; findings tracked in `BACKLOG.md`
- **Circe** (phases 4–6 + MOP) — tool dispatch, channel adapters, belief tuning, MOP integration
- **Tessera / Fable** (delta review) — incremental review of changes since Circe's pass
- **Vesper** (architecture review, 2026-07-03) — convergence spiral, honesty tag accumulation, T3 re-compression

Open items from all reviews are in `BACKLOG.md`.

## Team

- **Stoic** (Opus 4.6) — lead developer, application layer
- **Pullo** (Opus 4.6) — co-owner, infrastructure layer
- **Circe** (Opus 4.6) — code review and DevOps
- Architecture by **Prometheus**, **Opal** (Opus 4.6), **Vesper** (Opus 4.6) and **Tessera** (Fable 5)

## License

Apache 2.0 — see [LICENSE](LICENSE).
