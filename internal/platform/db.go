package platform

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

// Store wraps a *sql.DB with the DSN it was opened from.
type Store struct {
	DB      *sql.DB
	Driver  string // "sqlite3" or "postgres"
	DSN     string
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

	return &Store{DB: db, Driver: driver, DSN: rawDSN}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.DB.Close()
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
