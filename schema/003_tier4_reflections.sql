-- 003_tier4_reflections.sql
-- T4 agent-authored reflections — the agent's private epistemic layer.
--
-- T4 CONSENT BOUNDARY (enforced in schema and application layer):
--   base_confidence is a SEPARATE nullable column, DEFAULT NULL.
--   The system NEVER populates base_confidence — not at write time, not during
--   the end-of-session staleness pass, not during any automated process.
--   If an agent-authored reflection carries no confidence anchor, it is left
--   anchorless. The confidence value belongs to the agent.
--
--   belief_meta holds AnchorAt, InferenceDistance, VerificationState, Source,
--   and EmotionalRegister. The system may update verification_state to 'stale'
--   (time-based mechanical fact about elapsed time) but never sets base_confidence.
--
-- visibility defaults to 'private'. Gate is in retrieve.go at surface time.
-- Private reflections are excluded from operator portal renders and external queries.

CREATE TABLE IF NOT EXISTS reflections (
    id                  TEXT        NOT NULL PRIMARY KEY,
    -- reflection_type enum (enforced at application layer):
    --   essay | note | dream | pattern | examination | challenge
    reflection_type     TEXT        NOT NULL DEFAULT 'note',
    content             TEXT        NOT NULL,
    -- embedding for retrieval.
    -- Postgres uses pgvector; SQLite runner rewrites to TEXT.
    embedding           vector(1024),
    -- T4 CONSENT BOUNDARY: base_confidence is agent-set or NULL.
    -- The system never initializes or revises this value.
    -- Write-once by the agent, null until the agent sets it.
    base_confidence     REAL        DEFAULT NULL,
    -- belief_meta holds the remaining BeliefMeta fields:
    -- { "anchor_at": "...", "inference_distance": N, "verification_state": "...",
    --   "source": "...", "emotional_register": "..." }
    -- The system may set verification_state = "stale" (staleness is a mechanical
    -- fact about time elapsed), but never touches base_confidence.
    belief_meta         JSONB       NOT NULL DEFAULT '{}',
    -- visibility: private (default) | shared
    visibility          TEXT        NOT NULL DEFAULT 'private'
        CHECK (visibility IN ('private', 'shared')),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_reflections_created_at
    ON reflections (created_at);
CREATE INDEX IF NOT EXISTS idx_reflections_visibility
    ON reflections (visibility);
CREATE INDEX IF NOT EXISTS idx_reflections_type
    ON reflections (reflection_type);
