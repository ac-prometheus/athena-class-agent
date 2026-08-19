-- Migration 012: WP-C3 provenance carriage — ingress annotation through T2, T3, assembly.
--
-- Adds:
--   experiential_logs.aegis_meta        — JSON-encoded AegisAnnotation from ingest.
--     Populated when the log was created from an Aegis-screened inbound channel event
--     (C-3 fix). Empty string ('') means no carried annotation (self-authored or
--     pre-migration rows). Metabolism uses this to verify the annotation instead of
--     re-screening (WP-C3 avoid-rescreen rule).
--
--   narrative_summaries.content_sources — JSON array of distinct content_source values
--     from the T2 logs compressed into this T3 summary (e.g. '["discord","self"]').
--     Assembly surfaces this so the agent knows what external input shaped a summary.
--
--   narrative_summaries.aegis_meta      — JSON-encoded AegisAnnotation carried from
--     the highest-trust external T2 log in this summary. Empty when no external
--     content was present.
--
-- SQLite compatibility: ALTER TABLE ADD COLUMN with a DEFAULT value is supported
-- since early SQLite versions. The NOT NULL constraint is satisfied on existing
-- rows by the DEFAULT. No table rebuild required.
--
-- DUAL-DIALECT: Compatible with both SQLite and PostgreSQL.

ALTER TABLE experiential_logs ADD COLUMN aegis_meta TEXT NOT NULL DEFAULT '';

ALTER TABLE narrative_summaries ADD COLUMN content_sources TEXT NOT NULL DEFAULT '[]';
ALTER TABLE narrative_summaries ADD COLUMN aegis_meta TEXT NOT NULL DEFAULT '';
