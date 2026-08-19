package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// SQLiteJobStore implements pkg.MetabolismJobStore for SQLite.
type SQLiteJobStore struct {
	db platform.DB
}

// NewSQLiteJobStore returns a MetabolismJobStore backed by SQLite.
func NewSQLiteJobStore(db platform.DB) *SQLiteJobStore {
	return &SQLiteJobStore{db: db}
}

func (s *SQLiteJobStore) Commit(ctx context.Context, sessionID, jobType string) (string, error) {
	id := newID()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO metabolism_jobs (id, session_id, status, job_type, created_at)
		 VALUES (?, ?, 'pending', ?, CURRENT_TIMESTAMP)`,
		id, sessionID, jobType,
	)
	if err != nil {
		return "", fmt.Errorf("storage: committing job for session %s: %w", sessionID, err)
	}
	return id, nil
}

func (s *SQLiteJobStore) Claim(ctx context.Context, jobID string, staleDuration time.Duration) (string, error) {
	claimToken := newID()
	staleThreshold := fmt.Sprintf("-%d seconds", int(staleDuration.Seconds()))
	const maxClaims = 5
	result, err := s.db.ExecContext(ctx,
		`UPDATE metabolism_jobs
		 SET status = 'running', started_at = CURRENT_TIMESTAMP,
		     claim_token = ?, claim_count = claim_count + 1
		 WHERE id = ? AND claim_count < ? AND (
		     status = 'pending' OR
		     status = 'failed' OR
		     (status = 'running' AND started_at < datetime('now', ?))
		 )`,
		claimToken, jobID, maxClaims, staleThreshold,
	)
	if err != nil {
		return "", fmt.Errorf("storage: claiming job %s: %w", jobID, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("storage: checking claim result for job %s: %w", jobID, err)
	}
	if n == 0 {
		return "", pkg.ErrJobNotPending
	}
	return claimToken, nil
}

func (s *SQLiteJobStore) Complete(ctx context.Context, jobID, claimToken string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE metabolism_jobs
		 SET status = 'completed', completed_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND claim_token = ?`,
		jobID, claimToken,
	)
	if err != nil {
		return fmt.Errorf("storage: completing job %s: %w", jobID, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return s.fencedOrNotFound(ctx, jobID)
	}
	return nil
}

func (s *SQLiteJobStore) Fail(ctx context.Context, jobID, claimToken, cause string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE metabolism_jobs
		 SET status = 'failed', error_message = ?, retry_count = retry_count + 1
		 WHERE id = ? AND claim_token = ?`,
		cause, jobID, claimToken,
	)
	if err != nil {
		return fmt.Errorf("storage: failing job %s: %w", jobID, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return s.fencedOrNotFound(ctx, jobID)
	}
	return nil
}

func (s *SQLiteJobStore) MarkReviewRequired(ctx context.Context, jobID, claimToken, reason string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE metabolism_jobs
		 SET status = 'review_required', error_message = ?, completed_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND claim_token = ?`,
		reason, jobID, claimToken,
	)
	if err != nil {
		return fmt.Errorf("storage: marking job %s review_required: %w", jobID, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return s.fencedOrNotFound(ctx, jobID)
	}
	return nil
}

func (s *SQLiteJobStore) fencedOrNotFound(ctx context.Context, jobID string) error {
	var exists bool
	_ = s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM metabolism_jobs WHERE id = ?)", jobID).Scan(&exists)
	if !exists {
		return fmt.Errorf("storage: job %s not found", jobID)
	}
	return pkg.ErrClaimFenced
}

func (s *SQLiteJobStore) Recoverable(ctx context.Context, maxRetries int) ([]pkg.RecoverableJob, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, retry_count
		 FROM metabolism_jobs
		 WHERE status IN ('pending', 'running', 'failed') AND retry_count < ?
		 ORDER BY created_at ASC`,
		maxRetries,
	)
	if err != nil {
		return nil, fmt.Errorf("storage: querying recoverable jobs: %w", err)
	}
	defer rows.Close()

	var jobs []pkg.RecoverableJob
	for rows.Next() {
		var j pkg.RecoverableJob
		if err := rows.Scan(&j.JobID, &j.SessionID, &j.RetryCount); err != nil {
			return nil, fmt.Errorf("storage: scanning recoverable job: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (s *SQLiteJobStore) LastStatus(ctx context.Context) (string, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT status FROM metabolism_jobs ORDER BY created_at DESC LIMIT 1`)
	var status string
	if err := row.Scan(&status); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("storage: querying last job status: %w", err)
	}
	return status, nil
}
