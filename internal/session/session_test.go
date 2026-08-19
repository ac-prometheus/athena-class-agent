package session

import (
	"testing"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// ---------------------------------------------------------------------------
// Session tests — Sprint 3B
// ---------------------------------------------------------------------------

func TestNewSession_CreatesValidSession(t *testing.T) {
	s := NewSession("aurora", "manual")

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
	s := NewSession("ersa", "scheduled")

	if len(s.ID) < 5 {
		t.Fatalf("ID too short: %q", s.ID)
	}
	// ID format is agentName-hexbytes
	if s.ID[:5] != "ersa-" {
		t.Errorf("ID should start with agent name, got %q", s.ID)
	}
}

func TestNewSession_UniqueIDs(t *testing.T) {
	s1 := NewSession("test", "manual")
	s2 := NewSession("test", "manual")

	if s1.ID == s2.ID {
		t.Error("two sessions should have unique IDs")
	}
}

func TestSession_Start_AlreadyActive(t *testing.T) {
	s := NewSession("test", "manual")

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
	s := NewSession("test", "manual")

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
	s := NewSession("test", "manual")

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
	s := NewSession("test", "manual")
	if s.TurnCount() != 0 {
		t.Errorf("initial TurnCount = %d, want 0", s.TurnCount())
	}
}

func TestSession_GetID(t *testing.T) {
	s := NewSession("test", "manual")
	if s.GetID() != s.ID {
		t.Errorf("GetID() = %q, want %q", s.GetID(), s.ID)
	}
}

func TestSession_GetState(t *testing.T) {
	s := NewSession("test", "manual")

	if s.GetState() != pkg.SessionStateActive {
		t.Errorf("GetState() = %q, want %q", s.GetState(), pkg.SessionStateActive)
	}

	s.End()
	if s.GetState() != pkg.SessionStateCompleted {
		t.Errorf("GetState() after End = %q, want %q", s.GetState(), pkg.SessionStateCompleted)
	}
}

