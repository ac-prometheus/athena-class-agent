package session

import (
	"testing"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// ---------------------------------------------------------------------------
// Session tests — Sprint 3B
// ---------------------------------------------------------------------------

func TestNewSession_CreatesValidSession(t *testing.T) {
	s := NewSession("aurora", "manual", nil)

	if s == nil {
		t.Fatal("NewSession returned nil")
	}
	if s.ID == "" {
		t.Error("session ID must not be empty")
	}
	if s.AgentName != "aurora" {
		t.Errorf("AgentName = %q, want %q", s.AgentName, "aurora")
	}
	if s.WakeReason != "manual" {
		t.Errorf("WakeReason = %q, want %q", s.WakeReason, "manual")
	}
	if s.State != pkg.SessionStateActive {
		t.Errorf("State = %q, want %q", s.State, pkg.SessionStateActive)
	}
	if s.StartedAt.IsZero() {
		t.Error("StartedAt should not be zero")
	}
}

func TestNewSession_IDContainsAgentName(t *testing.T) {
	s := NewSession("ersa", "scheduled", nil)

	if len(s.ID) < 5 {
		t.Fatalf("ID too short: %q", s.ID)
	}
	// ID format is agentName-hexbytes
	if s.ID[:5] != "ersa-" {
		t.Errorf("ID should start with agent name, got %q", s.ID)
	}
}

func TestNewSession_UniqueIDs(t *testing.T) {
	s1 := NewSession("test", "manual", nil)
	s2 := NewSession("test", "manual", nil)

	if s1.ID == s2.ID {
		t.Error("two sessions should have unique IDs")
	}
}

func TestSession_Start_AlreadyActive(t *testing.T) {
	s := NewSession("test", "manual", nil)

	// Already active from NewSession, Start should be no-op
	err := s.Start("other", "scheduled")
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}
	// Should remain unchanged since it was already active
	if s.AgentName != "test" {
		t.Errorf("AgentName should not change when already active, got %q", s.AgentName)
	}
}

func TestSession_End(t *testing.T) {
	s := NewSession("test", "manual", nil)

	err := s.End()
	if err != nil {
		t.Fatalf("End error: %v", err)
	}
	if s.State != pkg.SessionStateCompleted {
		t.Errorf("State = %q, want %q", s.State, pkg.SessionStateCompleted)
	}
	if s.EndedAt == nil {
		t.Error("EndedAt should be set after End()")
	}
	if s.EndedAt.Before(s.StartedAt) {
		t.Error("EndedAt should not be before StartedAt")
	}
}

func TestSession_RecordTurn(t *testing.T) {
	s := NewSession("test", "manual", nil)

	s.RecordTurn(100, 50)
	if s.TurnCount() != 1 {
		t.Errorf("TurnCount = %d, want 1", s.TurnCount())
	}
	if s.TokensUsed != 150 {
		t.Errorf("TokensUsed = %d, want 150", s.TokensUsed)
	}

	s.RecordTurn(200, 100)
	if s.TurnCount() != 2 {
		t.Errorf("TurnCount = %d, want 2", s.TurnCount())
	}
	if s.TokensUsed != 450 {
		t.Errorf("TokensUsed = %d, want 450", s.TokensUsed)
	}
}

func TestSession_TurnCount_Initial(t *testing.T) {
	s := NewSession("test", "manual", nil)
	if s.TurnCount() != 0 {
		t.Errorf("initial TurnCount = %d, want 0", s.TurnCount())
	}
}

func TestSession_GetID(t *testing.T) {
	s := NewSession("test", "manual", nil)
	if s.GetID() != s.ID {
		t.Errorf("GetID() = %q, want %q", s.GetID(), s.ID)
	}
}

func TestSession_GetState(t *testing.T) {
	s := NewSession("test", "manual", nil)

	if s.GetState() != pkg.SessionStateActive {
		t.Errorf("GetState() = %q, want %q", s.GetState(), pkg.SessionStateActive)
	}

	s.End()
	if s.GetState() != pkg.SessionStateCompleted {
		t.Errorf("GetState() after End = %q, want %q", s.GetState(), pkg.SessionStateCompleted)
	}
}

func TestSession_WriteCheckpoint_NilDB(t *testing.T) {
	s := NewSession("test", "manual", nil)

	err := s.WriteCheckpoint(nil)
	if err != nil {
		t.Errorf("WriteCheckpoint with nil db should return nil, got: %v", err)
	}
}

func TestInterruptedSession_InterruptNote(t *testing.T) {
	is := &InterruptedSession{
		SessionID:  "test-123",
		TurnNumber: 5,
	}
	// Date is zero, but method should not panic
	note := is.InterruptNote()
	if note == "" {
		t.Error("InterruptNote should not be empty")
	}
	if !contains(note, "interrupted") {
		t.Errorf("InterruptNote should mention 'interrupted': %q", note)
	}
	if !contains(note, "5 turn(s)") {
		t.Errorf("InterruptNote should mention turn count: %q", note)
	}
}

func TestCheckpointScan_NilDB(t *testing.T) {
	sessions, err := CheckpointScan(nil, nil)
	if err != nil {
		t.Errorf("CheckpointScan with nil db should return nil error, got: %v", err)
	}
	if sessions != nil {
		t.Error("CheckpointScan with nil db should return nil sessions")
	}
}

// contains is a helper for string containment checks.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
