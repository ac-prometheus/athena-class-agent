-- Migration 011: Add review_required status to metabolism_jobs
-- Supports HARN-80: when external content lacks Aegis annotation, the
-- metabolism job stops with review_required status instead of bracketing
-- and promoting untrusted text to T3.
--
-- SQLite does not support ALTER TABLE ... DROP CONSTRAINT / ADD CONSTRAINT,
-- so we recreate the table with the updated CHECK constraint.

-- Step 1: Create the new table with the updated CHECK constraint.
CREATE TABLE IF NOT EXISTS metabolism_jobs_new (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    job_type TEXT NOT NULL DEFAULT 'standard',
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    error_message TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (status IN ('pending', 'running', 'completed', 'failed', 'interrupted', 'review_required'))
);

-- Step 2: Copy existing data.
-- ON CONFLICT DO NOTHING is standard SQL supported by both SQLite 3.24+ and
-- PostgreSQL, replacing the SQLite-only INSERT OR IGNORE form.
INSERT INTO metabolism_jobs_new
    SELECT * FROM metabolism_jobs
    ON CONFLICT DO NOTHING;

-- Step 3: Drop old table and rename.
DROP TABLE IF EXISTS metabolism_jobs;
ALTER TABLE metabolism_jobs_new RENAME TO metabolism_jobs;

-- Step 4: Recreate indexes.
CREATE INDEX IF NOT EXISTS idx_metabolism_jobs_session_id ON metabolism_jobs(session_id);
CREATE INDEX IF NOT EXISTS idx_metabolism_jobs_status ON metabolism_jobs(status);
