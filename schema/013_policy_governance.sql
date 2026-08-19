-- Migration 013: Add policy governance columns to configuration_applied.
-- Tracks policy source, normalized snapshot, and rejection reasons for
-- audit and provenance.
--
-- Uses the same create-new/copy/drop/rename pattern as 010/011/012.

-- Step 1: Create the new table with governance columns.
CREATE TABLE IF NOT EXISTS configuration_applied_new (
    id INTEGER PRIMARY KEY,
    session_id TEXT NOT NULL,
    policy_path TEXT NOT NULL,
    policy_hash TEXT NOT NULL,
    previous_hash TEXT,
    changes_summary TEXT,
    policy_source TEXT NOT NULL DEFAULT '',
    policy_snapshot TEXT NOT NULL DEFAULT '',
    rejection_reason TEXT,
    applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Step 2: Copy existing data (new columns get defaults).
INSERT OR IGNORE INTO configuration_applied_new
    (id, session_id, policy_path, policy_hash, previous_hash,
     changes_summary, applied_at)
    SELECT id, session_id, policy_path, policy_hash, previous_hash,
           changes_summary, applied_at
    FROM configuration_applied;

-- Step 3: Drop old table and rename.
DROP TABLE IF EXISTS configuration_applied;
ALTER TABLE configuration_applied_new RENAME TO configuration_applied;

-- Step 4: Recreate indexes (idx_configuration_applied_session_id from 009).
CREATE INDEX IF NOT EXISTS idx_configuration_applied_session_id ON configuration_applied(session_id);
