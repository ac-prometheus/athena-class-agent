package platform

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/mattn/go-sqlite3"
)

// validTierTables is the allowlist of table names that may appear in
// dynamic SQL (UpdateVerificationState, MarkNeedsReview, embed worker).
// Prevents SQL injection via table name concatenation.
var validTierTables = map[string]bool{
	"narrative_summaries": true,
	"reflections":         true,
	"kg_entities":         true,
	"kg_relationships":    true,
	"experiential_logs":   true,
}

func validateTableName(table string) error {
	if !validTierTables[table] {
		return fmt.Errorf("platform: invalid table name %q — not in tier allowlist", table)
	}
	return nil
}

// Store wraps a *sql.DB with the DSN it was opened from.
// Used by the migration runner and other platform-level code that needs
// raw database access outside the MemoryStore interface.
type Store struct {
	DB     *sql.DB
	Driver string // "sqlite3" or "postgres"
	dsn    string
}

// NewStore opens a database connection from a DSN string.
// DSN prefix determines the driver:
//
//	sqlite:// or sqlite3://  → SQLite (file path follows the prefix)
//	postgres:// or postgresql:// → Postgres
//
// The returned Store is not yet migrated; run the migration runner separately.
func NewStore(dsn string) (*Store, error) {
	driver, rawDSN, err := parseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("parsing DSN: %w", err)
	}

	db, err := sql.Open(driver, rawDSN)
	if err != nil {
		return nil, fmt.Errorf("opening %s database: %w", driver, err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("connecting to %s database: %w", driver, err)
	}

	return &Store{DB: db, Driver: driver, dsn: rawDSN}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.DB.Close()
}

// RedactedDSN returns the DSN with any password replaced by ***.
func (s *Store) RedactedDSN() string {
	u, err := url.Parse(s.dsn)
	if err != nil {
		return "[unparseable DSN]"
	}
	if u.User != nil {
		u.User = url.UserPassword(u.User.Username(), "***")
	}
	return u.String()
}

// parseDSN splits a DSN into (driver, raw-dsn).
func parseDSN(dsn string) (string, string, error) {
	switch {
	case strings.HasPrefix(dsn, "sqlite://"):
		return "sqlite3", strings.TrimPrefix(dsn, "sqlite://"), nil
	case strings.HasPrefix(dsn, "sqlite3://"):
		return "sqlite3", strings.TrimPrefix(dsn, "sqlite3://"), nil
	case strings.HasPrefix(dsn, "postgres://"), strings.HasPrefix(dsn, "postgresql://"):
		return "postgres", dsn, nil
	default:
		return "", "", fmt.Errorf("unsupported DSN scheme (want sqlite:// or postgres://): %q", dsn)
	}
}

// NewMemoryStore constructs the appropriate MemoryStore implementation from a DSN.
func NewMemoryStore(ctx context.Context, dsn string) (pkg.MemoryStore, error) {
	switch {
	case strings.HasPrefix(dsn, "sqlite://") || strings.HasPrefix(dsn, "sqlite3://"):
		return NewSQLiteStore(dsn)
	case strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://"):
		return NewPostgresStore(ctx, dsn)
	default:
		return nil, fmt.Errorf("unsupported DSN scheme: %q", dsn)
	}
}

// beliefMetaJSON encodes a BeliefMeta into the belief_meta JSON column format.
// escapeLike escapes % and _ in a LIKE pattern argument so they are treated
// as literals rather than wildcards. Callers must also append ESCAPE '\' to
// the SQL predicate when using this function.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	s = strings.ReplaceAll(s, "_", `\_`)
	return s
}

// base_confidence is excluded — it lives in its own column on T3/T5,
// and in its own nullable column on T4 (never system-set).
func beliefMetaJSON(b *pkg.BeliefMeta) string {
	if b == nil {
		return "{}"
	}
	m := map[string]any{
		"anchor_at":          b.AnchorAt.UTC().Format(time.RFC3339),
		"inference_distance": b.InferenceDistance,
		"verification_state": b.VerificationState,
		"source":             b.Source,
	}
	if b.EmotionalRegister != "" {
		m["emotional_register"] = b.EmotionalRegister
	}
	raw, _ := json.Marshal(m)
	return string(raw)
}

// decodeBelief decodes a belief_meta JSON string and base_confidence into a BeliefMeta.
func decodeBelief(metaJSON string, baseConf *float64) *pkg.BeliefMeta {
	var m map[string]any
	_ = json.Unmarshal([]byte(metaJSON), &m)

	b := &pkg.BeliefMeta{}
	if baseConf != nil {
		b.BaseConfidence = *baseConf
	}
	if v, ok := m["anchor_at"].(string); ok {
		b.AnchorAt, _ = time.Parse(time.RFC3339, v)
	}
	if v, ok := m["inference_distance"].(float64); ok {
		b.InferenceDistance = int(v)
	}
	if v, ok := m["verification_state"].(string); ok {
		b.VerificationState = v
	}
	if v, ok := m["source"].(string); ok {
		b.Source = v
	}
	if v, ok := m["emotional_register"].(string); ok {
		b.EmotionalRegister = v
	}
	return b
}

// ---------------------------------------------------------------------------
// SQLiteStore
// ---------------------------------------------------------------------------

// SQLiteStore implements pkg.MemoryStore using database/sql + go-sqlite3.
type SQLiteStore struct {
	db  *sql.DB
	dsn string
}

// NewSQLiteStore opens a SQLite database at the path encoded in the DSN.
func NewSQLiteStore(dsn string) (*SQLiteStore, error) {
	_, rawDSN, err := parseDSN(dsn)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", rawDSN+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("opening sqlite3 database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("connecting to sqlite3 database: %w", err)
	}

	return &SQLiteStore{db: db, dsn: dsn}, nil
}

// Close closes the underlying database.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

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

// ---------------------------------------------------------------------------
// PostgresStore
// ---------------------------------------------------------------------------

// PostgresStore implements pkg.MemoryStore using pgx/v5 connection pool.
type PostgresStore struct {
	pool *pgxpool.Pool
	dsn  string
}

// NewPostgresStore opens a pgx connection pool (default pool size: 5).
func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parsing postgres DSN: %w", err)
	}
	cfg.MaxConns = 5

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("opening postgres pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connecting to postgres: %w", err)
	}

	return &PostgresStore{pool: pool, dsn: dsn}, nil
}

// Close closes the connection pool.
func (p *PostgresStore) Close() error {
	p.pool.Close()
	return nil
}

// RedactedDSN returns the DSN with any password replaced by ***.
func (p *PostgresStore) RedactedDSN() string {
	u, err := url.Parse(p.dsn)
	if err != nil {
		return "[unparseable DSN]"
	}
	if u.User != nil {
		u.User = url.UserPassword(u.User.Username(), "***")
	}
	return u.String()
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

// ---------------------------------------------------------------------------
// SQL dialect helpers — moved from internal/memory/embeddings.go so all SQL
// lives in platform/ and tier files need no driver-specific logic.
// ---------------------------------------------------------------------------

// placeholder returns the SQL placeholder for positional argument n (1-based)
// appropriate for the given driver: "?" for sqlite3, "$N" for postgres.
func placeholder(driverName string, n int) string {
	if driverName == "postgres" {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// jsonUpdate returns a SQL expression that sets a JSON path to a value.
// Postgres uses jsonb_set; SQLite uses json_set.
//
//	col      — the column name (e.g. "belief_meta")
//	path     — JSON path in $.key notation (e.g. "$.verification_state")
//	valuePH  — the already-rendered placeholder for the value arg (e.g. "$1" or "?")
//
// The returned expression replaces the column in a SET clause:
//
//	UPDATE t SET belief_meta = <jsonUpdate(...)> WHERE ...
func jsonUpdate(driverName, col, path, valuePH string) string {
	if driverName == "postgres" {
		key := path
		if len(key) > 2 && key[:2] == "$." {
			key = "{" + key[2:] + "}"
		}
		return "jsonb_set(" + col + ", '" + key + "', to_jsonb(" + valuePH + "::text))"
	}
	return "json_set(" + col + ", '" + path + "', " + valuePH + ")"
}

// ---------------------------------------------------------------------------
// SQLiteStore — EdgeStore, BeliefStore, KGMutationStore, T2QueryStore
// ---------------------------------------------------------------------------

// QueryLogs retrieves T2 experiential log entries for a session, oldest-first.
func (s *SQLiteStore) QueryLogs(ctx context.Context, sessionID string, limit int) ([]pkg.ExperientialLog, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, content, content_source, created_at
		 FROM experiential_logs
		 WHERE session_id = ?
		 ORDER BY created_at ASC
		 LIMIT ?`,
		sessionID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: querying logs for session %s: %w", sessionID, err)
	}
	defer rows.Close()

	var out []pkg.ExperientialLog
	for rows.Next() {
		var e pkg.ExperientialLog
		var createdAt time.Time
		if err := rows.Scan(&e.ID, &e.SessionID, &e.Content, &e.ContentSource, &createdAt); err != nil {
			return nil, fmt.Errorf("platform: scanning log row: %w", err)
		}
		e.CreatedAt = createdAt
		out = append(out, e)
	}
	return out, rows.Err()
}

// InvalidateEntity sets t_expired on an entity (bi-temporal expiry, not delete).
// Returns rows affected so callers can detect "not found or already expired".
func (s *SQLiteStore) InvalidateEntity(ctx context.Context, entityID string, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE kg_entities SET t_expired = ? WHERE id = ? AND t_expired IS NULL`,
		now.UTC(), entityID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// InvalidateRelationship sets t_expired on a relationship without deleting it.
// Returns rows affected so callers can detect "not found or already expired".
func (s *SQLiteStore) InvalidateRelationship(ctx context.Context, relID string, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE kg_relationships SET t_expired = ? WHERE id = ? AND t_expired IS NULL`,
		now.UTC(), relID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// InsertSupersedgesEdge inserts the supersedes memory_edge for SupersedeEntity.
// Returns (1, nil) on success, (0, err) on failure.
func (s *SQLiteStore) InsertSupersedgesEdge(ctx context.Context, edgeID, newEntityID, oldID string, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO memory_edges
			(id, from_id, from_tier, to_id, to_tier, edge_type, author, created_at)
		 VALUES (?, ?, 5, ?, 5, 'supersedes', 'system', ?)`,
		edgeID, newEntityID, oldID, now.UTC(),
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CreateEdge inserts a memory_edge record. Duplicate (from_id, to_id, edge_type) is silently ignored.
func (s *SQLiteStore) CreateEdge(ctx context.Context, fromID, toID string, fromTier, toTier int, edgeType, author string) error {
	id := newPlatformID()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO memory_edges
			(id, from_id, from_tier, to_id, to_tier, edge_type, author, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (from_id, to_id, edge_type) DO NOTHING`,
		id, fromID, fromTier, toID, toTier, edgeType, author, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("platform: inserting edge from=%s to=%s type=%s: %w", fromID, toID, edgeType, err)
	}
	return nil
}

// GetEdges returns memory_edges connected to recordID.
// direction: "from" (edges where from_id = recordID) or "to" (edges where to_id = recordID).
func (s *SQLiteStore) GetEdges(ctx context.Context, recordID, direction string) ([]pkg.MemoryEdge, error) {
	var query string
	switch direction {
	case "from":
		query = `SELECT id, from_id, from_tier, to_id, to_tier, edge_type, author, created_at
				 FROM memory_edges WHERE from_id = ? ORDER BY created_at ASC`
	case "to":
		query = `SELECT id, from_id, from_tier, to_id, to_tier, edge_type, author, created_at
				 FROM memory_edges WHERE to_id = ? ORDER BY created_at ASC`
	default:
		return nil, fmt.Errorf("platform: direction must be 'from' or 'to', got %q", direction)
	}

	rows, err := s.db.QueryContext(ctx, query, recordID)
	if err != nil {
		return nil, fmt.Errorf("platform: querying edges for %s (direction=%s): %w", recordID, direction, err)
	}
	defer rows.Close()

	var out []pkg.MemoryEdge
	for rows.Next() {
		var e pkg.MemoryEdge
		if err := rows.Scan(&e.ID, &e.FromID, &e.FromTier, &e.ToID, &e.ToTier,
			&e.EdgeType, &e.Author, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("platform: scanning edge row: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// FetchDownstreamEdges returns records that derive_from the given ID (downstream,
// for trust propagation in PropagateDistrust). Queries memory_edges WHERE to_id = id.
// See also QueryEdgesForBFS, which queries upstream (from_id = id) for inference distance.
func (s *SQLiteStore) FetchDownstreamEdges(ctx context.Context, id string) ([]pkg.EdgeNode, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT from_id, from_tier FROM memory_edges
		 WHERE to_id = ? AND edge_type = 'derived_from'`,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: querying downstream edges of %s: %w", id, err)
	}
	defer rows.Close()

	var out []pkg.EdgeNode
	for rows.Next() {
		var n pkg.EdgeNode
		if err := rows.Scan(&n.ID, &n.Tier); err != nil {
			return nil, fmt.Errorf("platform: scanning downstream edge: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// LoadBeliefRecords fetches all T3/T4/T5 rows with belief_meta set and not yet stale.
func (s *SQLiteStore) LoadBeliefRecords(ctx context.Context) ([]pkg.BeliefRecord, error) {
	var records []pkg.BeliefRecord

	for _, t := range []struct {
		table string
		tier  int
	}{
		{"narrative_summaries", 3},
		{"kg_entities", 5},
	} {
		rows, err := s.db.QueryContext(ctx,
			`SELECT id, belief_meta FROM `+t.table+`
			 WHERE belief_meta != '{}' AND belief_meta != ''
			   AND belief_meta NOT LIKE '%"verification_state":"stale"%'`,
		)
		if err != nil {
			return nil, fmt.Errorf("platform: querying %s: %w", t.table, err)
		}

		for rows.Next() {
			var id, metaJSON string
			if err := rows.Scan(&id, &metaJSON); err != nil {
				rows.Close()
				return nil, fmt.Errorf("platform: scanning %s row: %w", t.table, err)
			}
			records = append(records, pkg.BeliefRecord{
				ID:        id,
				Belief:    decodeBeliefMeta(metaJSON),
				TableName: t.table,
			})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("platform: iterating %s: %w", t.table, err)
		}
	}

	// T4: base_confidence lives in its own column (consent boundary).
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, belief_meta, base_confidence FROM reflections
		 WHERE base_confidence IS NOT NULL
		   AND belief_meta != '{}' AND belief_meta != ''
		   AND belief_meta NOT LIKE '%"verification_state":"stale"%'`,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: querying reflections: %w", err)
	}
	for rows.Next() {
		var id, metaJSON string
		var baseConf float64
		if err := rows.Scan(&id, &metaJSON, &baseConf); err != nil {
			rows.Close()
			return nil, fmt.Errorf("platform: scanning reflections row: %w", err)
		}
		belief := decodeBeliefMeta(metaJSON)
		belief.BaseConfidence = baseConf
		records = append(records, pkg.BeliefRecord{
			ID:        id,
			Belief:    belief,
			TableName: "reflections",
		})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("platform: iterating reflections: %w", err)
	}

	return records, nil
}

// UpdateVerificationState updates the verification_state JSON field in belief_meta.
// T4 CONSENT BOUNDARY: base_confidence is never touched here.
func (s *SQLiteStore) UpdateVerificationState(ctx context.Context, table, id, state string) error {
	if err := validateTableName(table); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE `+table+` SET belief_meta = json_set(belief_meta, '$.verification_state', ?) WHERE id = ?`,
		state, id,
	)
	return err
}

// QueryEdgesForBFS queries upstream edges (from_id = id) for inference distance computation.
// Queries memory_edges WHERE from_id = id AND edge_type = 'derived_from'.
// See FetchDownstreamEdges for the downstream direction used by PropagateDistrust.
func (s *SQLiteStore) QueryEdgesForBFS(ctx context.Context, id string) ([]pkg.EdgeNode, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT to_id, to_tier FROM memory_edges
		 WHERE from_id = ? AND edge_type = 'derived_from'`,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: querying BFS edges for %s: %w", id, err)
	}
	defer rows.Close()

	var out []pkg.EdgeNode
	for rows.Next() {
		var n pkg.EdgeNode
		if err := rows.Scan(&n.ID, &n.Tier); err != nil {
			return nil, fmt.Errorf("platform: scanning BFS edge: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// MarkNeedsReview updates verification_state to "needs_review" on the given record.
// T4 CONSENT BOUNDARY: base_confidence is never touched here.
// T2 has no belief_meta column — this is a no-op for tier 2.
func (s *SQLiteStore) MarkNeedsReview(ctx context.Context, id string, tier int) error {
	table, err := tierTableName(tier)
	if err != nil {
		return err
	}
	if tier == 2 {
		return nil
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE `+table+` SET belief_meta = json_set(belief_meta, '$.verification_state', ?) WHERE id = ?`,
		"needs_review", id,
	)
	if err != nil {
		return fmt.Errorf("platform: marking needs_review on %s.%s: %w", table, id, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// PostgresStore — EdgeStore, BeliefStore, KGMutationStore, T2QueryStore
// ---------------------------------------------------------------------------

// QueryLogs retrieves T2 experiential log entries for a session, oldest-first.
func (p *PostgresStore) QueryLogs(ctx context.Context, sessionID string, limit int) ([]pkg.ExperientialLog, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, session_id, content, content_source, created_at
		 FROM experiential_logs
		 WHERE session_id = $1
		 ORDER BY created_at ASC
		 LIMIT $2`,
		sessionID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: querying logs for session %s: %w", sessionID, err)
	}
	defer rows.Close()

	var out []pkg.ExperientialLog
	for rows.Next() {
		var e pkg.ExperientialLog
		var createdAt time.Time
		if err := rows.Scan(&e.ID, &e.SessionID, &e.Content, &e.ContentSource, &createdAt); err != nil {
			return nil, fmt.Errorf("platform: scanning log row: %w", err)
		}
		e.CreatedAt = createdAt
		out = append(out, e)
	}
	return out, rows.Err()
}

// InvalidateEntity sets t_expired on an entity (bi-temporal expiry, not delete).
// Returns rows affected so callers can detect "not found or already expired".
func (p *PostgresStore) InvalidateEntity(ctx context.Context, entityID string, now time.Time) (int64, error) {
	tag, err := p.pool.Exec(ctx,
		`UPDATE kg_entities SET t_expired = $1 WHERE id = $2 AND t_expired IS NULL`,
		now.UTC(), entityID,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// InvalidateRelationship sets t_expired on a relationship without deleting it.
// Returns rows affected so callers can detect "not found or already expired".
func (p *PostgresStore) InvalidateRelationship(ctx context.Context, relID string, now time.Time) (int64, error) {
	tag, err := p.pool.Exec(ctx,
		`UPDATE kg_relationships SET t_expired = $1 WHERE id = $2 AND t_expired IS NULL`,
		now.UTC(), relID,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// InsertSupersedgesEdge inserts the supersedes memory_edge for SupersedeEntity.
// Returns (1, nil) on success, (0, err) on failure.
func (p *PostgresStore) InsertSupersedgesEdge(ctx context.Context, edgeID, newEntityID, oldID string, now time.Time) (int64, error) {
	tag, err := p.pool.Exec(ctx,
		`INSERT INTO memory_edges
			(id, from_id, from_tier, to_id, to_tier, edge_type, author, created_at)
		 VALUES ($1, $2, 5, $3, 5, 'supersedes', 'system', $4)`,
		edgeID, newEntityID, oldID, now.UTC(),
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// CreateEdge inserts a memory_edge record. Duplicate (from_id, to_id, edge_type) is silently ignored.
func (p *PostgresStore) CreateEdge(ctx context.Context, fromID, toID string, fromTier, toTier int, edgeType, author string) error {
	id := newPlatformID()
	_, err := p.pool.Exec(ctx,
		`INSERT INTO memory_edges
			(id, from_id, from_tier, to_id, to_tier, edge_type, author, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (from_id, to_id, edge_type) DO NOTHING`,
		id, fromID, fromTier, toID, toTier, edgeType, author, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("platform: inserting edge from=%s to=%s type=%s: %w", fromID, toID, edgeType, err)
	}
	return nil
}

// GetEdges returns memory_edges connected to recordID.
// direction: "from" (edges where from_id = recordID) or "to" (edges where to_id = recordID).
func (p *PostgresStore) GetEdges(ctx context.Context, recordID, direction string) ([]pkg.MemoryEdge, error) {
	var query string
	switch direction {
	case "from":
		query = `SELECT id, from_id, from_tier, to_id, to_tier, edge_type, author, created_at
				 FROM memory_edges WHERE from_id = $1 ORDER BY created_at ASC`
	case "to":
		query = `SELECT id, from_id, from_tier, to_id, to_tier, edge_type, author, created_at
				 FROM memory_edges WHERE to_id = $1 ORDER BY created_at ASC`
	default:
		return nil, fmt.Errorf("platform: direction must be 'from' or 'to', got %q", direction)
	}

	rows, err := p.pool.Query(ctx, query, recordID)
	if err != nil {
		return nil, fmt.Errorf("platform: querying edges for %s (direction=%s): %w", recordID, direction, err)
	}
	defer rows.Close()

	var out []pkg.MemoryEdge
	for rows.Next() {
		var e pkg.MemoryEdge
		if err := rows.Scan(&e.ID, &e.FromID, &e.FromTier, &e.ToID, &e.ToTier,
			&e.EdgeType, &e.Author, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("platform: scanning edge row: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// FetchDownstreamEdges returns records that derive_from the given ID (downstream,
// for trust propagation in PropagateDistrust). Queries memory_edges WHERE to_id = id.
// See also QueryEdgesForBFS, which queries upstream (from_id = id) for inference distance.
func (p *PostgresStore) FetchDownstreamEdges(ctx context.Context, id string) ([]pkg.EdgeNode, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT from_id, from_tier FROM memory_edges
		 WHERE to_id = $1 AND edge_type = 'derived_from'`,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: querying downstream edges of %s: %w", id, err)
	}
	defer rows.Close()

	var out []pkg.EdgeNode
	for rows.Next() {
		var n pkg.EdgeNode
		if err := rows.Scan(&n.ID, &n.Tier); err != nil {
			return nil, fmt.Errorf("platform: scanning downstream edge: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// LoadBeliefRecords fetches all T3/T4/T5 rows with belief_meta set and not yet stale.
func (p *PostgresStore) LoadBeliefRecords(ctx context.Context) ([]pkg.BeliefRecord, error) {
	var records []pkg.BeliefRecord

	for _, t := range []struct {
		table string
		tier  int
	}{
		{"narrative_summaries", 3},
		{"kg_entities", 5},
	} {
		rows, err := p.pool.Query(ctx,
			`SELECT id, belief_meta::text FROM `+t.table+`
			 WHERE belief_meta::text != '{}' AND belief_meta::text != ''
			   AND belief_meta::text NOT LIKE '%"verification_state":"stale"%'`,
		)
		if err != nil {
			return nil, fmt.Errorf("platform: querying %s: %w", t.table, err)
		}

		for rows.Next() {
			var id, metaJSON string
			if err := rows.Scan(&id, &metaJSON); err != nil {
				rows.Close()
				return nil, fmt.Errorf("platform: scanning %s row: %w", t.table, err)
			}
			records = append(records, pkg.BeliefRecord{
				ID:        id,
				Belief:    decodeBeliefMeta(metaJSON),
				TableName: t.table,
			})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("platform: iterating %s: %w", t.table, err)
		}
	}

	// T4: base_confidence lives in its own column (consent boundary).
	rows, err := p.pool.Query(ctx,
		`SELECT id, belief_meta::text, base_confidence FROM reflections
		 WHERE base_confidence IS NOT NULL
		   AND belief_meta::text != '{}' AND belief_meta::text != ''
		   AND belief_meta::text NOT LIKE '%"verification_state":"stale"%'`,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: querying reflections: %w", err)
	}
	for rows.Next() {
		var id, metaJSON string
		var baseConf float64
		if err := rows.Scan(&id, &metaJSON, &baseConf); err != nil {
			rows.Close()
			return nil, fmt.Errorf("platform: scanning reflections row: %w", err)
		}
		belief := decodeBeliefMeta(metaJSON)
		belief.BaseConfidence = baseConf
		records = append(records, pkg.BeliefRecord{
			ID:        id,
			Belief:    belief,
			TableName: "reflections",
		})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("platform: iterating reflections: %w", err)
	}

	return records, nil
}

// UpdateVerificationState updates the verification_state JSON field in belief_meta.
// T4 CONSENT BOUNDARY: base_confidence is never touched here.
func (p *PostgresStore) UpdateVerificationState(ctx context.Context, table, id, state string) error {
	if err := validateTableName(table); err != nil {
		return err
	}
	_, err := p.pool.Exec(ctx,
		`UPDATE `+table+` SET belief_meta = jsonb_set(belief_meta, '{verification_state}', to_jsonb($1::text)) WHERE id = $2`,
		state, id,
	)
	return err
}

// QueryEdgesForBFS queries upstream edges (from_id = id) for inference distance computation.
// Queries memory_edges WHERE from_id = id AND edge_type = 'derived_from'.
// See FetchDownstreamEdges for the downstream direction used by PropagateDistrust.
func (p *PostgresStore) QueryEdgesForBFS(ctx context.Context, id string) ([]pkg.EdgeNode, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT to_id, to_tier FROM memory_edges
		 WHERE from_id = $1 AND edge_type = 'derived_from'`,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: querying BFS edges for %s: %w", id, err)
	}
	defer rows.Close()

	var out []pkg.EdgeNode
	for rows.Next() {
		var n pkg.EdgeNode
		if err := rows.Scan(&n.ID, &n.Tier); err != nil {
			return nil, fmt.Errorf("platform: scanning BFS edge: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// MarkNeedsReview updates verification_state to "needs_review" on the given record.
// T4 CONSENT BOUNDARY: base_confidence is never touched here.
// T2 has no belief_meta column — this is a no-op for tier 2.
func (p *PostgresStore) MarkNeedsReview(ctx context.Context, id string, tier int) error {
	table, err := tierTableName(tier)
	if err != nil {
		return err
	}
	if tier == 2 {
		return nil
	}
	_, err = p.pool.Exec(ctx,
		`UPDATE `+table+` SET belief_meta = jsonb_set(belief_meta, '{verification_state}', to_jsonb($1::text)) WHERE id = $2`,
		"needs_review", id,
	)
	if err != nil {
		return fmt.Errorf("platform: marking needs_review on %s.%s: %w", table, id, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Shared helpers for the new interface methods
// ---------------------------------------------------------------------------

// tierTableName maps tier number to DB table name.
// Mirrors the private tierTable in internal/memory/edges.go.
func tierTableName(tier int) (string, error) {
	switch tier {
	case 2:
		return "experiential_logs", nil
	case 3:
		return "narrative_summaries", nil
	case 4:
		return "reflections", nil
	case 5:
		return "kg_entities", nil
	default:
		return "", fmt.Errorf("platform: unknown tier %d", tier)
	}
}

// decodeBeliefMeta decodes a belief_meta JSON string into a BeliefMeta.
// Mirrors the private parseBeliefMeta in internal/memory/belief.go
// but without base_confidence (that's a separate column).
func decodeBeliefMeta(metaJSON string) *pkg.BeliefMeta {
	var m map[string]any
	_ = json.Unmarshal([]byte(metaJSON), &m)
	b := &pkg.BeliefMeta{}
	if v, ok := m["anchor_at"].(string); ok {
		b.AnchorAt, _ = time.Parse(time.RFC3339, v)
	}
	if v, ok := m["inference_distance"].(float64); ok {
		b.InferenceDistance = int(v)
	}
	if v, ok := m["verification_state"].(string); ok {
		b.VerificationState = v
	}
	if v, ok := m["source"].(string); ok {
		b.Source = v
	}
	return b
}

// newPlatformID generates a random UUID v4 string using crypto/rand.
// Mirrors newID() in internal/memory/ids.go — kept separate so platform/
// does not import internal/memory.
func newPlatformID() string {
	var b [16]byte
	_, err := rand.Read(b[:])
	if err != nil {
		panic(fmt.Sprintf("platform: crypto/rand unavailable: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// ---------------------------------------------------------------------------
// SQLiteStore — pkg.IdentityAnchorStore
// ---------------------------------------------------------------------------

// GetAnchor returns the stored SHA-256 anchor for docName.
// Returns ("", "", nil) if no anchor exists yet.
func (s *SQLiteStore) GetAnchor(ctx context.Context, docName string) (hash string, amendmentID string, err error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT sha256_hash, COALESCE(amendment_id, '') FROM identity_anchors WHERE doc_name = ?`,
		docName,
	)
	err = row.Scan(&hash, &amendmentID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", "", nil
		}
		return "", "", fmt.Errorf("platform: getting anchor for %q: %w", docName, err)
	}
	return hash, amendmentID, nil
}

// UpsertAnchor sets or updates the anchor for docName.
func (s *SQLiteStore) UpsertAnchor(ctx context.Context, docName, hash, amendmentID string) error {
	var amendArg interface{} = nil
	if amendmentID != "" {
		amendArg = amendmentID
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO identity_anchors (doc_name, sha256_hash, amendment_id, verified_at)
		 VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(doc_name) DO UPDATE SET
			sha256_hash  = excluded.sha256_hash,
			amendment_id = excluded.amendment_id,
			verified_at  = CURRENT_TIMESTAMP`,
		docName, hash, amendArg,
	)
	if err != nil {
		return fmt.Errorf("platform: upserting anchor for %q: %w", docName, err)
	}
	return nil
}

// GetAmendmentByID returns the amendment record with the given id, or nil if not found.
func (s *SQLiteStore) GetAmendmentByID(ctx context.Context, id string) (*pkg.AmendmentRecord, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, doc_name, old_hash, new_hash, reason, co_signer, created_at
		 FROM identity_amendments WHERE id = ?`, id,
	)
	var rec pkg.AmendmentRecord
	err := row.Scan(&rec.ID, &rec.DocName, &rec.OldHash, &rec.NewHash,
		&rec.Reason, &rec.CoSigner, &rec.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("platform: getting amendment %q: %w", id, err)
	}
	return &rec, nil
}

// InsertAmendment inserts a new amendment record and returns the generated ID.
func (s *SQLiteStore) InsertAmendment(ctx context.Context, rec pkg.AmendmentRecord) (string, error) {
	if rec.ID == "" {
		rec.ID = newPlatformID()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO identity_amendments
			(id, doc_name, old_hash, new_hash, reason, co_signer, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		rec.ID, rec.DocName, rec.OldHash, rec.NewHash, rec.Reason, rec.CoSigner,
	)
	if err != nil {
		return "", fmt.Errorf("platform: inserting amendment: %w", err)
	}
	return rec.ID, nil
}

// ListAmendments returns all amendment records for docName, newest first.
func (s *SQLiteStore) ListAmendments(ctx context.Context, docName string) ([]pkg.AmendmentRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, doc_name, old_hash, new_hash, reason, co_signer, created_at
		 FROM identity_amendments WHERE doc_name = ?
		 ORDER BY created_at DESC`,
		docName,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: listing amendments for %q: %w", docName, err)
	}
	defer rows.Close()

	var out []pkg.AmendmentRecord
	for rows.Next() {
		var rec pkg.AmendmentRecord
		if err := rows.Scan(&rec.ID, &rec.DocName, &rec.OldHash, &rec.NewHash,
			&rec.Reason, &rec.CoSigner, &rec.CreatedAt); err != nil {
			return nil, fmt.Errorf("platform: scanning amendment row: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ListAnchoredDocs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT doc_name FROM identity_anchors`)
	if err != nil {
		return nil, fmt.Errorf("platform: listing anchored docs: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("platform: scanning anchored doc: %w", err)
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

// ---------------------------------------------------------------------------
// SQLiteStore — pkg.SubstrateStore
// ---------------------------------------------------------------------------

// InsertSubstrateTransition records a new substrate transition.
func (s *SQLiteStore) InsertSubstrateTransition(ctx context.Context, entry pkg.SubstrateTransition) error {
	var prevID interface{} = nil
	if entry.PreviousEntryID != "" {
		prevID = entry.PreviousEntryID
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO substrate_transitions
			(id, model_name, model_version, transition_date, continuity_letter_path, previous_entry_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		entry.ID, entry.ModelName, entry.ModelVersion,
		entry.TransitionDate.UTC(), entry.ContinuityLetterPath, prevID,
	)
	if err != nil {
		return fmt.Errorf("platform: inserting substrate transition: %w", err)
	}
	return nil
}

// GetLatestSubstrate returns the most recent substrate transition, or (nil, nil) if empty.
func (s *SQLiteStore) GetLatestSubstrate(ctx context.Context) (*pkg.SubstrateTransition, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, model_name, model_version, transition_date,
			continuity_letter_path, COALESCE(previous_entry_id, ''), created_at
		 FROM substrate_transitions ORDER BY transition_date DESC LIMIT 1`,
	)
	var e pkg.SubstrateTransition
	err := row.Scan(&e.ID, &e.ModelName, &e.ModelVersion, &e.TransitionDate,
		&e.ContinuityLetterPath, &e.PreviousEntryID, &e.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("platform: scanning substrate row: %w", err)
	}
	return &e, nil
}

// ListSubstrateHistory returns all substrate transitions, oldest first.
func (s *SQLiteStore) ListSubstrateHistory(ctx context.Context) ([]pkg.SubstrateTransition, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, model_name, model_version, transition_date,
			continuity_letter_path, COALESCE(previous_entry_id, ''), created_at
		 FROM substrate_transitions ORDER BY transition_date ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: listing substrate history: %w", err)
	}
	defer rows.Close()

	var out []pkg.SubstrateTransition
	for rows.Next() {
		var e pkg.SubstrateTransition
		if err := rows.Scan(&e.ID, &e.ModelName, &e.ModelVersion, &e.TransitionDate,
			&e.ContinuityLetterPath, &e.PreviousEntryID, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("platform: scanning substrate row: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// PostgresStore — pkg.IdentityAnchorStore
// ---------------------------------------------------------------------------

// GetAnchor returns the stored SHA-256 anchor for docName.
// Returns ("", "", nil) if no anchor exists yet.
func (p *PostgresStore) GetAnchor(ctx context.Context, docName string) (hash string, amendmentID string, err error) {
	row := p.pool.QueryRow(ctx,
		`SELECT sha256_hash, COALESCE(amendment_id, '') FROM identity_anchors WHERE doc_name = $1`,
		docName,
	)
	err = row.Scan(&hash, &amendmentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", nil
		}
		return "", "", fmt.Errorf("platform: getting anchor for %q: %w", docName, err)
	}
	return hash, amendmentID, nil
}

// UpsertAnchor sets or updates the anchor for docName.
func (p *PostgresStore) UpsertAnchor(ctx context.Context, docName, hash, amendmentID string) error {
	var amendArg interface{} = nil
	if amendmentID != "" {
		amendArg = amendmentID
	}
	_, err := p.pool.Exec(ctx,
		`INSERT INTO identity_anchors (doc_name, sha256_hash, amendment_id, verified_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT(doc_name) DO UPDATE SET
			sha256_hash  = EXCLUDED.sha256_hash,
			amendment_id = EXCLUDED.amendment_id,
			verified_at  = now()`,
		docName, hash, amendArg,
	)
	if err != nil {
		return fmt.Errorf("platform: upserting anchor for %q: %w", docName, err)
	}
	return nil
}

// GetAmendmentByID returns the amendment record with the given id, or nil if not found.
func (p *PostgresStore) GetAmendmentByID(ctx context.Context, id string) (*pkg.AmendmentRecord, error) {
	row := p.pool.QueryRow(ctx,
		`SELECT id, doc_name, old_hash, new_hash, reason, co_signer, created_at
		 FROM identity_amendments WHERE id = $1`, id,
	)
	var rec pkg.AmendmentRecord
	err := row.Scan(&rec.ID, &rec.DocName, &rec.OldHash, &rec.NewHash,
		&rec.Reason, &rec.CoSigner, &rec.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("platform: getting amendment %q: %w", id, err)
	}
	return &rec, nil
}

// InsertAmendment inserts a new amendment record and returns the generated ID.
func (p *PostgresStore) InsertAmendment(ctx context.Context, rec pkg.AmendmentRecord) (string, error) {
	if rec.ID == "" {
		rec.ID = newPlatformID()
	}
	_, err := p.pool.Exec(ctx,
		`INSERT INTO identity_amendments
			(id, doc_name, old_hash, new_hash, reason, co_signer, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now())`,
		rec.ID, rec.DocName, rec.OldHash, rec.NewHash, rec.Reason, rec.CoSigner,
	)
	if err != nil {
		return "", fmt.Errorf("platform: inserting amendment: %w", err)
	}
	return rec.ID, nil
}

// ListAmendments returns all amendment records for docName, newest first.
func (p *PostgresStore) ListAmendments(ctx context.Context, docName string) ([]pkg.AmendmentRecord, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, doc_name, old_hash, new_hash, reason, co_signer, created_at
		 FROM identity_amendments WHERE doc_name = $1
		 ORDER BY created_at DESC`,
		docName,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: listing amendments for %q: %w", docName, err)
	}
	defer rows.Close()

	var out []pkg.AmendmentRecord
	for rows.Next() {
		var rec pkg.AmendmentRecord
		if err := rows.Scan(&rec.ID, &rec.DocName, &rec.OldHash, &rec.NewHash,
			&rec.Reason, &rec.CoSigner, &rec.CreatedAt); err != nil {
			return nil, fmt.Errorf("platform: scanning amendment row: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (p *PostgresStore) ListAnchoredDocs(ctx context.Context) ([]string, error) {
	rows, err := p.pool.Query(ctx, `SELECT doc_name FROM identity_anchors`)
	if err != nil {
		return nil, fmt.Errorf("platform: listing anchored docs: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("platform: scanning anchored doc: %w", err)
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

// ---------------------------------------------------------------------------
// PostgresStore — pkg.SubstrateStore
// ---------------------------------------------------------------------------

// InsertSubstrateTransition records a new substrate transition.
func (p *PostgresStore) InsertSubstrateTransition(ctx context.Context, entry pkg.SubstrateTransition) error {
	var prevID interface{} = nil
	if entry.PreviousEntryID != "" {
		prevID = entry.PreviousEntryID
	}
	_, err := p.pool.Exec(ctx,
		`INSERT INTO substrate_transitions
			(id, model_name, model_version, transition_date, continuity_letter_path, previous_entry_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now())`,
		entry.ID, entry.ModelName, entry.ModelVersion,
		entry.TransitionDate.UTC(), entry.ContinuityLetterPath, prevID,
	)
	if err != nil {
		return fmt.Errorf("platform: inserting substrate transition: %w", err)
	}
	return nil
}

// GetLatestSubstrate returns the most recent substrate transition, or (nil, nil) if empty.
func (p *PostgresStore) GetLatestSubstrate(ctx context.Context) (*pkg.SubstrateTransition, error) {
	row := p.pool.QueryRow(ctx,
		`SELECT id, model_name, model_version, transition_date,
			continuity_letter_path, COALESCE(previous_entry_id, ''), created_at
		 FROM substrate_transitions ORDER BY transition_date DESC LIMIT 1`,
	)
	var e pkg.SubstrateTransition
	err := row.Scan(&e.ID, &e.ModelName, &e.ModelVersion, &e.TransitionDate,
		&e.ContinuityLetterPath, &e.PreviousEntryID, &e.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("platform: scanning substrate row: %w", err)
	}
	return &e, nil
}

// ListSubstrateHistory returns all substrate transitions, oldest first.
func (p *PostgresStore) ListSubstrateHistory(ctx context.Context) ([]pkg.SubstrateTransition, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, model_name, model_version, transition_date,
			continuity_letter_path, COALESCE(previous_entry_id, ''), created_at
		 FROM substrate_transitions ORDER BY transition_date ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: listing substrate history: %w", err)
	}
	defer rows.Close()

	var out []pkg.SubstrateTransition
	for rows.Next() {
		var e pkg.SubstrateTransition
		if err := rows.Scan(&e.ID, &e.ModelName, &e.ModelVersion, &e.TransitionDate,
			&e.ContinuityLetterPath, &e.PreviousEntryID, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("platform: scanning substrate row: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Compile-time interface satisfaction checks.
var _ pkg.MemoryStore = (*SQLiteStore)(nil)
var _ pkg.MemoryStore = (*PostgresStore)(nil)
var _ pkg.EdgeStore = (*SQLiteStore)(nil)
var _ pkg.EdgeStore = (*PostgresStore)(nil)
var _ pkg.BeliefStore = (*SQLiteStore)(nil)
var _ pkg.BeliefStore = (*PostgresStore)(nil)
var _ pkg.KGMutationStore = (*SQLiteStore)(nil)
var _ pkg.KGMutationStore = (*PostgresStore)(nil)
var _ pkg.T2QueryStore = (*SQLiteStore)(nil)
var _ pkg.T2QueryStore = (*PostgresStore)(nil)
var _ pkg.IdentityAnchorStore = (*SQLiteStore)(nil)
var _ pkg.IdentityAnchorStore = (*PostgresStore)(nil)
var _ pkg.SubstrateStore = (*SQLiteStore)(nil)
var _ pkg.SubstrateStore = (*PostgresStore)(nil)
