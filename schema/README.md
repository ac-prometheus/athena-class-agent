# schema/

SQL migration files for the Athena-class agent database.

## Naming convention

```
NNN_description.sql
```

Where `NNN` is a zero-padded integer (001, 002, ...). The migration runner applies
files in numeric order and tracks applied migrations in a `schema_migrations` table.

## Dialect

Migrations are written in Postgres-compatible SQL. The migration runner applies
dialect rewrite rules for SQLite before executing:

- `SERIAL` / `BIGSERIAL` → `INTEGER PRIMARY KEY AUTOINCREMENT`
- `vector(N)` type → `TEXT` (legacy fallback; sqlite-vec is preferred)
- `ON CONFLICT DO UPDATE` syntax differences
- No `CREATE INDEX CONCURRENTLY` (SQLite has no concurrent DDL)

**Never edit an applied migration.** Add a new file. The runner rejects checksum
mismatches on applied files.

## Phase 2

Migration files are added in Phase 2 alongside the memory tier implementation.
This directory is a placeholder in Phase 1.

## Tables (Phase 2+)

- `schema_migrations` — applied migration tracking
- `sessions` — session records with state and checkpoints
- `experiential_logs` — T2 append-only archive
- `narrative_summaries` — T3 compressed summaries
- `reflections` — T4 agent-authored reflections
- `kg_entities` / `kg_relationships` — T5 world model (bi-temporal)
- `memory_edges` — cross-tier citation and contradiction links
- `relational_profiles` — per-person relational data
- `agent_focus` — focus note for next-session echo retrieval
- `agent_settings` — agent-adjustable settings with bounds
- `operator_actions` — audit log for all operator actions
- `founding_records` — witness letter and operator intro (write-once)
- `wake_requests` — agent-scheduled wake entries
- `trust_registry` — Aegis trust scores (persisted across sessions)
