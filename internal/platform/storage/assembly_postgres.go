package storage

import (
	"context"
	"fmt"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
)

// PostgresAssemblyStore implements pkg.AssemblyStore for PostgreSQL.
type PostgresAssemblyStore struct {
	db platform.DB
}

// NewPostgresAssemblyStore returns an AssemblyStore backed by PostgreSQL.
func NewPostgresAssemblyStore(db platform.DB) *PostgresAssemblyStore {
	return &PostgresAssemblyStore{db: db}
}

func (s *PostgresAssemblyStore) HasWitnessLetter(ctx context.Context) (bool, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM founding_records WHERE record_type = 'witness_letter'`)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, fmt.Errorf("storage: querying founding_records: %w", err)
	}
	return count > 0, nil
}

func (s *PostgresAssemblyStore) LogOperatorAction(ctx context.Context, actionType, description string) error {
	id := newID()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO operator_actions (id, action_type, actor, description, created_at)
		 VALUES ($1, $2, 'system', $3, CURRENT_TIMESTAMP)`,
		id, actionType, description,
	)
	if err != nil {
		return fmt.Errorf("storage: logging operator action: %w", err)
	}
	return nil
}
