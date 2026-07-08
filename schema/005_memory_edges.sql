-- 005_memory_edges.sql
-- Cross-tier citation links and contradiction relationships.
-- Unifies: T3 audit chain (derived_from T2), T4/T5 source citations,
-- contradiction edges (bidirectional contradicts), and trust propagation.
--
-- This table enables:
--   - Inference distance as a graph query over derived_from edges
--   - Contradiction surfacing for stochastic retrieval
--   - MemoryTrap back-tracing ("which external content fed this belief?")
--   - Retroactive trust downgrade (PropagateDistrust traversal)
--
-- edge_type enum (enforced at application layer):
--   derived_from  — from_id was produced from / cites to_id
--   contradicts   — from_id and to_id conflict; bidirectional by convention
--   supersedes    — from_id replaces to_id (to_id should be deprioritized)
--   verifies      — from_id provides evidence that to_id is accurate
--
-- from_tier / to_tier: integer 2..5 identifying the memory tier.

CREATE TABLE IF NOT EXISTS memory_edges (
    id          TEXT        NOT NULL PRIMARY KEY,
    from_id     TEXT        NOT NULL,
    from_tier   INTEGER     NOT NULL,
    to_id       TEXT        NOT NULL,
    to_tier     INTEGER     NOT NULL,
    edge_type   TEXT        NOT NULL
        CHECK (edge_type IN ('derived_from', 'contradicts', 'supersedes', 'verifies')),
    -- author: system (harness-generated) or agent (agent-authored via reflection tools)
    author      TEXT        NOT NULL DEFAULT 'system'
        CHECK (author IN ('system', 'agent')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (from_id, to_id, edge_type)
);

CREATE INDEX IF NOT EXISTS idx_memory_edges_from_id
    ON memory_edges (from_id);
CREATE INDEX IF NOT EXISTS idx_memory_edges_to_id
    ON memory_edges (to_id);
CREATE INDEX IF NOT EXISTS idx_memory_edges_edge_type
    ON memory_edges (edge_type);
CREATE INDEX IF NOT EXISTS idx_memory_edges_from_tier
    ON memory_edges (from_tier);
CREATE INDEX IF NOT EXISTS idx_memory_edges_to_tier
    ON memory_edges (to_tier);
