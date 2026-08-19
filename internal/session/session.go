package session

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// Session manages a single agent session from wake to end-of-session compression.
// Implements pkg.SessionLifecycle.
type Session struct {
	pkg.Session

	// Runtime state — not persisted directly.
	TokensUsed int
	turnCount  int
}

// Ensure Session implements pkg.SessionLifecycle at compile time.
var _ pkg.SessionLifecycle = (*Session)(nil)

// NewSession initialises a new session with a generated ID.
// Checkpoint persistence is handled by LifecycleStore (see internal/platform/storage).
func NewSession(agentName, wakeReason string) *Session {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic("session: crypto/rand unavailable: " + err.Error())
	}
	id := agentName + "-" + hex.EncodeToString(buf[:])
	slog.Info("session: new session", "session_id", id, "agent", agentName, "wake_reason", wakeReason)

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

// Start initialises session state. For backward compatibility with sessions
// created via NewSession, this is a no-op if the session is already active.
func (s *Session) Start(agentName, wakeReason string) error {
	if s.State == pkg.SessionStateActive {
		return nil
	}
	s.AgentName = agentName
	s.WakeReason = wakeReason
	s.StartedAt = time.Now()
	s.State = pkg.SessionStateActive
	return nil
}

// End marks the session completed and logs final metrics.
// Phase 1 stub: no T2→T3 compression, no salience scoring, no dream cycle.
func (s *Session) End() error {
	now := time.Now()
	s.EndedAt = &now
	s.State = pkg.SessionStateCompleted

	duration := now.Sub(s.StartedAt)
	slog.Info("session: session ended",
		"session_id", s.ID,
		"turns", s.turnCount,
		"tokens_used", s.TokensUsed,
		"duration_s", duration.Seconds(),
	)
	return nil
}

// RecordTurn updates session counters after a completed turn.
func (s *Session) RecordTurn(promptToks, completionToks int) {
	s.turnCount++
	s.TokensUsed += promptToks + completionToks
}

// GetID returns the session identifier.
// Callers that need checkpoint persistence should use pkg.LifecycleStore.WriteCheckpoint.
func (s *Session) GetID() string {
	return s.ID
}

// GetState returns the current session state.
func (s *Session) GetState() pkg.SessionState {
	return s.State
}

// TurnCount returns the number of turns recorded so far.
func (s *Session) TurnCount() int {
	return s.turnCount
}

