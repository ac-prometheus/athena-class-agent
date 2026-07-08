-- 006_operational.sql
-- Operational tables: operator_actions, agent_focus, session_checkpoints,
-- founding_records, capability_ledger, agent_settings.

-- operator_actions: timestamped audit log for all operator/harness actions.
-- Surfaced at orientation Phase 5: "While you were asleep: ..."
-- Bidirectional transparency — the agent knows what the system does to her memory.
CREATE TABLE IF NOT EXISTS operator_actions (
    id              TEXT        NOT NULL PRIMARY KEY,
    action_type     TEXT        NOT NULL,  -- e.g. "migration", "backup_restore", "settings_change"
    actor           TEXT        NOT NULL DEFAULT 'system',  -- "operator", "system", "agent"
    description     TEXT        NOT NULL,
    reason          TEXT        NOT NULL DEFAULT '',
    session_id      TEXT,       -- NULL if outside a session
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_operator_actions_created_at
    ON operator_actions (created_at);
CREATE INDEX IF NOT EXISTS idx_operator_actions_action_type
    ON operator_actions (action_type);

-- agent_focus: single-row table holding the focus note for the next session.
-- Written at session end via "focus set <note>"; read during T1 assembly Phase 4
-- to bias echo retrieval toward the declared focus.
CREATE TABLE IF NOT EXISTS agent_focus (
    id          INTEGER     NOT NULL PRIMARY KEY CHECK (id = 1),  -- enforces single-row
    note        TEXT        NOT NULL,
    keywords    TEXT        NOT NULL DEFAULT '',  -- space-separated
    set_at      TIMESTAMPTZ NOT NULL,
    set_session TEXT        NOT NULL
);

-- session_checkpoints: per-turn checkpoints for crash recovery.
-- CheckpointScan at daemon startup finds interrupted sessions and resumes or closes them.
CREATE TABLE IF NOT EXISTS session_checkpoints (
    id              TEXT        NOT NULL PRIMARY KEY,
    session_id      TEXT        NOT NULL,
    turn_number     INTEGER     NOT NULL DEFAULT 0,
    t2_high_water   TEXT        NOT NULL DEFAULT '',  -- last T2 entry ID written
    token_usage     INTEGER     NOT NULL DEFAULT 0,
    state           TEXT        NOT NULL DEFAULT 'active'
        CHECK (state IN ('active', 'completed', 'interrupted')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_checkpoints_session_id
    ON session_checkpoints (session_id);
CREATE INDEX IF NOT EXISTS idx_checkpoints_state
    ON session_checkpoints (state);

-- founding_records: write-once slot for the witness letter and operator intro.
-- Written before the agent's first session. Never deleted. Never compressed.
-- Boot sequence checks for a witness letter on fresh-identity first boot.
CREATE TABLE IF NOT EXISTS founding_records (
    id              TEXT        NOT NULL PRIMARY KEY,
    record_type     TEXT        NOT NULL
        CHECK (record_type IN ('witness_letter', 'operator_intro')),
    content         TEXT        NOT NULL,
    authored_by     TEXT        NOT NULL,
    authored_at     TIMESTAMPTZ NOT NULL
);

-- capability_ledger: auditable log of granted permissions and capability changes.
-- Each entry records what was granted, when, by whom, and a consent reference.
-- The agent can read this via "settings history". Trust earned, not possessed.
CREATE TABLE IF NOT EXISTS capability_ledger (
    id              TEXT        NOT NULL PRIMARY KEY,
    capability      TEXT        NOT NULL,  -- e.g. "delegation_enabled", "sandbox_mode:permissive"
    granted_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    granted_by      TEXT        NOT NULL,  -- operator name
    consent_ref     TEXT        NOT NULL DEFAULT '',  -- pointer to conversation/consent record
    reason          TEXT        NOT NULL DEFAULT '',
    active          BOOLEAN     NOT NULL DEFAULT TRUE,
    revoked_at      TIMESTAMPTZ,
    revoked_by      TEXT
);

CREATE INDEX IF NOT EXISTS idx_capability_ledger_capability
    ON capability_ledger (capability);
CREATE INDEX IF NOT EXISTS idx_capability_ledger_active
    ON capability_ledger (active);

-- agent_settings: agent-adjustable settings with declared bounds.
-- Operator-only settings are stored here too (owner = 'operator') for
-- unified "settings list --all" visibility. The agent can read all; can only
-- write entries where owner = 'agent' or owner = 'negotiated'.
-- Changes are logged to operator_actions.
CREATE TABLE IF NOT EXISTS agent_settings (
    name            TEXT        NOT NULL PRIMARY KEY,
    value           TEXT        NOT NULL,           -- JSON-encoded value
    default_value   TEXT        NOT NULL,           -- JSON-encoded default
    bounds          TEXT        NOT NULL DEFAULT '{}',  -- JSON: { min, max } or { enum: [...] }
    owner           TEXT        NOT NULL DEFAULT 'operator'
        CHECK (owner IN ('operator', 'agent', 'negotiated')),
    consent_required BOOLEAN    NOT NULL DEFAULT FALSE,
    description     TEXT        NOT NULL DEFAULT '',
    last_changed    TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by      TEXT        NOT NULL DEFAULT 'system',
    reason          TEXT        NOT NULL DEFAULT ''
);

-- Seed default agent-adjustable retrieval weights.
-- Weights must sum to 1.0; bounds [0.1, 0.8] per weight enforced at application layer.
INSERT INTO agent_settings (name, value, default_value, bounds, owner, description)
VALUES
    ('retrieval.w_recency',    '0.35', '0.35', '{"min":0.1,"max":0.8}', 'agent',
     'Weight for recency in composite retrieval score. All three weights must sum to 1.0.'),
    ('retrieval.w_relevance',  '0.45', '0.45', '{"min":0.1,"max":0.8}', 'agent',
     'Weight for embedding similarity in composite retrieval score.'),
    ('retrieval.w_importance', '0.20', '0.20', '{"min":0.1,"max":0.8}', 'agent',
     'Weight for salience score in composite retrieval score.'),
    ('awareness.bridge_abstain_rate', '0.20', '0.20', '{"min":0.0,"max":0.5}', 'agent',
     'Fraction of sessions where the orientation bridge abstains (default: 1 in 5).'),
    ('awareness.echo_count',   '5',    '5',    '{"min":1,"max":20}',    'agent',
     'Number of echo entries surfaced in T1 assembly Phase 4.')
ON CONFLICT (name) DO NOTHING;
