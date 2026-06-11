package daemon

import "context"

// Daemon runs the persistent process. In heartbeat mode, it polls wake
// conditions and starts sessions. In external mode, it waits for triggers.
type Daemon struct {
	cfg     Config
	harness sessionRunner
}

// sessionRunner is the minimal interface the daemon needs from harness.
// Keeps daemon from importing harness directly in Phase 1.
type sessionRunner interface {
	RunSession(wakeReason string) error
}

// Config holds daemon-level configuration.
// Loaded from platform.Config at startup.
type Config struct {
	AgentName      string
	SessionTrigger string // "heartbeat", "external"
}

// New creates a Daemon wired to the given session runner.
func New(cfg Config, runner sessionRunner) *Daemon {
	return &Daemon{cfg: cfg, harness: runner}
}

// Run blocks, selecting on channel events, scheduled wakes, and OS signals.
// When a wake condition fires, it starts a full session via the harness.
//
// Phase 1 behaviour: runs exactly one session and exits.
// Phase 2+ adds the select loop with channel adapters, scheduled-wake ticker,
// and OS signal handling.
func (d *Daemon) Run(ctx context.Context) error {
	return d.harness.RunSession("daemon-startup")
}
