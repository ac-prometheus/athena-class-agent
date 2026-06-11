package harness

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// Session manages a single agent session from wake to end-of-session compression.
type Session struct {
	pkg.Session

	// Runtime state — not persisted directly.
	TokensUsed int
	TurnCount  int
}

// NewSession initialises a new session with a generated ID.
// Phase 1 stub: no database persistence, no checkpoint scan, no identity loading.
// Those land in Phase 2 (memory) and Phase 3 (identity).
func NewSession(agentName, wakeReason string) *Session {
	id := fmt.Sprintf("%s-%d", agentName, time.Now().UnixNano())
	slog.Info("harness: new session", "session_id", id, "agent", agentName, "wake_reason", wakeReason)

	return &Session{
		Session: pkg.Session{
			ID:         id,
			AgentName:  agentName,
			StartedAt:  time.Now(),
			State:      pkg.SessionStateActive,
			WakeReason: wakeReason,
		},
	}
}

// End marks the session completed and logs final metrics.
// Phase 1 stub: no T2→T3 compression, no salience scoring, no dream cycle.
func (s *Session) End() {
	now := time.Now()
	s.EndedAt = &now
	s.State = pkg.SessionStateCompleted

	duration := now.Sub(s.StartedAt)
	slog.Info("harness: session ended",
		"session_id", s.ID,
		"turns", s.TurnCount,
		"tokens_used", s.TokensUsed,
		"duration_s", duration.Seconds(),
	)
}

// RecordTurn updates session counters after a completed turn.
func (s *Session) RecordTurn(promptToks, completionToks int) {
	s.TurnCount++
	s.TokensUsed += promptToks + completionToks
}
