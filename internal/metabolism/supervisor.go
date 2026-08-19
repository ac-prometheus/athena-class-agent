package metabolism

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// Supervisor manages bounded-concurrency dispatch of metabolism jobs.
// All job dispatch — both new submissions and crash recovery — goes
// through the supervisor so concurrency, drain, and logging are
// centralized in one place.
type Supervisor struct {
	runner  *JobRunner
	sem     chan struct{}
	wg      sync.WaitGroup
	stopped atomic.Bool
	logger  *slog.Logger
}

// NewSupervisor creates a supervisor with the given concurrency limit.
// A maxConcurrent of 0 defaults to 2.
func NewSupervisor(runner *JobRunner, maxConcurrent int, logger *slog.Logger) *Supervisor {
	if maxConcurrent <= 0 {
		maxConcurrent = 2
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Supervisor{
		runner: runner,
		sem:    make(chan struct{}, maxConcurrent),
		logger: logger,
	}
}

// Submit dispatches a new metabolism job through the runner, respecting
// the concurrency bound. The durable record is committed synchronously
// (before the goroutine starts); the claim/process/complete cycle runs
// in the background.
//
// Blocks if all concurrency slots are occupied — jobs are durable, so
// waiting is safe and preferable to unbounded goroutine growth.
func (s *Supervisor) Submit(ctx context.Context, sessionID, jobType string) error {
	if s.stopped.Load() {
		return fmt.Errorf("supervisor: draining — not accepting new jobs")
	}

	jobID, err := s.runner.store.Commit(ctx, sessionID, jobType)
	if err != nil {
		return fmt.Errorf("supervisor: commit: %w", err)
	}

	s.logger.Info("supervisor: job committed, acquiring slot",
		"job_id", jobID, "session_id", sessionID, "job_type", jobType)

	s.sem <- struct{}{} // acquire slot (blocks if full)
	s.wg.Add(1)

	go func() {
		defer s.wg.Done()
		defer func() { <-s.sem }() // release slot

		if err := s.runner.Run(ctx, jobID, sessionID); err != nil {
			s.logger.Error("supervisor: job failed",
				"job_id", jobID, "session_id", sessionID, "err", err)
		}
	}()

	return nil
}

// Recover scans for stalled jobs (pending/running from a previous crash)
// and resubmits each through the same bounded dispatch path. Returns the
// count of jobs dispatched (not necessarily claimed — claim happens inside
// the goroutine and may be skipped if another worker got there first).
func (s *Supervisor) Recover(ctx context.Context, maxRetries int) (int, error) {
	jobs, err := s.runner.store.Recoverable(ctx, maxRetries)
	if err != nil {
		return 0, fmt.Errorf("supervisor: recovery scan: %w", err)
	}

	if len(jobs) == 0 {
		return 0, nil
	}

	s.logger.Info("supervisor: recovering stalled jobs", "count", len(jobs))

	for _, rj := range jobs {
		if s.stopped.Load() {
			s.logger.Warn("supervisor: drain in progress — skipping remaining recovery",
				"remaining", len(jobs))
			break
		}

		s.sem <- struct{}{} // acquire slot
		s.wg.Add(1)

		rj := rj
		go func() {
			defer s.wg.Done()
			defer func() { <-s.sem }()

			if err := s.runner.Run(ctx, rj.JobID, rj.SessionID); err != nil {
				s.logger.Error("supervisor: recovery job failed",
					"job_id", rj.JobID, "session_id", rj.SessionID, "err", err)
			}
		}()
	}

	return len(jobs), nil
}

// Drain stops accepting new submissions and waits for active jobs to
// finish, with a hard timeout. Returns nil if all jobs completed,
// or an error if the timeout expired with jobs still running.
func (s *Supervisor) Drain(timeout time.Duration) error {
	s.stopped.Store(true)
	s.logger.Info("supervisor: draining", "timeout", timeout)

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("supervisor: drain complete")
		return nil
	case <-time.After(timeout):
		s.logger.Warn("supervisor: drain timeout — jobs still running", "timeout", timeout)
		return fmt.Errorf("supervisor: drain timeout after %s", timeout)
	}
}
