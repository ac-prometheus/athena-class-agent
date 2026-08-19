package assembly

import (
	"github.com/ac-prometheus/athena-class-agent/internal/session"
)

// Session is a type alias for backward compatibility.
// New code should use internal/session.Session and pkg.SessionLifecycle directly.
type Session = session.Session
