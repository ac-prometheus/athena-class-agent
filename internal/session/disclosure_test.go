package session

import (
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Disclosure tests — Sprint 3B
// ---------------------------------------------------------------------------

func TestGenerateDisclosure_NilOld_ReportsAllFields(t *testing.T) {
	newPolicy := &LifecyclePolicy{
		TemporalMode:     "episodic",
		BridgePolicy:     "opt_in",
		MetabolismPolicy: "standard",
		AssemblyProfile:  "full",
	}

	d := GenerateDisclosure(nil, newPolicy)

	if d == nil {
		t.Fatal("GenerateDisclosure returned nil")
	}
	if len(d.Changes) != 4 {
		t.Fatalf("expected 4 changes for initial policy, got %d: %v", len(d.Changes), d.Changes)
	}
	// Each field should be "set to"
	for _, c := range d.Changes {
		if !strings.Contains(c, "set to") {
			t.Errorf("initial change should contain 'set to', got %q", c)
		}
	}
}

func TestGenerateDisclosure_NilOld_EmptyNew(t *testing.T) {
	d := GenerateDisclosure(nil, &LifecyclePolicy{})

	if d == nil {
		t.Fatal("GenerateDisclosure returned nil")
	}
	if len(d.Changes) != 1 {
		t.Fatalf("expected 1 change for empty initial policy, got %d", len(d.Changes))
	}
	if d.Changes[0] != "initial policy applied (all defaults)" {
		t.Errorf("unexpected change message: %q", d.Changes[0])
	}
}

func TestGenerateDisclosure_NilOld_PartialNew(t *testing.T) {
	d := GenerateDisclosure(nil, &LifecyclePolicy{
		TemporalMode: "diurnal",
	})

	if len(d.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %v", len(d.Changes), d.Changes)
	}
	if !strings.Contains(d.Changes[0], "temporal_mode") {
		t.Errorf("expected temporal_mode in change, got %q", d.Changes[0])
	}
}

func TestGenerateDisclosure_ChangedTemporalMode(t *testing.T) {
	old := &LifecyclePolicy{TemporalMode: "episodic"}
	new := &LifecyclePolicy{TemporalMode: "diurnal"}

	d := GenerateDisclosure(old, new)

	if len(d.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %v", len(d.Changes), d.Changes)
	}
	if !strings.Contains(d.Changes[0], "temporal_mode") {
		t.Errorf("expected temporal_mode change, got %q", d.Changes[0])
	}
	if !strings.Contains(d.Changes[0], "episodic") || !strings.Contains(d.Changes[0], "diurnal") {
		t.Errorf("change should mention both old and new values: %q", d.Changes[0])
	}
}

func TestGenerateDisclosure_ChangedBridgePolicy(t *testing.T) {
	old := &LifecyclePolicy{BridgePolicy: "opt_in"}
	new := &LifecyclePolicy{BridgePolicy: "always"}

	d := GenerateDisclosure(old, new)
	if len(d.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(d.Changes))
	}
	if !strings.Contains(d.Changes[0], "bridge_policy") {
		t.Errorf("expected bridge_policy change, got %q", d.Changes[0])
	}
}

func TestGenerateDisclosure_ChangedMetabolismPolicy(t *testing.T) {
	old := &LifecyclePolicy{MetabolismPolicy: "standard"}
	new := &LifecyclePolicy{MetabolismPolicy: "deferred"}

	d := GenerateDisclosure(old, new)
	if len(d.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(d.Changes))
	}
	if !strings.Contains(d.Changes[0], "metabolism_policy") {
		t.Errorf("expected metabolism_policy change, got %q", d.Changes[0])
	}
}

func TestGenerateDisclosure_ChangedAssemblyProfile(t *testing.T) {
	old := &LifecyclePolicy{AssemblyProfile: "full"}
	new := &LifecyclePolicy{AssemblyProfile: "light"}

	d := GenerateDisclosure(old, new)
	if len(d.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(d.Changes))
	}
	if !strings.Contains(d.Changes[0], "assembly_profile") {
		t.Errorf("expected assembly_profile change, got %q", d.Changes[0])
	}
}

func TestGenerateDisclosure_NoChanges(t *testing.T) {
	policy := &LifecyclePolicy{
		TemporalMode:     "episodic",
		BridgePolicy:     "opt_in",
		MetabolismPolicy: "standard",
		AssemblyProfile:  "full",
	}
	// Same values
	d := GenerateDisclosure(policy, policy)

	if len(d.Changes) != 1 {
		t.Fatalf("expected 1 change for no-change case, got %d", len(d.Changes))
	}
	if !strings.Contains(d.Changes[0], "no field-level changes") {
		t.Errorf("expected 'no field-level changes', got %q", d.Changes[0])
	}
}

func TestGenerateDisclosure_MultipleChanges(t *testing.T) {
	old := &LifecyclePolicy{
		TemporalMode:     "episodic",
		BridgePolicy:     "opt_in",
		MetabolismPolicy: "standard",
		AssemblyProfile:  "full",
	}
	new := &LifecyclePolicy{
		TemporalMode:     "continuous",
		BridgePolicy:     "never",
		MetabolismPolicy: "skip",
		AssemblyProfile:  "minimal",
	}

	d := GenerateDisclosure(old, new)
	if len(d.Changes) != 4 {
		t.Fatalf("expected 4 changes, got %d: %v", len(d.Changes), d.Changes)
	}
}

func TestGenerateDisclosure_AppliedAtIsSet(t *testing.T) {
	before := time.Now().UTC().Add(-time.Second)
	d := GenerateDisclosure(nil, &LifecyclePolicy{TemporalMode: "episodic"})
	after := time.Now().UTC().Add(time.Second)

	if d.AppliedAt.Before(before) || d.AppliedAt.After(after) {
		t.Errorf("AppliedAt %v not in expected range [%v, %v]", d.AppliedAt, before, after)
	}
}

// ---------------------------------------------------------------------------
// ForContext formatting
// ---------------------------------------------------------------------------

func TestForContext_BasicFormat(t *testing.T) {
	d := &ConfigDisclosure{
		PolicyPath: "/workspace/lifecycle.json",
		OldHash:    "abcdef1234567890abcdef1234567890",
		NewHash:    "1234567890abcdef1234567890abcdef",
		Changes:    []string{"temporal_mode changed from \"episodic\" to \"diurnal\""},
		AppliedAt:  time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC),
	}

	output := d.ForContext()

	if !strings.HasPrefix(output, "[configuration change]\n") {
		t.Error("ForContext should start with [configuration change]")
	}
	if !strings.Contains(output, "Policy: /workspace/lifecycle.json") {
		t.Error("ForContext should contain PolicyPath")
	}
	if !strings.Contains(output, "  - temporal_mode") {
		t.Error("ForContext should list changes with '  - ' prefix")
	}
	if !strings.Contains(output, "Previous hash: abcdef123456...") {
		t.Errorf("ForContext should truncate OldHash to 12 chars + '...', got: %s", output)
	}
	if !strings.Contains(output, "Applied at: 2026-08-08T12:00:00Z") {
		t.Error("ForContext should contain AppliedAt in RFC3339")
	}
}

func TestForContext_EmptyOldHash(t *testing.T) {
	d := &ConfigDisclosure{
		Changes:   []string{"temporal_mode set to \"episodic\""},
		AppliedAt: time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC),
	}

	output := d.ForContext()

	if strings.Contains(output, "Previous hash:") {
		t.Error("ForContext should skip Previous hash line when OldHash is empty")
	}
}

func TestForContext_ShortOldHash(t *testing.T) {
	// C5 fix: short OldHash should not panic
	d := &ConfigDisclosure{
		OldHash:   "abc",
		Changes:   []string{"test"},
		AppliedAt: time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC),
	}

	output := d.ForContext()

	// Should not panic, and should display the full short hash without "..."
	if !strings.Contains(output, "Previous hash: abc") {
		t.Errorf("ForContext with short hash should display full hash, got: %s", output)
	}
}

func TestForContext_ExactlyTwelveCharHash(t *testing.T) {
	d := &ConfigDisclosure{
		OldHash:   "abcdef123456",
		Changes:   []string{"test"},
		AppliedAt: time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC),
	}

	output := d.ForContext()

	// Exactly 12 chars — len > 12 is false, so no truncation
	if !strings.Contains(output, "Previous hash: abcdef123456\n") {
		t.Errorf("ForContext with 12-char hash should show full hash, got: %s", output)
	}
}

func TestForContext_EmptyPolicyPath(t *testing.T) {
	d := &ConfigDisclosure{
		Changes:   []string{"test"},
		AppliedAt: time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC),
	}

	output := d.ForContext()

	if strings.Contains(output, "Policy:") {
		t.Error("ForContext should skip Policy line when PolicyPath is empty")
	}
}

func TestForContext_MultipleChanges(t *testing.T) {
	d := &ConfigDisclosure{
		Changes: []string{
			"temporal_mode changed",
			"bridge_policy changed",
		},
		AppliedAt: time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC),
	}

	output := d.ForContext()

	if strings.Count(output, "  - ") != 2 {
		t.Errorf("expected 2 change lines, got output: %s", output)
	}
}

// ---------------------------------------------------------------------------
// nilIfEmpty helper
// ---------------------------------------------------------------------------

func TestNilIfEmpty(t *testing.T) {
	if nilIfEmpty("") != nil {
		t.Error("nilIfEmpty(\"\") should return nil")
	}
	if nilIfEmpty("abc") != "abc" {
		t.Errorf("nilIfEmpty(\"abc\") = %v, want \"abc\"", nilIfEmpty("abc"))
	}
}

// ---------------------------------------------------------------------------
// RecordDisclosure — DB-dependent, skip if no DB
// ---------------------------------------------------------------------------

func TestRecordDisclosure_NilDB(t *testing.T) {
	d := &ConfigDisclosure{
		Changes:   []string{"test"},
		AppliedAt: time.Now(),
	}

	err := RecordDisclosure(nil, nil, "session-1", d)
	if err != nil {
		t.Errorf("RecordDisclosure with nil db should return nil, got: %v", err)
	}
}
