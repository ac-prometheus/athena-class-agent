-- Migration 009: Lifecycle resolution tables
-- Supports Sprint 3B+ lifecycle resolver, assembly manifests, and metabolism pipeline.
-- SQLite-native syntax (TEXT timestamps with strftime defaults, no Postgres-specific features).

CREATE TABLE IF NOT EXISTS lifecycle_plans (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    temporal_mode TEXT NOT NULL DEFAULT 'episodic',
    wake_cause TEXT NOT NULL,
    assembly_profile TEXT NOT NULL DEFAULT 'full',
    bridge_policy TEXT NOT NULL DEFAULT 'opt_in',
    metabolism_policy TEXT NOT NULL DEFAULT 'standard',
    resolver_version TEXT NOT NULL DEFAULT 'v1',
    resolved_at TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    policy_hash TEXT,
    FOREIGN KEY (session_id) REFERENCES session_checkpoints(session_id)
);

CREATE TABLE IF NOT EXISTS wake_facts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id TEXT NOT NULL,
    fact_key TEXT NOT NULL,
    fact_value TEXT NOT NULL,
    FOREIGN KEY (plan_id) REFERENCES lifecycle_plans(id)
);

CREATE TABLE IF NOT EXISTS assembly_manifests (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    plan_id TEXT NOT NULL,
    phases_loaded TEXT NOT NULL,
    phases_skipped TEXT,
    known_omissions TEXT,
    total_tokens_used INTEGER NOT NULL DEFAULT 0,
    total_tokens_budget INTEGER NOT NULL DEFAULT 0,
    context_utilization REAL,
    assembled_at TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    FOREIGN KEY (plan_id) REFERENCES lifecycle_plans(id)
);

CREATE TABLE IF NOT EXISTS metabolism_jobs (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    job_type TEXT NOT NULL DEFAULT 'standard',
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    error_message TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    CHECK (status IN ('pending', 'running', 'completed', 'failed', 'interrupted'))
);

CREATE TABLE IF NOT EXISTS configuration_applied (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    policy_path TEXT NOT NULL,
    policy_hash TEXT NOT NULL,
    previous_hash TEXT,
    changes_summary TEXT,
    applied_at TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- Indexes for common query patterns.
CREATE INDEX IF NOT EXISTS idx_lifecycle_plans_session_id ON lifecycle_plans(session_id);
CREATE INDEX IF NOT EXISTS idx_wake_facts_plan_id ON wake_facts(plan_id);
CREATE INDEX IF NOT EXISTS idx_assembly_manifests_session_id ON assembly_manifests(session_id);
CREATE INDEX IF NOT EXISTS idx_assembly_manifests_plan_id ON assembly_manifests(plan_id);
CREATE INDEX IF NOT EXISTS idx_metabolism_jobs_session_id ON metabolism_jobs(session_id);
CREATE INDEX IF NOT EXISTS idx_metabolism_jobs_status ON metabolism_jobs(status);
CREATE INDEX IF NOT EXISTS idx_configuration_applied_session_id ON configuration_applied(session_id);
