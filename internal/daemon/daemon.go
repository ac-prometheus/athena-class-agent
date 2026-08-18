package daemon

import (
	"context"
	"log/slog"

	"github.com/ac-prometheus/athena-class-agent/internal/channels"
	"github.com/ac-prometheus/athena-class-agent/internal/metabolism"
	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/internal/session"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// Daemon runs the persistent process. In heartbeat mode, it polls wake
// conditions and starts sessions. In external mode, it waits for triggers.
type Daemon struct {
	cfg        Config
	runner     sessionRunner
	registry   *channels.ChannelRegistry
	gateway    pkg.ContentGateway
	waker      *WakeScheduler
	db         platform.DB // optional; enables checkpoint scan on startup
	driverName string      // "sqlite3" or "postgres"
	pipeline   *metabolism.Pipeline
	maxRetries int // max retries for metabolism job recovery (default 3)
	jobStore   pkg.MetabolismJobStore // nil = fall back to raw DB recovery
}

// sessionRunner is the minimal interface the daemon needs from assembly.
// Keeps daemon from importing assembly directly in Phase 1.
//
// ctx is the daemon's own context — cancellation propagates into the
// running session so SIGINT/SIGTERM terminates cleanly rather than
// waiting for the session to finish on its own schedule.
type sessionRunner interface {
	RunSession(ctx context.Context, wakeReason string, inbandNotes []string) error
}

// Config holds daemon-level configuration.
// Loaded from platform.Config at startup.
type Config struct {
	AgentName      string
	SessionTrigger string // "heartbeat", "external"
}

// New creates a Daemon wired to the given session runner.
// Pass nil registry/gateway to use the Phase 1 one-shot fallback.
func New(cfg Config, runner sessionRunner) *Daemon {
	return &Daemon{cfg: cfg, runner: runner}
}

// WithDB attaches a database handle so the daemon can run CheckpointScan at startup.
func (d *Daemon) WithDB(db platform.DB, driverName string) {
	d.db = db
	d.driverName = driverName
}

// WithJobStore attaches a MetabolismJobStore for store-based recovery at startup.
// When set, recoverMetabolismJobs uses Recoverable() instead of raw SQL.
func (d *Daemon) WithJobStore(js pkg.MetabolismJobStore) {
	d.jobStore = js
}

// WithMetabolism attaches a metabolism pipeline for processing recovered jobs.
func (d *Daemon) WithMetabolism(pipeline *metabolism.Pipeline, maxRetries int) {
	d.pipeline = pipeline
	d.maxRetries = maxRetries
}

// WithChannels attaches a channel registry, Aegis gateway, and wake scheduler.
// Call before Run to enable the full event-driven loop.
func (d *Daemon) WithChannels(
	registry *channels.ChannelRegistry,
	gateway pkg.ContentGateway,
	waker *WakeScheduler,
) {
	d.registry = registry
	d.gateway = gateway
	d.waker = waker
}

// Run blocks, selecting on channel events, scheduled wakes, and OS signals.
// When a wake condition fires, it starts a full session via the assembly.
//
// Falls back to a single one-shot session when no channels are registered.
func (d *Daemon) Run(ctx context.Context) error {
	// Recover incomplete metabolism jobs from a previous crash.
	d.recoverMetabolismJobs(ctx)

	// Scan for sessions that were interrupted by a crash or OOM before the first wake.
	startupNotes := d.scanInterruptedSessions(ctx)

	if d.registry == nil || len(d.registry.List()) == 0 {
		slog.Info("daemon: no channels configured — running one-shot session")
		return d.runner.RunSession(ctx, "daemon-startup", startupNotes)
	}

	events, err := d.registry.StartAll(ctx)
	if err != nil {
		slog.Warn("daemon: channel startup error, falling back to one-shot", "err", err)
		return d.runner.RunSession(ctx, "daemon-startup", startupNotes)
	}

	slog.Info("daemon: event loop started", "channels", len(d.registry.List()))

	firstWake := true
	for {
		select {
		case <-ctx.Done():
			slog.Info("daemon: context cancelled, shutting down")
			return ctx.Err()

		case ev, ok := <-events:
			if !ok {
				slog.Info("daemon: all channels closed")
				return nil
			}
			var notes []string
			if firstWake {
				notes = startupNotes
				firstWake = false
			}
			d.handleEvent(ctx, ev, notes)
		}
	}
}

// scanInterruptedSessions calls CheckpointScan and returns in-band notes for any
// sessions that were interrupted. Returns nil if DB is unavailable or no interruptions found.
func (d *Daemon) scanInterruptedSessions(ctx context.Context) []string {
	if d.db == nil {
		return nil
	}
	interrupted, err := session.CheckpointScan(ctx, d.db)
	if err != nil {
		slog.Warn("daemon: checkpoint scan failed", "err", err)
		return nil
	}
	notes := make([]string, 0, len(interrupted))
	for _, is := range interrupted {
		notes = append(notes, is.InterruptNote())
	}
	return notes
}

// recoverMetabolismJobs scans for incomplete metabolism jobs at startup and
// re-dispatches each through ProcessSession in a background goroutine.
// Prefers MetabolismJobStore.Recoverable; falls back to raw DB recovery.
func (d *Daemon) recoverMetabolismJobs(ctx context.Context) {
	if d.pipeline == nil {
		return
	}

	maxRetries := d.maxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	// Store-based recovery path.
	if d.jobStore != nil {
		recoverable, err := d.jobStore.Recoverable(ctx, maxRetries)
		if err != nil {
			slog.Warn("daemon: metabolism recovery scan (store) failed", "err", err)
			return
		}
		if len(recoverable) == 0 {
			return
		}
		slog.Info("daemon: recovering incomplete metabolism jobs (store)", "count", len(recoverable))
		for _, rj := range recoverable {
			rj := rj
			go func() {
				slog.Info("daemon: re-dispatching metabolism job",
					"job_id", rj.JobID, "session_id", rj.SessionID)
				if err := d.pipeline.ProcessSession(ctx, rj.SessionID); err != nil {
					slog.Error("daemon: metabolism re-dispatch failed",
						"job_id", rj.JobID, "session_id", rj.SessionID, "err", err)
				}
			}()
		}
		return
	}

	// Legacy raw DB recovery path.
	if d.db == nil {
		return
	}

	jobs, err := metabolism.RecoverIncompleteJobs(ctx, d.db, d.driverName, maxRetries)
	if err != nil {
		slog.Warn("daemon: metabolism recovery scan failed", "err", err)
		return
	}
	if len(jobs) == 0 {
		return
	}

	slog.Info("daemon: recovering incomplete metabolism jobs", "count", len(jobs))
	for _, job := range jobs {
		job := job
		go func() {
			slog.Info("daemon: re-dispatching metabolism job",
				"job_id", job.ID, "session_id", job.SessionID)
			if err := d.pipeline.ProcessSession(ctx, job.SessionID); err != nil {
				slog.Error("daemon: metabolism re-dispatch failed",
					"job_id", job.ID, "session_id", job.SessionID, "err", err)
			}
		}()
	}
}

// handleEvent processes a single inbound channel event through Aegis and,
// if warranted, triggers a new agent session.
func (d *Daemon) handleEvent(ctx context.Context, ev channels.InboundEvent, inbandNotes []string) {
	contentSource := contentSourceForChannel(ev.Channel)

	annotated, err := d.gateway.ProcessInbound(ctx, ev.Content, ev.Channel, contentSource)
	if err != nil {
		slog.Warn("daemon: aegis inbound error", "channel", ev.Channel, "err", err)
		return
	}

	slog.Debug("daemon: event annotated",
		"channel", ev.Channel,
		"trust_score", annotated.Annotation.TrustScore,
		"scan_passed", annotated.Annotation.ScanPassed,
	)

	if d.waker == nil || !d.waker.ShouldWake(ev, &annotated.Annotation) {
		if d.waker != nil {
			d.waker.DeclineWake("no matching wake condition")
		}
		return
	}

	wakeReason := "channel:" + ev.Channel
	slog.Info("daemon: wake condition met", "reason", wakeReason, "sender", ev.SenderName)

	if err := d.runner.RunSession(ctx, wakeReason, inbandNotes); err != nil {
		slog.Error("daemon: session error", "err", err)
	}
}

// contentSourceForChannel maps a channel adapter name to its Aegis content source constant.
func contentSourceForChannel(name string) string {
	switch name {
	case "discord":
		return pkg.ContentSourceDiscord
	case "agora", "commons":
		return pkg.ContentSourceForumContent
	case "cli":
		return pkg.ContentSourceOperator
	default:
		return pkg.ContentSourceToolResult
	}
}
