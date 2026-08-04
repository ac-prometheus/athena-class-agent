package daemon

import (
	"context"
	"log/slog"

	"github.com/ac-prometheus/athena-class-agent/internal/channels"
	"github.com/ac-prometheus/athena-class-agent/internal/assembly"
	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// Daemon runs the persistent process. In heartbeat mode, it polls wake
// conditions and starts sessions. In external mode, it waits for triggers.
type Daemon struct {
	cfg      Config
	assembly sessionRunner
	registry *channels.ChannelRegistry
	gateway  pkg.ContentGateway
	waker    *WakeScheduler
	db       platform.DB // optional; enables checkpoint scan on startup
}

// sessionRunner is the minimal interface the daemon needs from assembly.
// Keeps daemon from importing assembly directly in Phase 1.
type sessionRunner interface {
	RunSession(wakeReason string, inbandNotes []string) error
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
	return &Daemon{cfg: cfg, assembly: runner}
}

// WithDB attaches a database handle so the daemon can run CheckpointScan at startup.
func (d *Daemon) WithDB(db platform.DB) {
	d.db = db
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
	// Scan for sessions that were interrupted by a crash or OOM before the first wake.
	startupNotes := d.scanInterruptedSessions(ctx)

	if d.registry == nil || len(d.registry.List()) == 0 {
		slog.Info("daemon: no channels configured — running one-shot session")
		return d.assembly.RunSession("daemon-startup", startupNotes)
	}

	events, err := d.registry.StartAll(ctx)
	if err != nil {
		slog.Warn("daemon: channel startup error, falling back to one-shot", "err", err)
		return d.assembly.RunSession("daemon-startup", startupNotes)
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
	interrupted, err := assembly.CheckpointScan(ctx, d.db)
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

	if err := d.assembly.RunSession(wakeReason, inbandNotes); err != nil {
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
