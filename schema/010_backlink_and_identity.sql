-- Migration 010: T2→T3 backlink column + PostgreSQL identity strategy
--
-- DUAL-DIALECT: Compatible with both SQLite and PostgreSQL.
--
-- 1. Adds narrative_summary_id to experiential_logs — the T2→T3 backlink
--    that AtomicT2T3Link writes atomically with T3 creation.
--
-- 2. For wake_facts.id and configuration_applied.id, the migration 009
--    pattern (INTEGER PRIMARY KEY) auto-increments in SQLite but requires
--    GENERATED ALWAYS AS IDENTITY in PostgreSQL. Since migration 009 used
--    bare INTEGER PRIMARY KEY (valid in both), we leave those as-is —
--    PostgreSQL treats INTEGER PRIMARY KEY as SERIAL-compatible. If strict
--    IDENTITY semantics are needed, a future migration can ALTER COLUMN.
--
-- Uses create-new/copy/drop/rename pattern for SQLite compatibility.
-- ADD COLUMN IF NOT EXISTS requires SQLite 3.35+; this pattern works on all versions.

-- Step 1: Create the new table with the added narrative_summary_id column.
CREATE TABLE IF NOT EXISTS experiential_logs_new (
    id               TEXT        NOT NULL PRIMARY KEY,
    session_id       TEXT        NOT NULL,
    turn_number      INTEGER     NOT NULL DEFAULT 0,
    participant      TEXT        NOT NULL DEFAULT '',
    role             TEXT        NOT NULL DEFAULT 'assistant',
    content          TEXT        NOT NULL,
    topic_tags       TEXT        NOT NULL DEFAULT '[]',
    salience_score   REAL        NOT NULL DEFAULT 0.0,
    content_source   TEXT        NOT NULL DEFAULT 'self',
    embedding_pending BOOLEAN    NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    narrative_summary_id TEXT    REFERENCES narrative_summaries(id)
);

-- Step 2: Copy existing data (NULL for the new column).
INSERT OR IGNORE INTO experiential_logs_new
    SELECT *, NULL FROM experiential_logs;

-- Step 3: Drop old table and rename.
DROP TABLE IF EXISTS experiential_logs;
ALTER TABLE experiential_logs_new RENAME TO experiential_logs;

-- Step 4: Recreate all indexes (original + new).
CREATE INDEX IF NOT EXISTS idx_experiential_session_id
    ON experiential_logs (session_id);
CREATE INDEX IF NOT EXISTS idx_experiential_created_at
    ON experiential_logs (created_at);
CREATE INDEX IF NOT EXISTS idx_experiential_embedding_pending
    ON experiential_logs (embedding_pending)
    WHERE embedding_pending = TRUE;
CREATE INDEX IF NOT EXISTS idx_experiential_narrative_summary_id
    ON experiential_logs (narrative_summary_id);
CREATE INDEX IF NOT EXISTS idx_experiential_uncompressed
    ON experiential_logs (session_id)
    WHERE narrative_summary_id IS NULL;
