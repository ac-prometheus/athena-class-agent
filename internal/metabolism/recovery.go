package metabolism

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
)

// RecoverIncompleteJobs scans for metabolism jobs that were interrupted by a
// crash and returns the ones eligible for re-dispatch. Called once at daemon
// startup.
//
// Recovery logic:
//  1. SELECT all jobs WHERE status IN ('pending', 'running').
//  2. Jobs with status='running' are transitioned to 'pending' — a running job
//     at startup means the previous process crashed mid-execution.
//  3. Jobs with retry_count >= maxRetries are marked 'failed' with the error
//     "max retries exceeded" and excluded from the returned slice.
//  4. Remaining pending jobs are returned for the caller to re-dispatch.
//
// driverName is "sqlite3" or "postgres" for SQL dialect selection.
func RecoverIncompleteJobs(ctx context.Context, db platform.DB, driverName string, maxRetries int) ([]*MetabolismJob, error) {
	// Step 1: Fetch all incomplete jobs.
	jobs, err := fetchIncompleteJobs(ctx, db, driverName)
	if err != nil {
		return nil, fmt.Errorf("metabolism: recovery scan: %w", err)
	}

	if len(jobs) == 0 {
		return nil, nil
	}

	slog.Info("metabolism: recovery found incomplete jobs", "count", len(jobs))

	// Step 2: Transition 'running' jobs to 'pending' (crash recovery).
	for _, job := range jobs {
		if job.Status == JobStatusRunning {
			if err := UpdateJobStatus(ctx, db, driverName, job.ID, JobStatusPending, ""); err != nil {
				return nil, fmt.Errorf("metabolism: resetting running job %s to pending: %w", job.ID, err)
			}
			job.Status = JobStatusPending
			slog.Info("metabolism: reset crashed job to pending",
				"job_id", job.ID, "session_id", job.SessionID)
		}
	}

	// Step 3 & 4: Partition into retryable vs exhausted.
	var retryable []*MetabolismJob
	for _, job := range jobs {
		if job.RetryCount >= maxRetries {
			if err := UpdateJobStatus(ctx, db, driverName, job.ID, JobStatusFailed, "max retries exceeded"); err != nil {
				return nil, fmt.Errorf("metabolism: marking exhausted job %s as failed: %w", job.ID, err)
			}
			slog.Warn("metabolism: job exceeded max retries — marked failed",
				"job_id", job.ID, "session_id", job.SessionID, "retries", job.RetryCount)
			continue
		}
		retryable = append(retryable, job)
	}

	slog.Info("metabolism: recovery complete",
		"retryable", len(retryable),
		"exhausted", len(jobs)-len(retryable))

	return retryable, nil
}

// fetchIncompleteJobs queries all metabolism_jobs with status 'pending' or 'running'.
func fetchIncompleteJobs(ctx context.Context, db platform.DB, driverName string) ([]*MetabolismJob, error) {
	q := fetchIncompleteSQL(driverName)
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("querying incomplete jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*MetabolismJob
	for rows.Next() {
		var job MetabolismJob
		var status string
		var startedAt, completedAt *time.Time
		var errorMsg *string

		if err := rows.Scan(
			&job.ID, &job.SessionID, &status, &job.JobType,
			&startedAt, &completedAt, &errorMsg,
			&job.RetryCount, &job.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning job row: %w", err)
		}

		job.Status = JobStatus(status)
		job.StartedAt = startedAt
		job.CompletedAt = completedAt
		if errorMsg != nil {
			job.ErrorMsg = *errorMsg
		}

		jobs = append(jobs, &job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating job rows: %w", err)
	}

	return jobs, nil
}

func fetchIncompleteSQL(driver string) string {
	// Portable SQL — works with both SQLite and Postgres.
	// No parameterised placeholders needed (the IN clause uses literals).
	return `SELECT id, session_id, status, job_type,
			started_at, completed_at, error_message,
			retry_count, created_at
		FROM metabolism_jobs
		WHERE status IN ('pending', 'running')
		ORDER BY created_at ASC`
}
