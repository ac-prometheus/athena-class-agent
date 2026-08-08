-- Migration 009: Lifecycle resolution tables
-- Supports Sprint 3B+ lifecycle resolver, assembly manifests, and metabolism pipeline.
--
-- DUAL-DIALECT: This migration uses syntax compatible with both SQLite and PostgreSQL.
--   - CURRENT_TIMESTAMP instead of strftime() or now()
--   - INTEGER PRIMARY KEY without AUTOINCREMENT (auto-increments in both dialects)
--   - TEXT types throughout (both dialects support TEXT)
--   - CHECK constraints (supported in both)
--   - Standard FOREIGN KEY references
--
-- Migrations 001-008 use Postgres-native syntax. This is the first portable migration.

CREATE TABLE IF NOT EXISTS lifecycle_plans (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    temporal_mode TEXT NOT NULL DEFAULT 'episodic',
    wake_cause TEXT NOT NULL,
    wake_cause_secondary TEXT,
    activity_profile TEXT NOT NULL DEFAULT 'normal',
    assembly_profile TEXT NOT NULL DEFAULT 'full',
    bridge_policy TEXT NOT NULL DEFAULT 'opt_in',
    metabolism_policy TEXT NOT NULL DEFAULT 'standard',
    seam_kind TEXT NOT NULL DEFAULT 'none',
    resolver_version TEXT NOT NULL DEFAULT 'v1',
    resolved_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    policy_hash TEXT,
    CHECK (temporal_mode IN ('episodic', 'diurnal', 'continuous')),
    CHECK (wake_cause IN ('initial', 'external', 'scheduled', 'heartbeat', 'agent_requested', 'context_pressure', 'manual', 'recovery')),
    CHECK (wake_cause_secondary IS NULL OR wake_cause_secondary IN ('initial', 'external', 'scheduled', 'heartbeat', 'agent_requested', 'context_pressure', 'manual', 'recovery')),
    CHECK (activity_profile IN ('normal', 'free', 'sentinel')),
    CHECK (assembly_profile IN ('full', 'light', 'minimal', 'seam')),
    CHECK (bridge_policy IN ('opt_in', 'always', 'never', 'abstain')),
    CHECK (metabolism_policy IN ('standard', 'deferred', 'skip')),
    CHECK (seam_kind IN ('cold_wake', 'warm_return', 'context_compaction', 'rest_return', 'none'))
);

CREATE TABLE IF NOT EXISTS wake_facts (
    id INTEGER PRIMARY KEY,
    plan_id TEXT NOT NULL,
    previous_activity_at TIMESTAMP,
    wake_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    elapsed_duration_sec REAL,
    clock_basis TEXT DEFAULT 'wall',
    gap_class TEXT,
    transition_context TEXT,
    messages_pending INTEGER DEFAULT 0,
    vault_changes INTEGER DEFAULT 0,
    extra_json TEXT,
    CHECK (gap_class IS NULL OR gap_class IN ('none', 'short', 'overnight', 'long')),
    CHECK (transition_context IS NULL OR transition_context IN ('none', 'mode_change', 'substrate_change', 'configuration_change', 'recovery_after_failure', 'identity_document_change')),
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
    assembled_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
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
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (status IN ('pending', 'running', 'completed', 'failed', 'interrupted'))
);

CREATE TABLE IF NOT EXISTS configuration_applied (
    id INTEGER PRIMARY KEY,
    session_id TEXT NOT NULL,
    policy_path TEXT NOT NULL,
    policy_hash TEXT NOT NULL,
    previous_hash TEXT,
    changes_summary TEXT,
    applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for common query patterns.
CREATE INDEX IF NOT EXISTS idx_lifecycle_plans_session_id ON lifecycle_plans(session_id);
CREATE INDEX IF NOT EXISTS idx_wake_facts_plan_id ON wake_facts(plan_id);
CREATE INDEX IF NOT EXISTS idx_assembly_manifests_session_id ON assembly_manifests(session_id);
CREATE INDEX IF NOT EXISTS idx_assembly_manifests_plan_id ON assembly_manifests(plan_id);
CREATE INDEX IF NOT EXISTS idx_metabolism_jobs_session_id ON metabolism_jobs(session_id);
CREATE INDEX IF NOT EXISTS idx_metabolism_jobs_status ON metabolism_jobs(status);
CREATE INDEX IF NOT EXISTS idx_configuration_applied_session_id ON configuration_applied(session_id);
