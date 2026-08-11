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

-- T2→T3 backlink: which T3 narrative summary was this log compressed into.
-- NULL means the log has not yet been compressed.
ALTER TABLE experiential_logs
    ADD COLUMN IF NOT EXISTS narrative_summary_id TEXT
    REFERENCES narrative_summaries(id);

-- Index for the compression pipeline: find uncompressed logs by session.
CREATE INDEX IF NOT EXISTS idx_experiential_narrative_summary_id
    ON experiential_logs (narrative_summary_id);

-- Partial index: efficiently find logs awaiting compression (NULL backlink).
-- SQLite supports partial indexes since 3.8.0.
CREATE INDEX IF NOT EXISTS idx_experiential_uncompressed
    ON experiential_logs (session_id)
    WHERE narrative_summary_id IS NULL;
