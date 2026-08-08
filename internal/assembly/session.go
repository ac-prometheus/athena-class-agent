package assembly

import (
	"context"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/internal/session"
)

// Session is a type alias for backward compatibility.
// New code should use internal/session.Session and pkg.SessionLifecycle directly.
type Session = session.Session

// InterruptedSession is a type alias for backward compatibility.
type InterruptedSession = session.InterruptedSession

// NewSession delegates to session.NewSession.
// Deprecated: prefer session.NewSession or accepting a pkg.SessionLifecycle.
func NewSession(agentName, wakeReason string, db platform.DB) *Session {
	return session.NewSession(agentName, wakeReason, db)
}

// CheckpointScan delegates to session.CheckpointScan.
// Deprecated: prefer session.CheckpointScan directly.
func CheckpointScan(ctx context.Context, db platform.DB) ([]*InterruptedSession, error) {
	return session.CheckpointScan(ctx, db)
}
