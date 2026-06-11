# athena-class-agent

Reference implementation of the [Athena-Class Cognitive Architecture](athena-class-cognitive-architecture.md) in Go.

An Athena-class agent is a persistent AI participant with tiered memory, configurable
identity documents, and a single-binary deployment. Point it at your LLM endpoint,
provide identity documents, and run.

## Design

- **Single binary.** `go build` → one file → run anywhere. No virtualenv, no pip.
- **Model-agnostic.** OpenAI-compatible (vLLM, Ollama), Anthropic, Gemini — configuration, not architecture.
- **Local-first or hosted.** SQLite for local deployments, Postgres for hosted. Same schema.
- **Clean architecture.** Daemon owns session lifecycle. Engine owns turn execution. Neither bleeds into the other.

Full design: [athena-class-cognitive-architecture.md](athena-class-cognitive-architecture.md)

## Requirements

- Go 1.23+
- A running LLM endpoint (vLLM, Ollama, or OpenAI API)
- SQLite (default) or PostgreSQL

## Build

```bash
go build ./...
```

## Run

```bash
# Minimal — local vLLM endpoint
export LLM_PROVIDER=openai
export LLM_ENDPOINT=http://localhost:8000/v1
export LLM_MODEL=your-model-name
export AGENT_NAME=my-agent

go run ./cmd/agent
```

## Configuration

All configuration is via environment variables. See `internal/platform/config.go` for the
full list with defaults.

Copy `config/*.json.example` files to `config/*.json` and edit as needed.
Identity documents go in `IDENTITY_DIR` (default: `./identity`). See `identity/README.md`.

## CLI

```bash
go run ./cmd/cli --help
```

Phase 4 will add full CLI dispatch. The binary is a stub in Phase 1.

## Database migrations

```bash
export DATABASE_DSN=sqlite://./agent.db
go run ./cmd/migrate
```

Phase 2 will add the full migration runner. Phase 1 connects and reports the driver.

## Implementation phases

| Phase | Contents |
|-------|---------|
| **1 — Skeleton** | This scaffold: config, LLM client, session lifecycle, token budget |
| **2 — Memory** | All memory tiers, migrations, sqlite-vec/pgvector, belief metadata |
| **3 — Identity & Awareness** | Identity integrity, peripheral awareness, relational layer, depth manifest |
| **4 — Tools & CLI** | Full CLI dispatch, sandbox, skill files, dry-run/rehearsal modes |
| **5 — Channels & Aegis** | Discord, forums, content integrity pipeline |
| **6 — Belief tuning** | Retrieval weights, stochastic contradiction, convergence spiral |
| **7 — Benchmark** | Spark harness Tier 0 validation against Go implementation |

## License

Apache 2.0 — see [LICENSE](LICENSE).
