-- 001_tier2_experiential.sql
-- T2 experiential log — append-only archive. Records are never deleted.
-- content_source tracks provenance for MemoryTrap defense and T3 audit chain.
-- No embedding column: T2 is an archive, not a retrieval target.
-- Retrieval goes through T3 (narrative summaries with embeddings).
--
-- Dialect note: BOOLEAN → INTEGER (SQLite), TIMESTAMPTZ → TEXT (SQLite),
-- now() → CURRENT_TIMESTAMP (SQLite). The migration runner handles rewrites.

CREATE TABLE IF NOT EXISTS experiential_logs (
    id               TEXT        NOT NULL PRIMARY KEY,
    session_id       TEXT        NOT NULL,
    turn_number      INTEGER     NOT NULL DEFAULT 0,
    participant      TEXT        NOT NULL DEFAULT '',
    role             TEXT        NOT NULL DEFAULT 'assistant',
    content          TEXT        NOT NULL,
    topic_tags       TEXT        NOT NULL DEFAULT '[]',  -- JSON array
    salience_score   REAL        NOT NULL DEFAULT 0.0,
    -- content_source enum (enforced at application layer):
    --   operator       — direct from operator / harness config
    --   self           — agent's own reasoning output
    --   tool-result    — structured output from a sandboxed tool call
    --   browser-content— content fetched via browser/fetch pipeline
    --   search-result  — content from a search API response
    --   forum-content  — content from a forum adapter
    content_source   TEXT        NOT NULL DEFAULT 'self',
    -- embedding_pending: set TRUE on insert; cleared by the T2→T3 compression pipeline
    -- (Phase 2 internal/memory/tier3.go) when the entry has been summarised into a
    -- narrative_summaries row and an embedding backfilled. The background EmbedWorker
    -- queries WHERE embedding_pending = TRUE to identify unprocessed entries.
    embedding_pending BOOLEAN    NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_experiential_session_id
    ON experiential_logs (session_id);
CREATE INDEX IF NOT EXISTS idx_experiential_created_at
    ON experiential_logs (created_at);
CREATE INDEX IF NOT EXISTS idx_experiential_embedding_pending
    ON experiential_logs (embedding_pending)
    WHERE embedding_pending = TRUE;
