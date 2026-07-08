package platform

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB abstracts database query/exec operations for both *sql.DB (SQLite) and
// pgxpool.Pool (Postgres). Callers should also carry a driver name ("sqlite3"
// or "postgres") for dialect-specific SQL generation.
type DB interface {
	QueryContext(ctx context.Context, query string, args ...any) (Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) Row
}

// Rows is a minimal row-iterator interface satisfied by *sql.Rows and the pgx
// adapter below.
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// Row is a minimal single-row interface satisfied by *sql.Row and the pgx
// adapter below.
type Row interface {
	Scan(dest ...any) error
}

// ---------------------------------------------------------------------------
// sqlDB wraps *sql.DB to implement platform.DB.
// *sql.DB already has QueryContext/ExecContext/QueryRowContext, but their
// return types (*sql.Rows / *sql.Row) need to be wrapped to satisfy our
// interfaces.
// ---------------------------------------------------------------------------

type sqlDB struct{ db *sql.DB }

// WrapSQLDB wraps a *sql.DB to satisfy platform.DB.
func WrapSQLDB(db *sql.DB) DB { return &sqlDB{db} }

func (s *sqlDB) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *sqlDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, query, args...)
}

func (s *sqlDB) QueryRowContext(ctx context.Context, query string, args ...any) Row {
	return s.db.QueryRowContext(ctx, query, args...)
}

// ---------------------------------------------------------------------------
// pgxDB wraps *pgxpool.Pool to implement platform.DB.
// pgx returns its own pgx.Rows / pgx.Row types; we adapt them here.
// ---------------------------------------------------------------------------

type pgxDB struct{ pool *pgxpool.Pool }

// WrapPgxPool wraps a *pgxpool.Pool to satisfy platform.DB.
func WrapPgxPool(pool *pgxpool.Pool) DB { return &pgxDB{pool} }

func (p *pgxDB) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &pgxRowsAdapter{rows: rows}, nil
}

func (p *pgxDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	tag, err := p.pool.Exec(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgxResult(tag.RowsAffected()), nil
}

func (p *pgxDB) QueryRowContext(ctx context.Context, query string, args ...any) Row {
	return &pgxRowAdapter{row: p.pool.QueryRow(ctx, query, args...)}
}

// pgxRowsAdapter adapts pgx.Rows to platform.Rows.
type pgxRowsAdapter struct {
	rows pgx.Rows
}

func (r *pgxRowsAdapter) Next() bool        { return r.rows.Next() }
func (r *pgxRowsAdapter) Scan(dest ...any) error { return r.rows.Scan(dest...) }
func (r *pgxRowsAdapter) Close() error      { r.rows.Close(); return nil }
func (r *pgxRowsAdapter) Err() error        { return r.rows.Err() }

// pgxRowAdapter adapts pgx.Row to platform.Row.
type pgxRowAdapter struct {
	row pgx.Row
}

func (r *pgxRowAdapter) Scan(dest ...any) error { return r.row.Scan(dest...) }

// pgxResult is a sql.Result backed by a pgx rows-affected count.
type pgxResult int64

func (r pgxResult) LastInsertId() (int64, error) { return 0, nil }
func (r pgxResult) RowsAffected() (int64, error) { return int64(r), nil }
