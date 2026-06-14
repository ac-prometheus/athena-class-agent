-- 002_tier3_narrative.sql
-- T3 narrative summaries — compressed records produced by the summarizer.
-- Each row is derived from T2 entries; the audit chain is in memory_edges
-- (derived_from edges from this T3 row to the source T2 entries).
--
-- belief_meta JSONB holds BeliefMeta as a JSON object:
--   { base_confidence, anchor_at, inference_distance, verification_state, source }
-- BeliefMeta anchors are immutable; confidence is computed at read time.
--
-- Dialect note: JSONB → TEXT (SQLite), vector(N) → TEXT (SQLite).

CREATE TABLE IF NOT EXISTS narrative_summaries (
    id                  TEXT        NOT NULL PRIMARY KEY,
    session_id          TEXT        NOT NULL DEFAULT '',
    key_topics          TEXT        NOT NULL DEFAULT '[]',  -- JSON array of strings
    emotional_tone      TEXT        NOT NULL DEFAULT '',
    continuity_prev     TEXT        NOT NULL DEFAULT '',    -- ID of prior summary
    continuity_next     TEXT        NOT NULL DEFAULT '',    -- filled when next is written
    content             TEXT        NOT NULL,
    -- embedding: vector(N) for similarity retrieval.
    -- Postgres uses pgvector column type; SQLite runner rewrites to TEXT.
    embedding           vector(1024),
    -- belief_meta: JSON object containing BeliefMeta anchors.
    -- { "base_confidence": 1.0, "anchor_at": "...", "inference_distance": 1,
    --   "verification_state": "unverified", "source": "experience" }
    belief_meta         JSONB       NOT NULL DEFAULT '{}',
    content_source      TEXT        NOT NULL DEFAULT 'self',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_narrative_session_id
    ON narrative_summaries (session_id);
CREATE INDEX IF NOT EXISTS idx_narrative_created_at
    ON narrative_summaries (created_at);
