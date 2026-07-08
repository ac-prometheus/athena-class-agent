# schema/

SQL migration files for the Athena-class agent database.

## Naming convention

```
NNN_description.sql
```

Where `NNN` is a zero-padded integer (001, 002, ...). The migration runner applies files in numeric order and tracks applied migrations in a `schema_migrations` table.

## Migrations

| File | Contents |
|------|----------|
| `001_tier2_experiential.sql` | T2 experiential archive — append-only session log with embedding support |
| `002_tier3_narrative.sql` | T3 narrative summaries — compressed session summaries with recency metadata |
| `003_tier4_reflections.sql` | T4 reflections — agent-authored reflections with salience scoring |
| `004_tier5_knowledge_graph.sql` | T5 world model — bi-temporal knowledge graph entities and relationships |
| `005_memory_edges.sql` | Cross-tier citation and contradiction links between memory records |
| `006_operational.sql` | Operational tables: sessions, focus note, settings, operator actions, founding records, wake requests |
| `007_identity.sql` | Identity integrity anchors — SHA-256 hashes for identity documents |
| `008_trust_registry.sql` | Aegis trust registry — persisted EWMA trust scores per source |

## Dialect

Migrations are written in Postgres-compatible SQL. The migration runner applies dialect rewrite rules for SQLite before executing:

- `SERIAL` / `BIGSERIAL` → `INTEGER PRIMARY KEY AUTOINCREMENT`
- `vector(N)` type → `TEXT` (legacy fallback; sqlite-vec is preferred)
- `ON CONFLICT DO UPDATE` syntax differences
- No `CREATE INDEX CONCURRENTLY` (SQLite has no concurrent DDL)

**Never edit an applied migration.** Add a new file. The runner rejects checksum mismatches on applied files.

## Tables

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
- `identity_anchors` — SHA-256 hashes for identity document integrity
- `trust_registry` — Aegis trust scores (persisted across sessions)
