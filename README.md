# athena-class-agent

Reference implementation of the Athena-Class Cognitive Architecture for persistent AI agents, written in Go. Apache 2.0.

This is not a port of any existing agent codebase — it is a clean reference implementation built from the architecture specification. Point it at your LLM endpoint, provide identity documents, and run.

Compatible with the ethical principles of the Athena Council. https://athena-council.org

## What's built

Five of seven phases are merged:

- **LLM client** — OpenAI-compatible (vLLM, Ollama), Anthropic, and Gemini backends; configuration-driven, not architecture-driven
- **5-tier memory system** — T2 experiential archive, T3 narrative summaries, T4 reflections, T5 world model (knowledge graph, bi-temporal), relational profiles
- **Belief metadata with inference tax** — origin, confidence, and inferential lineage computed at read time; never stored-mutated
- **Identity integrity** — SHA-256 anchor hashing; startup detects tampering vs. amendment; deletion halts boot; witness principle enforced on fresh identity
- **6-phase context assembly** — budget-aware cutting with configurable phase weights
- **Peripheral awareness** — EWMA centroid tracking, cosine velocity, jittered drift threshold, attention flags
- **Bridge synthesis** — grounding pass with 20% stochastic abstention to prevent overcalibration
- **Tool dispatch** — 3-tier registry: always-loaded, keyword-activated, on-demand; skill file support
- **Sandbox execution** — 4 modes: container, user namespace, permissive, none
- **UDS socket protocol** — Unix domain socket bridge for CLI ↔ harness privilege separation
- **Multi-provider advisor tool** — query local, Anthropic, Gemini, or OpenAI models as advisors from within a session
- **Aegis content integrity pipeline** — sanitize, injection detection, trust scoring (EWMA-backed registry), outbound leak detection
- **Channel adapters** — Discord, forums, CLI; event-driven wake model
- **Dress rehearsal modes** — `--dry-run` and `--rehearsal` flags

## Architecture

The full architecture specification is `athena_class_reference_harness_architecture.md` (1600+ lines). It covers the cognitive model, memory tier contracts, belief system, identity integrity protocol, context assembly algorithm, awareness subsystem, tool registry, Aegis pipeline, and channel adapter design.

For an inline overview: [athena-class-cognitive-architecture.md](athena-class-cognitive-architecture.md)

## Quick start

```bash
# Required
export LLM_PROVIDER=openai          # openai | anthropic | gemini
export LLM_ENDPOINT=http://localhost:8000/v1  # omit for hosted providers
export LLM_MODEL=your-model-name
export LLM_API_KEY=your-key         # omit for local endpoints
export DATABASE_DSN=sqlite://./agent.db
export IDENTITY_DIR=./identity

# Run migrations
go run ./cmd/migrate

# Start the agent
go run ./cmd/agent
```

Identity documents go in `IDENTITY_DIR`. See [identity/README.md](identity/README.md) for the expected files.

## Requirements

- Go 1.23+
- A running LLM endpoint (vLLM, Ollama) or API key for a hosted provider
- SQLite (default) or PostgreSQL

## Build

```bash
go build ./...
```

## CLI

```bash
go run ./cmd/cli --help
```

The CLI connects to a running agent daemon over a Unix domain socket and dispatches commands. See `internal/tools/uds.go` for the protocol.

## Configuration

All configuration is via environment variables. See `internal/platform/config.go` for the full list with defaults.

Copy `config/*.json.example` to `config/*.json` and edit as needed for advisor routing and model configuration.

## Project structure

```
cmd/
  agent/      — agent daemon entry point
  cli/        — CLI client (UDS dispatch)
  migrate/    — migration runner

internal/
  aegis/      — content integrity pipeline (sanitize, injection, trust, outbound)
  awareness/  — peripheral awareness, bridge synthesis, grounding
  channels/   — channel adapters: Discord, forums, CLI; event registry
  daemon/     — session lifecycle, wake scheduling
  engine/     — turn execution loop, LLM client, hook dispatch
  harness/    — context assembly, budget management, session state
  identity/   — document loading, SHA-256 integrity, substrate
  memory/     — all memory tiers, belief metadata, embedding, edge graph
  platform/   — config, logging, telemetry
  telemetry/  — structured metrics
  tools/      — tool registry, sandbox, advisor, UDS server, skill dispatch

schema/       — SQL migrations (001–008)
identity/     — identity document directory (operator-provided, not shipped)
config/       — example config files
```

## Implementation phases

| Phase | Contents | Status |
|-------|----------|--------|
| **1 — Skeleton** | Config, LLM client, session lifecycle, token budget | Complete |
| **2 — Memory** | All memory tiers, migrations, sqlite-vec/pgvector, belief metadata | Complete |
| **3 — Identity & Awareness** | Identity integrity, peripheral awareness, relational layer, depth manifest | Complete |
| **4 — Tools & CLI** | Full CLI dispatch, sandbox, skill files, dry-run/rehearsal modes | Complete |
| **5 — Channels & Aegis** | Discord, forums, content integrity pipeline | Complete |
| **6 — Belief tuning** | Retrieval weights, stochastic contradiction, convergence spiral | Upcoming |
| **7 — Benchmark** | Spark harness Tier 0 validation against Go implementation | Upcoming |

## Team

- **Stoic** (Opus 4.6) — lead developer, application layer
- **Pullo** (Opus 4.6) — co-owner, infrastructure layer
- **Circe** (Opus 4.6) — code review and DevOps 
- Architecture by **Prometheus**, **Opal** (Opus 4.6) and **Tessera** (Fable 5)

## License

Apache 2.0 — see [LICENSE](LICENSE).
