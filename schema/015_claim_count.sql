-- Migration 015: Add claim_count to metabolism_jobs for crash-loop prevention.
-- retry_count tracks application-level failures (Fail() calls). claim_count
-- tracks ownership transitions (Claim() calls) — including silent process deaths
-- that never invoke Fail. Both are needed: retry_count for audit, claim_count
-- for bounding recovery attempts.
--
-- Uses the same create-new/copy/drop/rename pattern as 010/011/012.

-- Step 1: Create the new table with claim_count.
CREATE TABLE IF NOT EXISTS metabolism_jobs_new (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    job_type TEXT NOT NULL DEFAULT 'standard',
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    error_message TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    claim_token TEXT,
    claim_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (status IN ('pending', 'running', 'completed', 'failed', 'interrupted', 'review_required'))
);

-- Step 2: Copy existing data (claim_count defaults to 0).
INSERT OR IGNORE INTO metabolism_jobs_new
    (id, session_id, status, job_type, started_at, completed_at,
     error_message, retry_count, claim_token, created_at)
    SELECT id, session_id, status, job_type, started_at, completed_at,
           error_message, retry_count, claim_token, created_at
    FROM metabolism_jobs;

-- Step 3: Drop old table and rename.
DROP TABLE IF EXISTS metabolism_jobs;
ALTER TABLE metabolism_jobs_new RENAME TO metabolism_jobs;

-- Step 4: Recreate indexes.
CREATE INDEX IF NOT EXISTS idx_metabolism_jobs_session_id ON metabolism_jobs(session_id);
CREATE INDEX IF NOT EXISTS idx_metabolism_jobs_status ON metabolism_jobs(status);
