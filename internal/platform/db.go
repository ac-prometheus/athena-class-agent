package platform

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
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

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

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
