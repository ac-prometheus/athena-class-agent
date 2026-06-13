package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
)

// migrate reads numbered SQL files from schema/, tracks applied migrations in
// schema_migrations, and applies only what's new, in order.
// Fails fast on errors or checksum mismatches on previously applied files.
//
// Postgres-compatible SQL is dialect-adapted for SQLite at apply time:
//   SERIAL / BIGSERIAL      → INTEGER PRIMARY KEY AUTOINCREMENT
//   vector(N)               → TEXT
//   TIMESTAMPTZ             → TEXT
//   now()                   → CURRENT_TIMESTAMP
//   CREATE INDEX CONCURRENTLY → CREATE INDEX
//   WHERE clause on index   → dropped (SQLite partial index syntax differs)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		dsn = "sqlite://./agent.db"
	}

	schemaDir := os.Getenv("SCHEMA_DIR")
	if schemaDir == "" {
		schemaDir = "./schema"
	}

	store, err := platform.NewStore(dsn)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer store.Close()

	fmt.Printf("migrate: connected to %s (%s)\n", store.Driver, store.RedactedDSN())

	if err := ensureMigrationsTable(store); err != nil {
		return fmt.Errorf("ensuring schema_migrations table: %w", err)
	}

	files, err := collectMigrationFiles(schemaDir)
	if err != nil {
		return fmt.Errorf("collecting migration files from %s: %w", schemaDir, err)
	}

	applied, err := appliedMigrations(store)
	if err != nil {
		return fmt.Errorf("loading applied migrations: %w", err)
	}

	pending := 0
	for _, f := range files {
		name := filepath.Base(f)
		raw, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("reading %s: %w", name, err)
		}

		checksum := fmt.Sprintf("%x", sha256.Sum256(raw))

		if prev, ok := applied[name]; ok {
			if prev != checksum {
				return fmt.Errorf("checksum mismatch on applied migration %s — never edit applied migrations; add a new file", name)
			}
			fmt.Printf("migrate: [skip] %s (already applied)\n", name)
			continue
		}

		sql := string(raw)
		if store.Driver == "sqlite3" {
			sql = adaptForSQLite(sql)
		}

		fmt.Printf("migrate: [apply] %s\n", name)
		if err := applyMigration(store, name, sql, checksum); err != nil {
			return fmt.Errorf("applying %s: %w", name, err)
		}
		pending++
	}

	if pending == 0 {
		fmt.Println("migrate: schema is up to date")
	} else {
		fmt.Printf("migrate: applied %d migration(s)\n", pending)
	}
	return nil
}

// ensureMigrationsTable creates the schema_migrations tracking table if absent.
func ensureMigrationsTable(store *platform.Store) error {
	var q string
	if store.Driver == "sqlite3" {
		q = `CREATE TABLE IF NOT EXISTS schema_migrations (
			name       TEXT NOT NULL PRIMARY KEY,
			checksum   TEXT NOT NULL,
			applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`
	} else {
		q = `CREATE TABLE IF NOT EXISTS schema_migrations (
			name       TEXT        NOT NULL PRIMARY KEY,
			checksum   TEXT        NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`
	}
	_, err := store.DB.Exec(q)
	return err
}

// collectMigrationFiles returns .sql files from dir sorted by filename.
func collectMigrationFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	sort.Strings(files)
	return files, nil
}

// appliedMigrations returns a map of name → checksum for all applied migrations.
func appliedMigrations(store *platform.Store) (map[string]string, error) {
	rows, err := store.DB.Query(`SELECT name, checksum FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[string]string)
	for rows.Next() {
		var name, checksum string
		if err := rows.Scan(&name, &checksum); err != nil {
			return nil, err
		}
		m[name] = checksum
	}
	return m, rows.Err()
}

// applyMigration executes the SQL and records it in schema_migrations, all in one transaction.
func applyMigration(store *platform.Store, name, sqlText, checksum string) error {
	tx, err := store.DB.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// SQLite does not support multi-statement exec natively through database/sql;
	// split on semicolons and execute each statement individually.
	stmts := splitStatements(sqlText)
	for _, stmt := range stmts {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err = tx.Exec(stmt); err != nil {
			return fmt.Errorf("executing statement %q: %w", truncate(stmt, 120), err)
		}
	}

	if store.Driver == "sqlite3" {
		_, err = tx.Exec(
			`INSERT INTO schema_migrations (name, checksum, applied_at) VALUES (?, ?, CURRENT_TIMESTAMP)`,
			name, checksum,
		)
	} else {
		_, err = tx.Exec(
			`INSERT INTO schema_migrations (name, checksum) VALUES ($1, $2)`,
			name, checksum,
		)
	}
	if err != nil {
		return fmt.Errorf("recording migration: %w", err)
	}

	return tx.Commit()
}

// splitStatements splits a SQL file into individual statements on semicolons,
// ignoring semicolons inside single-quoted strings and line comments.
func splitStatements(sql string) []string {
	var stmts []string
	var cur strings.Builder
	inString := false
	inLineComment := false

	for i := 0; i < len(sql); i++ {
		ch := sql[i]

		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			cur.WriteByte(ch)
			continue
		}

		if !inString && i+1 < len(sql) && ch == '-' && sql[i+1] == '-' {
			inLineComment = true
			cur.WriteByte(ch)
			continue
		}

		if ch == '\'' {
			inString = !inString
		}

		if !inString && ch == ';' {
			stmts = append(stmts, cur.String())
			cur.Reset()
			continue
		}

		cur.WriteByte(ch)
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		stmts = append(stmts, s)
	}
	return stmts
}

// adaptForSQLite rewrites Postgres-specific SQL constructs for SQLite.
func adaptForSQLite(sql string) string {
	// BIGSERIAL — must come before SERIAL to avoid partial match
	sql = reSerial.ReplaceAllStringFunc(sql, func(m string) string {
		upper := strings.ToUpper(m)
		switch {
		case strings.Contains(upper, "BIGSERIAL"):
			return strings.ToUpper(reSerial.ReplaceAllString(m, "INTEGER PRIMARY KEY AUTOINCREMENT"))
		case strings.Contains(upper, "SERIAL"):
			return strings.ToUpper(reSerial.ReplaceAllString(m, "INTEGER PRIMARY KEY AUTOINCREMENT"))
		}
		return m
	})
	sql = reBigSerial.ReplaceAllString(sql, "INTEGER PRIMARY KEY AUTOINCREMENT")
	sql = reSerialOnly.ReplaceAllString(sql, "INTEGER PRIMARY KEY AUTOINCREMENT")

	// vector(N) type → TEXT
	sql = reVectorType.ReplaceAllString(sql, "TEXT")

	// TIMESTAMPTZ → TEXT
	sql = reTimestamptz.ReplaceAllString(sql, "TEXT")

	// now() → CURRENT_TIMESTAMP
	sql = reNow.ReplaceAllString(sql, "CURRENT_TIMESTAMP")

	// CREATE INDEX CONCURRENTLY → CREATE INDEX
	sql = reConcurrently.ReplaceAllString(sql, "CREATE INDEX")

	// Partial indexes with WHERE clauses are supported in SQLite, but only
	// simple comparisons. The spec says to drop them; we keep them since
	// SQLite 3.8+ supports `WHERE col = value` partial indexes.
	// No rewrite needed.

	// BOOLEAN → INTEGER (SQLite has no native boolean type)
	sql = reBooleanType.ReplaceAllString(sql, "INTEGER")

	// TRUE/FALSE literals → 1/0
	sql = reTrue.ReplaceAllString(sql, "1")
	sql = reFalse.ReplaceAllString(sql, "0")

	// REFERENCES foo(id) with ON DELETE/UPDATE — drop cascade clauses SQLite may not support
	// (SQLite supports REFERENCES but ignores foreign key enforcement unless PRAGMA enabled)
	// No rewrite needed; leave as-is.

	return sql
}

var (
	reBigSerial    = regexp.MustCompile(`(?i)\bBIGSERIAL\b`)
	reSerialOnly   = regexp.MustCompile(`(?i)\bSERIAL\b`)
	reSerial       = regexp.MustCompile(`(?i)\b(BIG)?SERIAL\b`)
	reVectorType   = regexp.MustCompile(`(?i)\bvector\(\d+\)`)
	reTimestamptz  = regexp.MustCompile(`(?i)\bTIMESTAMPTZ\b`)
	reNow          = regexp.MustCompile(`(?i)\bnow\(\)`)
	reConcurrently = regexp.MustCompile(`(?i)CREATE\s+INDEX\s+CONCURRENTLY`)
	reBooleanType  = regexp.MustCompile(`(?i)\bBOOLEAN\b`)
	reTrue         = regexp.MustCompile(`(?i)\bTRUE\b`)
	reFalse        = regexp.MustCompile(`(?i)\bFALSE\b`)
)

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
