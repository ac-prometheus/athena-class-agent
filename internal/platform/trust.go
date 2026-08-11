package platform

// Trust score persistence.
// Split from db.go for readability. Methods on SQLiteStore and PostgresStore.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ac-prometheus/athena-class-agent/pkg"
	"github.com/jackc/pgx/v5"
)

func (s *SQLiteStore) GetTrust(ctx context.Context, source string) (float64, int, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT trust_score, interactions FROM trust_registry WHERE source = ?`,
		source,
	)
	var score float64
	var interactions int
	if err := row.Scan(&score, &interactions); err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, fmt.Errorf("%w", pkg.ErrTrustNotFound)
		}
		return 0, 0, fmt.Errorf("platform: getting trust for %q: %w", source, err)
	}
	return score, interactions, nil
}

// UpdateTrust sets the trust_score for source, inserting a row if it does not exist.
func (s *SQLiteStore) UpdateTrust(ctx context.Context, source string, score float64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO trust_registry (source, trust_score, interactions, first_seen, last_seen)
		 VALUES (?, ?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		 ON CONFLICT(source) DO UPDATE SET
			trust_score = excluded.trust_score,
			last_seen   = CURRENT_TIMESTAMP`,
		source, score,
	)
	if err != nil {
		return fmt.Errorf("platform: updating trust for %q: %w", source, err)
	}
	return nil
}

// IncrementInteractions increments the interaction counter for source,
// inserting a row with the skeptical prior if it does not exist.
func (s *SQLiteStore) IncrementInteractions(ctx context.Context, source string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO trust_registry (source, trust_score, interactions, first_seen, last_seen)
		 VALUES (?, 0.40, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		 ON CONFLICT(source) DO UPDATE SET
			interactions = interactions + 1,
			last_seen    = CURRENT_TIMESTAMP`,
		source,
	)
	if err != nil {
		return fmt.Errorf("platform: incrementing interactions for %q: %w", source, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// PostgresStore — pkg.TrustStore
// ---------------------------------------------------------------------------

// GetTrust returns the trust score and interaction count for a source.
// Returns ErrTrustNotFound (wrapped) if the source has no record.
func (p *PostgresStore) GetTrust(ctx context.Context, source string) (float64, int, error) {
	row := p.pool.QueryRow(ctx,
		`SELECT trust_score, interactions FROM trust_registry WHERE source = $1`,
		source,
	)
	var score float64
	var interactions int
	if err := row.Scan(&score, &interactions); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, fmt.Errorf("%w", pkg.ErrTrustNotFound)
		}
		return 0, 0, fmt.Errorf("platform: getting trust for %q: %w", source, err)
	}
	return score, interactions, nil
}

// UpdateTrust sets the trust_score for source, inserting a row if it does not exist.
func (p *PostgresStore) UpdateTrust(ctx context.Context, source string, score float64) error {
	_, err := p.pool.Exec(ctx,
		`INSERT INTO trust_registry (source, trust_score, interactions, first_seen, last_seen)
		 VALUES ($1, $2, 0, now(), now())
		 ON CONFLICT(source) DO UPDATE SET
			trust_score = EXCLUDED.trust_score,
			last_seen   = now()`,
		source, score,
	)
	if err != nil {
		return fmt.Errorf("platform: updating trust for %q: %w", source, err)
	}
	return nil
}

// IncrementInteractions increments the interaction counter for source,
// inserting a row with the skeptical prior if it does not exist.
func (p *PostgresStore) IncrementInteractions(ctx context.Context, source string) error {
	_, err := p.pool.Exec(ctx,
		`INSERT INTO trust_registry (source, trust_score, interactions, first_seen, last_seen)
		 VALUES ($1, 0.40, 1, now(), now())
		 ON CONFLICT(source) DO UPDATE SET
			interactions = trust_registry.interactions + 1,
			last_seen    = now()`,
		source,
	)
	if err != nil {
		return fmt.Errorf("platform: incrementing interactions for %q: %w", source, err)
	}
	return nil
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
var _ pkg.TrustStore = (*SQLiteStore)(nil)
var _ pkg.TrustStore = (*PostgresStore)(nil)
