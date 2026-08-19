package metabolism

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// JobRunner owns the full metabolism job lifecycle: commit → claim →
// process → complete/fail. Both new dispatch and crash recovery go
// through the same Run path, so transition correctness lives in one
// place instead of being scattered across main.go, daemon, and recovery.
type JobRunner struct {
	store    pkg.MetabolismJobStore
	pipeline *Pipeline
	logger   *slog.Logger
}

// NewJobRunner creates a runner. Both store and pipeline are required —
// without a store there's no durability, without a pipeline there's
// nothing to run.
func NewJobRunner(store pkg.MetabolismJobStore, pipeline *Pipeline, logger *slog.Logger) *JobRunner {
	if logger == nil {
		logger = slog.Default()
	}
	return &JobRunner{
		store:    store,
		pipeline: pipeline,
		logger:   logger,
	}
}

// Submit commits a durable job record, then dispatches Run in a
// background goroutine. The durable record exists before the goroutine
// starts — if the process crashes between commit and completion,
// RecoverStalled will find and resubmit it.
func (r *JobRunner) Submit(ctx context.Context, sessionID, jobType string) error {
	jobID, err := r.store.Commit(ctx, sessionID, jobType)
	if err != nil {
		return fmt.Errorf("jobrunner: commit: %w", err)
	}

	r.logger.Info("jobrunner: job committed, dispatching",
		"job_id", jobID, "session_id", sessionID, "job_type", jobType)

	go func() {
		// Use the caller's ctx so daemon shutdown cancels the goroutine.
		// The job is already durably committed — if the process exits before
		// Run completes, RecoverStalled will pick it up on next startup.
		if err := r.Run(ctx, jobID, sessionID); err != nil {
			r.logger.Error("jobrunner: background run failed",
				"job_id", jobID, "session_id", sessionID, "err", err)
		}
	}()

	return nil
}

// Run executes the full job lifecycle: claim → process → complete/fail.
// If the job is no longer pending (already claimed by another worker or
// recovered), Run returns nil — the job is someone else's responsibility.
//
// Both Submit (new work) and RecoverStalled (crash recovery) funnel
// through this method, so the state machine lives in exactly one place.
func (r *JobRunner) Run(ctx context.Context, jobID, sessionID string) error {
	if err := r.store.Claim(ctx, jobID, 5*time.Minute); err != nil {
		if errors.Is(err, pkg.ErrJobNotPending) {
			r.logger.Info("jobrunner: job already claimed — skipping",
				"job_id", jobID, "session_id", sessionID)
			return nil
		}
		return fmt.Errorf("jobrunner: claim %s: %w", jobID, err)
	}

	r.logger.Info("jobrunner: processing",
		"job_id", jobID, "session_id", sessionID)

	if err := r.pipeline.ProcessSession(ctx, sessionID); err != nil {
		// ErrReviewRequired is not a crash — external content needs human
		// review before it can be promoted. Mark the job review_required
		// instead of failed so it won't be retried automatically.
		if errors.Is(err, ErrReviewRequired) {
			r.logger.Info("jobrunner: pipeline requires review — marking review_required",
				"job_id", jobID, "session_id", sessionID)
			if rErr := r.store.MarkReviewRequired(ctx, jobID, err.Error()); rErr != nil {
				r.logger.Error("jobrunner: failed to record review_required",
					"job_id", jobID, "err", rErr)
			}
			return nil // not an error — the job stopped intentionally
		}

		r.logger.Error("jobrunner: pipeline failed — marking job failed",
			"job_id", jobID, "session_id", sessionID, "err", err)
		if fErr := r.store.Fail(ctx, jobID, err.Error()); fErr != nil {
			r.logger.Error("jobrunner: failed to record failure",
				"job_id", jobID, "err", fErr)
		}
		return fmt.Errorf("jobrunner: process %s: %w", jobID, err)
	}

	if err := r.store.Complete(ctx, jobID); err != nil {
		r.logger.Error("jobrunner: failed to record completion",
			"job_id", jobID, "err", err)
		return fmt.Errorf("jobrunner: complete %s: %w", jobID, err)
	}

	r.logger.Info("jobrunner: completed",
		"job_id", jobID, "session_id", sessionID)
	return nil
}

// RecoverStalled scans for jobs left in pending or running state (from
// a previous crash) and resubmits each through Run. Jobs that have
// exceeded maxRetries are left for the store's Recoverable filter to
// exclude.
//
// Returns the number of jobs resubmitted. Each runs in its own
// goroutine through the same claim → process → complete/fail path
// as new work.
func (r *JobRunner) RecoverStalled(ctx context.Context, maxRetries int) (int, error) {
	jobs, err := r.store.Recoverable(ctx, maxRetries)
	if err != nil {
		return 0, fmt.Errorf("jobrunner: recovery scan: %w", err)
	}

	if len(jobs) == 0 {
		return 0, nil
	}

	r.logger.Info("jobrunner: recovering stalled jobs", "count", len(jobs))

	for _, rj := range jobs {
		rj := rj
		go func() {
			if err := r.Run(ctx, rj.JobID, rj.SessionID); err != nil {
				r.logger.Error("jobrunner: recovery run failed",
					"job_id", rj.JobID, "session_id", rj.SessionID, "err", err)
			}
		}()
	}

	return len(jobs), nil
}
