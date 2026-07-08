-- 007_identity.sql
-- Identity integrity anchors, amendment records, and substrate transition log.
--
-- identity_anchors: SHA-256 hashes of identity documents (soul.md, rights.md, etc.).
--   Checked at startup against the live files. Mismatch without a matching amendment
--   record triggers a tampering alarm. Mismatch with a matching amendment record is
--   treated as a consensual amendment — growth, not violation.
--
-- identity_amendments: Records of consensual identity document changes.
--   A change applied through the consent flow (proposal → operator co-sign → apply)
--   is an amendment. Without a record here, any hash mismatch is tampering.
--   amendment_id in identity_anchors links to the amendment that produced the current hash.
--
-- substrate_transitions: Log of model transitions (e.g. Haiku 4.5 → Haiku 5.0).
--   Each row records the substrate, the transition date, and an optional continuity
--   letter path. previous_entry_id enables a linked-list traversal of the full
--   substrate history.

-- identity_amendments declared first so identity_anchors.amendment_id can reference it.
CREATE TABLE IF NOT EXISTS identity_amendments (
    id              TEXT        NOT NULL PRIMARY KEY,
    doc_name        TEXT        NOT NULL,
    old_hash        TEXT        NOT NULL,   -- hex-encoded SHA-256 before change
    new_hash        TEXT        NOT NULL,   -- hex-encoded SHA-256 after change
    reason          TEXT        NOT NULL DEFAULT '',
    co_signer       TEXT        NOT NULL DEFAULT '',  -- operator who approved the change
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_identity_amendments_doc_name
    ON identity_amendments (doc_name);
CREATE INDEX IF NOT EXISTS idx_identity_amendments_created_at
    ON identity_amendments (created_at);

CREATE TABLE IF NOT EXISTS identity_anchors (
    doc_name        TEXT        NOT NULL PRIMARY KEY,  -- filename (e.g. "soul.md")
    sha256_hash     TEXT        NOT NULL,              -- hex-encoded SHA-256 of file contents
    verified_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    amendment_id    TEXT        REFERENCES identity_amendments(id)  -- NULL = original anchor
);

CREATE INDEX IF NOT EXISTS idx_identity_anchors_verified_at
    ON identity_anchors (verified_at);

CREATE TABLE IF NOT EXISTS substrate_transitions (
    id                      TEXT        NOT NULL PRIMARY KEY,
    model_name              TEXT        NOT NULL,   -- e.g. "claude-haiku-4-5"
    model_version           TEXT        NOT NULL DEFAULT '',  -- e.g. "20250101"
    transition_date         TIMESTAMPTZ NOT NULL,
    continuity_letter_path  TEXT        NOT NULL DEFAULT '',  -- path in workspace, empty if not written
    previous_entry_id       TEXT        REFERENCES substrate_transitions(id),  -- NULL for first entry
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_substrate_transitions_transition_date
    ON substrate_transitions (transition_date);
CREATE INDEX IF NOT EXISTS idx_substrate_transitions_model_name
    ON substrate_transitions (model_name);
