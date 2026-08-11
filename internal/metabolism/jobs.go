package metabolism

import (
	"context"
	"fmt"
	"time"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
)

// JobStatus represents the state of a metabolism job.
// Values match the CHECK constraint in schema/009_lifecycle.sql:
//
//	CHECK (status IN ('pending', 'running', 'completed', 'failed', 'interrupted'))
type JobStatus string

const (
	JobStatusPending        JobStatus = "pending"
	JobStatusRunning        JobStatus = "running"
	JobStatusCompleted      JobStatus = "completed"
	JobStatusFailed         JobStatus = "failed"
	JobStatusInterrupted    JobStatus = "interrupted"
	JobStatusReviewRequired JobStatus = "review_required"
)

// MetabolismJob is the in-memory representation of a metabolism_jobs row.
type MetabolismJob struct {
	ID          string
	SessionID   string
	Status      JobStatus
	JobType     string
	StartedAt   *time.Time
	CompletedAt *time.Time
	ErrorMsg    string
	RetryCount  int
	CreatedAt   time.Time
}

// CommitJob writes a new metabolism job record with status=pending.
// This MUST be called and committed BEFORE dispatching the goroutine.
// If the process crashes after CommitJob but before the goroutine completes,
// startup recovery (RecoverIncompleteJobs) will rediscover the job.
//
// driverName is "sqlite3" or "postgres" for SQL dialect selection.
func CommitJob(ctx context.Context, db platform.DB, driverName string, sessionID, jobType string) (*MetabolismJob, error) {
	job := &MetabolismJob{
		ID:        newMetabolismID(),
		SessionID: sessionID,
		Status:    JobStatusPending,
		JobType:   jobType,
		CreatedAt: time.Now().UTC(),
	}

	q := commitJobSQL(driverName)
	if _, err := db.ExecContext(ctx, q,
		job.ID, job.SessionID, string(job.Status), job.JobType,
	); err != nil {
		return nil, fmt.Errorf("metabolism: committing job for session %s: %w", sessionID, err)
	}
	return job, nil
}

// UpdateJobStatus transitions a job to a new status. For terminal states
// (completed, failed), prefer CompleteJob or FailJob which also set timestamps
// and error messages.
func UpdateJobStatus(ctx context.Context, db platform.DB, driverName string, jobID string, status JobStatus, errMsg string) error {
	q := updateJobStatusSQL(driverName)
	if _, err := db.ExecContext(ctx, q, string(status), errMsg, jobID); err != nil {
		return fmt.Errorf("metabolism: updating job %s to %s: %w", jobID, status, err)
	}
	return nil
}

// CompleteJob marks a job as completed with a timestamp.
func CompleteJob(ctx context.Context, db platform.DB, driverName string, jobID string) error {
	q := completeJobSQL(driverName)
	if _, err := db.ExecContext(ctx, q, jobID); err != nil {
		return fmt.Errorf("metabolism: completing job %s: %w", jobID, err)
	}
	return nil
}

// FailJob marks a job as failed with an error message and increments retry_count.
func FailJob(ctx context.Context, db platform.DB, driverName string, jobID string, errMsg string) error {
	q := failJobSQL(driverName)
	if _, err := db.ExecContext(ctx, q, errMsg, jobID); err != nil {
		return fmt.Errorf("metabolism: failing job %s: %w", jobID, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// SQL builders (dialect-aware)
// ---------------------------------------------------------------------------

func commitJobSQL(driver string) string {
	if driver == "postgres" {
		return `INSERT INTO metabolism_jobs (id, session_id, status, job_type, created_at)
			VALUES ($1, $2, $3, $4, now())`
	}
	return `INSERT INTO metabolism_jobs (id, session_id, status, job_type, created_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`
}

func updateJobStatusSQL(driver string) string {
	if driver == "postgres" {
		return `UPDATE metabolism_jobs SET status = $1, error_message = $2 WHERE id = $3`
	}
	return `UPDATE metabolism_jobs SET status = ?, error_message = ? WHERE id = ?`
}

func completeJobSQL(driver string) string {
	if driver == "postgres" {
		return `UPDATE metabolism_jobs SET status = 'completed', completed_at = now() WHERE id = $1`
	}
	return `UPDATE metabolism_jobs SET status = 'completed', completed_at = CURRENT_TIMESTAMP WHERE id = ?`
}

func failJobSQL(driver string) string {
	if driver == "postgres" {
		return `UPDATE metabolism_jobs
			SET status = 'failed', error_message = $1, retry_count = retry_count + 1
			WHERE id = $2`
	}
	return `UPDATE metabolism_jobs
		SET status = 'failed', error_message = ?, retry_count = retry_count + 1
		WHERE id = ?`
}
