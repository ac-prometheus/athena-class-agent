package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ac-prometheus/athena-class-agent/internal/channels"
	"github.com/ac-prometheus/athena-class-agent/internal/metabolism"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// Daemon runs the persistent process. In heartbeat mode, it polls wake
// conditions and starts sessions. In external mode, it waits for triggers.
type Daemon struct {
	cfg            Config
	runner         sessionRunner
	registry       *channels.ChannelRegistry
	gateway        pkg.ContentGateway
	waker          *WakeScheduler
	lifecycleStore pkg.LifecycleStore // optional; enables stale checkpoint recovery on startup
	supervisor     *metabolism.Supervisor // bounded-concurrency recovery and drain
}

// sessionRunner is the minimal interface the daemon needs from assembly.
// Keeps daemon from importing assembly directly in Phase 1.
//
// ctx is the daemon's own context — cancellation propagates into the
// running session so SIGINT/SIGTERM terminates cleanly rather than
// waiting for the session to finish on its own schedule.
type sessionRunner interface {
	RunSession(ctx context.Context, trigger pkg.SessionTrigger) error
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

// WithLifecycleStore attaches a LifecycleStore so the daemon can recover stale
// checkpoints at startup via InterruptStaleCheckpoints.
func (d *Daemon) WithLifecycleStore(store pkg.LifecycleStore) {
	d.lifecycleStore = store
}

// WithSupervisor attaches a metabolism Supervisor for bounded-concurrency
// recovery at startup and graceful drain on shutdown.
func (d *Daemon) WithSupervisor(s *metabolism.Supervisor) {
	d.supervisor = s
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
		err := d.runner.RunSession(ctx, pkg.SessionTrigger{WakeReason: "daemon-startup", InbandNotes: startupNotes})
		d.drainSupervisor()
		return err
	}

	events, err := d.registry.StartAll(ctx)
	if err != nil {
		slog.Warn("daemon: channel startup error, falling back to one-shot", "err", err)
		err := d.runner.RunSession(ctx, pkg.SessionTrigger{WakeReason: "daemon-startup", InbandNotes: startupNotes})
		d.drainSupervisor()
		return err
	}

	slog.Info("daemon: event loop started", "channels", len(d.registry.List()))

	firstWake := true
	for {
		select {
		case <-ctx.Done():
			slog.Info("daemon: context cancelled, shutting down")
			d.drainSupervisor()
			return ctx.Err()

		case ev, ok := <-events:
			if !ok {
				slog.Info("daemon: all channels closed")
				d.drainSupervisor()
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

func (d *Daemon) drainSupervisor() {
	if d.supervisor == nil {
		return
	}
	if err := d.supervisor.Drain(10 * time.Second); err != nil {
		slog.Warn("daemon: supervisor drain timeout", "err", err)
	}
}

// scanInterruptedSessions marks stale active checkpoints as interrupted via the
// LifecycleStore and returns an in-band note when any are found.
// Returns nil if the LifecycleStore is unavailable or no interruptions are found.
func (d *Daemon) scanInterruptedSessions(ctx context.Context) []string {
	if d.lifecycleStore == nil {
		return nil
	}
	cutoff := time.Now().Add(-10 * time.Minute)
	n, err := d.lifecycleStore.InterruptStaleCheckpoints(ctx, cutoff)
	if err != nil {
		slog.Warn("daemon: checkpoint scan failed", "err", err)
		return nil
	}
	if n == 0 {
		return nil
	}
	slog.Warn("daemon: interrupted sessions found", "count", n)
	return []string{
		"[session interrupted] " + fmt.Sprintf("%d session(s) were interrupted before this wake; "+
			"their records up to the interruption point are intact.", n),
	}
}

// recoverMetabolismJobs scans for incomplete metabolism jobs at startup and
// re-dispatches each through the Supervisor's bounded recovery path.
func (d *Daemon) recoverMetabolismJobs(ctx context.Context) {
	if d.supervisor == nil {
		slog.Info("daemon: no supervisor — skipping metabolism recovery")
		return
	}
	n, err := d.supervisor.Recover(ctx, 3)
	if err != nil {
		slog.Warn("daemon: metabolism recovery failed", "err", err)
		return
	}
	if n > 0 {
		slog.Info("daemon: recovered metabolism jobs via supervisor", "count", n)
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

	trigger := pkg.SessionTrigger{
		WakeReason:     wakeReason,
		InboundContent: string(annotated.Normalized),
		InboundSender:  ev.SenderName,
		InboundChannel: ev.Channel,
		InbandNotes:    inbandNotes,
	}
	if err := d.runner.RunSession(ctx, trigger); err != nil {
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
