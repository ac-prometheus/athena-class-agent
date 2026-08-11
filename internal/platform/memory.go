package platform

// Memory tier operations (T2/T3/T4/T5) and relational profiles.
// Split from db.go for readability. Methods on SQLiteStore and PostgresStore.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
	"github.com/jackc/pgx/v5"
)

// AppendExperiential inserts a T2 experiential log entry (append-only).
// T2 has no embedding column — it is an archive, not a retrieval target.
func (s *SQLiteStore) AppendExperiential(ctx context.Context, entry pkg.ExperientialLog) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO experiential_logs
			(id, session_id, content, content_source, embedding_pending, created_at)
		 VALUES (?, ?, ?, ?, 1, CURRENT_TIMESTAMP)`,
		entry.ID, entry.SessionID, entry.Content, entry.ContentSource,
	)
	return err
}

// SearchNarrative returns a stub empty slice.
// Full vector search is handled by the VectorIndex layer (sqlite-vec).
func (s *SQLiteStore) SearchNarrative(_ context.Context, _ []float32, _ int) ([]pkg.NarrativeSummary, error) {
	return nil, nil
}

// InsertNarrative inserts a T3 narrative summary.
// Belief anchors are stored in belief_meta JSON; base_confidence is a separate column.
func (s *SQLiteStore) InsertNarrative(ctx context.Context, summary pkg.NarrativeSummary) error {
	belief := summary.Belief
	if belief == nil {
		belief = &pkg.BeliefMeta{
			BaseConfidence:    1.0,
			AnchorAt:          time.Now().UTC(),
			InferenceDistance: 1,
			VerificationState: "unverified",
		}
	}
	if belief.AnchorAt.IsZero() {
		belief.AnchorAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO narrative_summaries
			(id, session_id, content, belief_meta, created_at)
		 VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		summary.ID, "", summary.Content, beliefMetaJSON(belief),
	)
	return err
}

// InsertReflection inserts a T4 agent-authored reflection.
//
// T4 CONSENT BOUNDARY: base_confidence is NEVER set by the system.
// It remains NULL unless the agent explicitly provided it (indicated by a
// non-zero AnchorAt on the Belief). The system may update verification_state
// to 'stale' via FlagStaleBeliefs, but never touches base_confidence.
func (s *SQLiteStore) InsertReflection(ctx context.Context, ref pkg.Reflection) error {
	visibility := ref.Visibility
	if visibility == "" {
		visibility = pkg.VisibilityPrivate
	}

	// T4 consent boundary: only write base_confidence if the agent provided it.
	// We detect agent authorship by a non-zero AnchorAt on the Belief.
	var baseConf interface{} = nil
	var metaJSON = "{}"

	if ref.Belief != nil && !ref.Belief.AnchorAt.IsZero() {
		baseConf = ref.Belief.BaseConfidence
		metaJSON = beliefMetaJSON(ref.Belief)
	} else if ref.Belief != nil {
		// AnchorAt is zero — agent did not provide confidence anchor.
		// Still encode the non-confidence fields into belief_meta.
		metaJSON = beliefMetaJSON(ref.Belief)
		if ref.Belief.BaseConfidence != 0 {
			slog.Warn("InsertReflection: base_confidence set without anchor_at — confidence will not decay correctly",
				"id", ref.ID, "base_confidence", ref.Belief.BaseConfidence)
		}
	}

	refType := ref.Type
	if refType == "" {
		refType = "note"
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO reflections
			(id, reflection_type, content, base_confidence, belief_meta, visibility, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		ref.ID, refType, ref.Content, baseConf, metaJSON, visibility,
	)
	return err
}

// SearchReflections returns T4 reflections by recency.
// Full vector search goes through the VectorIndex layer.
func (s *SQLiteStore) SearchReflections(ctx context.Context, _ []float32, limit int) ([]pkg.Reflection, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, content, visibility, belief_meta, base_confidence
		 FROM reflections
		 ORDER BY created_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []pkg.Reflection
	for rows.Next() {
		var r pkg.Reflection
		var metaJSON string
		var baseConf sql.NullFloat64
		if err := rows.Scan(&r.ID, &r.Content, &r.Visibility, &metaJSON, &baseConf); err != nil {
			return nil, err
		}
		var bc *float64
		if baseConf.Valid {
			bc = &baseConf.Float64
		}
		r.Belief = decodeBelief(metaJSON, bc)
		out = append(out, r)
	}
	return out, rows.Err()
}

// SearchEntities returns T5 entities matching the query string (simple LIKE).
func (s *SQLiteStore) SearchEntities(ctx context.Context, query string, limit int) ([]pkg.Entity, error) {
	like := "%" + escapeLike(query) + "%"
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, entity_type, summary, belief_meta
		 FROM kg_entities
		 WHERE (name LIKE ? ESCAPE '\' OR summary LIKE ? ESCAPE '\') AND t_expired IS NULL
		 ORDER BY t_created DESC LIMIT ?`,
		like, like, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []pkg.Entity
	for rows.Next() {
		var e pkg.Entity
		var metaJSON string
		if err := rows.Scan(&e.ID, &e.Name, &e.Type, &e.Content, &metaJSON); err != nil {
			return nil, err
		}
		e.Belief = decodeBelief(metaJSON, nil)
		out = append(out, e)
	}
	return out, rows.Err()
}

// UpsertEntity inserts or updates a T5 knowledge graph entity.
// On conflict (same ID), updates name, type, summary, and belief_meta.
func (s *SQLiteStore) UpsertEntity(ctx context.Context, entity pkg.Entity) error {
	belief := entity.Belief
	if belief == nil {
		belief = &pkg.BeliefMeta{
			BaseConfidence:    1.0,
			AnchorAt:          time.Now().UTC(),
			InferenceDistance: 1,
			VerificationState: "unverified",
		}
	}
	if belief.AnchorAt.IsZero() {
		belief.AnchorAt = time.Now().UTC()
	}

	aliases, _ := json.Marshal([]string{})

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO kg_entities
			(id, name, entity_type, summary, aliases, belief_meta, t_created)
		 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(id) DO UPDATE SET
			name        = excluded.name,
			entity_type = excluded.entity_type,
			summary     = excluded.summary,
			belief_meta = excluded.belief_meta`,
		entity.ID, entity.Name, entity.Type, entity.Content,
		string(aliases), beliefMetaJSON(belief),
	)
	return err
}

// GetProfile returns a relational profile by name (or alias LIKE match).
func (s *SQLiteStore) GetProfile(ctx context.Context, name string) (*pkg.RelationalProfile, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, content FROM relational_profiles
		 WHERE name = ? OR aliases LIKE ? ESCAPE '\' LIMIT 1`,
		name, "%"+escapeLike(name)+"%",
	)
	var p pkg.RelationalProfile
	if err := row.Scan(&p.ID, &p.Name, &p.Content); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// ListProfiles returns all relational profiles.
func (s *SQLiteStore) ListProfiles(ctx context.Context) ([]pkg.RelationalProfile, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, content FROM relational_profiles ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []pkg.RelationalProfile
	for rows.Next() {
		var p pkg.RelationalProfile
		if err := rows.Scan(&p.ID, &p.Name, &p.Content); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
// AppendExperiential inserts a T2 experiential log entry (append-only).
// T2 has no embedding column — it is an archive, not a retrieval target.
func (p *PostgresStore) AppendExperiential(ctx context.Context, entry pkg.ExperientialLog) error {
	_, err := p.pool.Exec(ctx,
		`INSERT INTO experiential_logs
			(id, session_id, content, content_source, embedding_pending, created_at)
		 VALUES ($1, $2, $3, $4, TRUE, now())`,
		entry.ID, entry.SessionID, entry.Content, entry.ContentSource,
	)
	return err
}

// SearchNarrative returns a stub empty slice.
// Full vector search is handled by the pgvector VectorIndex implementation.
func (p *PostgresStore) SearchNarrative(_ context.Context, _ []float32, _ int) ([]pkg.NarrativeSummary, error) {
	return nil, nil
}

// InsertNarrative inserts a T3 narrative summary.
func (p *PostgresStore) InsertNarrative(ctx context.Context, summary pkg.NarrativeSummary) error {
	belief := summary.Belief
	if belief == nil {
		belief = &pkg.BeliefMeta{
			BaseConfidence:    1.0,
			AnchorAt:          time.Now().UTC(),
			InferenceDistance: 1,
			VerificationState: "unverified",
		}
	}
	if belief.AnchorAt.IsZero() {
		belief.AnchorAt = time.Now().UTC()
	}

	_, err := p.pool.Exec(ctx,
		`INSERT INTO narrative_summaries
			(id, session_id, content, belief_meta, created_at)
		 VALUES ($1, $2, $3, $4::jsonb, now())`,
		summary.ID, "", summary.Content, beliefMetaJSON(belief),
	)
	return err
}

// InsertReflection inserts a T4 agent-authored reflection.
//
// T4 CONSENT BOUNDARY: base_confidence is NEVER set by the system.
// See SQLiteStore.InsertReflection for full rationale.
func (p *PostgresStore) InsertReflection(ctx context.Context, ref pkg.Reflection) error {
	visibility := ref.Visibility
	if visibility == "" {
		visibility = pkg.VisibilityPrivate
	}

	var baseConf interface{} = nil
	var metaJSON = "{}"

	if ref.Belief != nil && !ref.Belief.AnchorAt.IsZero() {
		baseConf = ref.Belief.BaseConfidence
		metaJSON = beliefMetaJSON(ref.Belief)
	} else if ref.Belief != nil {
		metaJSON = beliefMetaJSON(ref.Belief)
		if ref.Belief.BaseConfidence != 0 {
			slog.Warn("InsertReflection: base_confidence set without anchor_at — confidence will not decay correctly",
				"id", ref.ID, "base_confidence", ref.Belief.BaseConfidence)
		}
	}

	refType := ref.Type
	if refType == "" {
		refType = "note"
	}

	_, err := p.pool.Exec(ctx,
		`INSERT INTO reflections
			(id, reflection_type, content, base_confidence, belief_meta, visibility, created_at)
		 VALUES ($1, $2, $3, $4, $5::jsonb, $6, now())`,
		ref.ID, refType, ref.Content, baseConf, metaJSON, visibility,
	)
	return err
}

// SearchReflections returns T4 reflections by recency.
func (p *PostgresStore) SearchReflections(ctx context.Context, _ []float32, limit int) ([]pkg.Reflection, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, content, visibility, belief_meta::text, base_confidence
		 FROM reflections
		 ORDER BY created_at DESC LIMIT $1`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []pkg.Reflection
	for rows.Next() {
		var r pkg.Reflection
		var metaJSON string
		var baseConf *float64
		if err := rows.Scan(&r.ID, &r.Content, &r.Visibility, &metaJSON, &baseConf); err != nil {
			return nil, err
		}
		r.Belief = decodeBelief(metaJSON, baseConf)
		out = append(out, r)
	}
	return out, rows.Err()
}

// SearchEntities returns T5 entities matching the query string.
func (p *PostgresStore) SearchEntities(ctx context.Context, query string, limit int) ([]pkg.Entity, error) {
	like := "%" + escapeLike(query) + "%"
	rows, err := p.pool.Query(ctx,
		`SELECT id, name, entity_type, summary, belief_meta::text
		 FROM kg_entities
		 WHERE (name ILIKE $1 OR summary ILIKE $1) AND t_expired IS NULL
		 ORDER BY t_created DESC LIMIT $2`,
		like, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []pkg.Entity
	for rows.Next() {
		var e pkg.Entity
		var metaJSON string
		if err := rows.Scan(&e.ID, &e.Name, &e.Type, &e.Content, &metaJSON); err != nil {
			return nil, err
		}
		e.Belief = decodeBelief(metaJSON, nil)
		out = append(out, e)
	}
	return out, rows.Err()
}

// UpsertEntity inserts or updates a T5 knowledge graph entity.
func (p *PostgresStore) UpsertEntity(ctx context.Context, entity pkg.Entity) error {
	belief := entity.Belief
	if belief == nil {
		belief = &pkg.BeliefMeta{
			BaseConfidence:    1.0,
			AnchorAt:          time.Now().UTC(),
			InferenceDistance: 1,
			VerificationState: "unverified",
		}
	}
	if belief.AnchorAt.IsZero() {
		belief.AnchorAt = time.Now().UTC()
	}

	aliases, _ := json.Marshal([]string{})

	_, err := p.pool.Exec(ctx,
		`INSERT INTO kg_entities
			(id, name, entity_type, summary, aliases, belief_meta, t_created)
		 VALUES ($1, $2, $3, $4, $5, $6::jsonb, now())
		 ON CONFLICT(id) DO UPDATE SET
			name        = EXCLUDED.name,
			entity_type = EXCLUDED.entity_type,
			summary     = EXCLUDED.summary,
			belief_meta = EXCLUDED.belief_meta`,
		entity.ID, entity.Name, entity.Type, entity.Content,
		string(aliases), beliefMetaJSON(belief),
	)
	return err
}

// GetProfile returns a relational profile by name or alias match.
func (p *PostgresStore) GetProfile(ctx context.Context, name string) (*pkg.RelationalProfile, error) {
	row := p.pool.QueryRow(ctx,
		`SELECT id, name, content FROM relational_profiles
		 WHERE name = $1 OR aliases::text ILIKE $2 LIMIT 1`,
		name, "%"+escapeLike(name)+"%",
	)
	var prof pkg.RelationalProfile
	if err := row.Scan(&prof.ID, &prof.Name, &prof.Content); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &prof, nil
}

// ListProfiles returns all relational profiles.
func (p *PostgresStore) ListProfiles(ctx context.Context) ([]pkg.RelationalProfile, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, name, content FROM relational_profiles ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []pkg.RelationalProfile
	for rows.Next() {
		var prof pkg.RelationalProfile
		if err := rows.Scan(&prof.ID, &prof.Name, &prof.Content); err != nil {
			return nil, err
		}
		out = append(out, prof)
	}
	return out, rows.Err()
}
