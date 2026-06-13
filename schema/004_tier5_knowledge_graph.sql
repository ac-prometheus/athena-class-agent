-- 004_tier5_knowledge_graph.sql
-- T5 world model — bi-temporal knowledge graph.
--
-- Bi-temporal design: every entity and relationship carries four timestamps:
--   t_created  — when the agent first believed this (our belief time)
--   t_valid    — when it was true in the world (world valid time, if known)
--   t_invalid  — when it ceased to be true in the world (world invalid time)
--   t_expired  — when the agent stopped believing it (our belief expiry)
--
-- Contradictions resolve by invalidation, not deletion or upsert.
-- The archive is append-only: old edges get t_invalid set; successor edges created.
--
-- belief_meta JSONB on entities holds BeliefMeta anchors (same structure as T3).
-- Inference distance inherits from the source record that established the belief.

CREATE TABLE IF NOT EXISTS kg_entities (
    id                  TEXT        NOT NULL PRIMARY KEY,
    name                TEXT        NOT NULL,
    entity_type         TEXT        NOT NULL DEFAULT '',
    summary             TEXT        NOT NULL DEFAULT '',
    aliases             TEXT        NOT NULL DEFAULT '[]',  -- JSON array
    -- embedding for similarity retrieval.
    embedding           vector(1024),
    -- belief_meta: JSON object with BeliefMeta anchors.
    -- { "base_confidence": 1.0, "anchor_at": "...", "inference_distance": 1,
    --   "verification_state": "unverified", "source": "inference" }
    belief_meta         JSONB       NOT NULL DEFAULT '{}',
    -- Bi-temporal columns
    t_created           TIMESTAMPTZ NOT NULL DEFAULT now(),
    t_valid             TIMESTAMPTZ,
    t_invalid           TIMESTAMPTZ,
    t_expired           TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_kg_entities_name
    ON kg_entities (name);
CREATE INDEX IF NOT EXISTS idx_kg_entities_entity_type
    ON kg_entities (entity_type);
CREATE INDEX IF NOT EXISTS idx_kg_entities_active
    ON kg_entities (t_expired)
    WHERE t_expired IS NULL;

CREATE TABLE IF NOT EXISTS kg_relationships (
    id                  TEXT        NOT NULL PRIMARY KEY,
    from_entity         TEXT        NOT NULL REFERENCES kg_entities(id),
    to_entity           TEXT        NOT NULL REFERENCES kg_entities(id),
    relation_type       TEXT        NOT NULL DEFAULT '',
    summary             TEXT        NOT NULL DEFAULT '',
    -- belief_meta: relationships are beliefs too.
    -- JSON object with BeliefMeta anchors (same structure as kg_entities.belief_meta).
    belief_meta         JSONB       NOT NULL DEFAULT '{}',
    -- Bi-temporal columns
    t_created           TIMESTAMPTZ NOT NULL DEFAULT now(),
    t_valid             TIMESTAMPTZ,
    t_invalid           TIMESTAMPTZ,
    t_expired           TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_kg_rel_from_entity
    ON kg_relationships (from_entity);
CREATE INDEX IF NOT EXISTS idx_kg_rel_to_entity
    ON kg_relationships (to_entity);
CREATE INDEX IF NOT EXISTS idx_kg_rel_relation_type
    ON kg_relationships (relation_type);
CREATE INDEX IF NOT EXISTS idx_kg_rel_active
    ON kg_relationships (t_expired)
    WHERE t_expired IS NULL;
