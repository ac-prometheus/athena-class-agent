package session

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// staleCheckpointAge is how long an active checkpoint can be untouched before
// CheckpointScan considers the session interrupted.
const staleCheckpointAge = 10 * time.Minute

// Session manages a single agent session from wake to end-of-session compression.
// Implements pkg.SessionLifecycle.
type Session struct {
	pkg.Session

	// Runtime state — not persisted directly.
	TokensUsed int
	turnCount  int

	// db is used for per-turn checkpoint writes. Nil when DB is unavailable.
	db platform.DB
}

// Ensure Session implements pkg.SessionLifecycle at compile time.
var _ pkg.SessionLifecycle = (*Session)(nil)

// NewSession initialises a new session with a generated ID.
// Pass a non-nil db to enable checkpoint persistence.
func NewSession(agentName, wakeReason string, db platform.DB) *Session {
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
		db: db,
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

// WriteCheckpoint upserts one row in session_checkpoints with the current turn state.
// Called after each turn so a crash leaves an auditable record.
// Silently skips if no DB is available.
func (s *Session) WriteCheckpoint() error {
	if s.db == nil {
		return nil
	}

	const q = `
INSERT INTO session_checkpoints (id, session_id, turn_number, t2_high_water, token_usage, state, created_at)
VALUES ($1, $2, $3, $4, $5, 'active', NOW())
ON CONFLICT (id) DO UPDATE
  SET turn_number   = EXCLUDED.turn_number,
      t2_high_water = EXCLUDED.t2_high_water,
      token_usage   = EXCLUDED.token_usage,
      state         = 'active',
      created_at    = NOW()`

	_, err := s.db.ExecContext(context.Background(), q,
		s.ID, s.ID, s.turnCount, "", s.TokensUsed,
	)
	if err != nil {
		return fmt.Errorf("session: write checkpoint: %w", err)
	}
	slog.Debug("session: checkpoint written", "session_id", s.ID, "turn", s.turnCount)
	return nil
}

// GetID returns the session identifier.
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

// InterruptedSession carries minimal information about a session that was
// interrupted mid-run, surfaced by CheckpointScan.
type InterruptedSession struct {
	SessionID  string
	TurnNumber int
	Date       time.Time
}

// InterruptNote returns the in-band orientation note for this interrupted session.
// Surfaced in Phase 5 (Incoming) of the next assembly.
func (is *InterruptedSession) InterruptNote() string {
	return fmt.Sprintf(
		"[session interrupted] A session on %s was interrupted after %d turn(s); "+
			"the record up to that point is intact.",
		is.Date.Format("2006-01-02"),
		is.TurnNumber,
	)
}

// CheckpointScan queries session_checkpoints for active rows older than staleCheckpointAge,
// marks them interrupted, and returns the interrupted sessions so the next orientation can
// surface an in-band note. Returns nil slice if none found.
func CheckpointScan(ctx context.Context, db platform.DB) ([]*InterruptedSession, error) {
	if db == nil {
		return nil, nil
	}

	cutoff := time.Now().Add(-staleCheckpointAge)

	rows, err := db.QueryContext(ctx, `
		SELECT session_id, turn_number, created_at
		FROM session_checkpoints
		WHERE state = 'active' AND created_at < $1`,
		cutoff,
	)
	if err != nil {
		return nil, fmt.Errorf("session: checkpoint scan query: %w", err)
	}
	defer rows.Close()

	var interrupted []*InterruptedSession
	for rows.Next() {
		var sessionID string
		var turnNumber int
		var createdAt time.Time
		if err := rows.Scan(&sessionID, &turnNumber, &createdAt); err != nil {
			return nil, fmt.Errorf("session: checkpoint scan scan: %w", err)
		}
		interrupted = append(interrupted, &InterruptedSession{
			SessionID:  sessionID,
			TurnNumber: turnNumber,
			Date:       createdAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("session: checkpoint scan rows: %w", err)
	}

	if len(interrupted) == 0 {
		return nil, nil
	}

	// Mark stale active checkpoints as interrupted.
	_, err = db.ExecContext(ctx, `
		UPDATE session_checkpoints
		SET state = 'interrupted'
		WHERE state = 'active' AND created_at < $1`,
		cutoff,
	)
	if err != nil {
		// Log but don't fail — the session notes were already collected.
		slog.Warn("session: could not mark checkpoints interrupted", "err", err)
	}

	for _, is := range interrupted {
		slog.Warn("session: interrupted session found",
			"session_id", is.SessionID,
			"turn", is.TurnNumber,
			"date", is.Date.Format(time.RFC3339),
		)
	}

	return interrupted, nil
}

// errNoRows is the sentinel for sql.ErrNoRows, used for driver-independent no-result checks.
var errNoRows = sql.ErrNoRows
