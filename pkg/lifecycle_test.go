package pkg

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Ontology enum consistency tests — Sprint 3B
//
// Every constant must be present in its validation map.
// Invalid values must be rejected.
// These tests cross-check Go constants against the CHECK constraints in
// schema/009_lifecycle.sql.
// ---------------------------------------------------------------------------

func TestValidWakeCauses_AllConstants(t *testing.T) {
	allCauses := []WakeCause{
		WakeCauseInitial, WakeCauseExternal, WakeCauseScheduled,
		WakeCauseHeartbeat, WakeCauseAgentRequested, WakeCauseContextPressure,
		WakeCauseManual, WakeCauseRecovery,
	}
	for _, c := range allCauses {
		if !ValidWakeCauses[c] {
			t.Errorf("WakeCause %q missing from ValidWakeCauses map", c)
		}
	}
}

func TestValidWakeCauses_RejectsInvalid(t *testing.T) {
	invalid := []WakeCause{"", "unknown", "cron", "INITIAL"}
	for _, c := range invalid {
		if ValidWakeCauses[c] {
			t.Errorf("ValidWakeCauses should reject %q", c)
		}
	}
}

func TestValidWakeCauses_MatchesSQL(t *testing.T) {
	// CHECK constraint values from schema/009_lifecycle.sql
	sqlValues := []string{
		"initial", "external", "scheduled", "heartbeat",
		"agent_requested", "context_pressure", "manual", "recovery",
	}
	for _, v := range sqlValues {
		if !ValidWakeCauses[WakeCause(v)] {
			t.Errorf("SQL CHECK value %q not in ValidWakeCauses", v)
		}
	}
	if len(ValidWakeCauses) != len(sqlValues) {
		t.Errorf("ValidWakeCauses has %d entries, SQL CHECK has %d", len(ValidWakeCauses), len(sqlValues))
	}
}

func TestValidTransitionContexts_AllConstants(t *testing.T) {
	all := []TransitionContext{
		TransitionNone, TransitionModeChange, TransitionSubstrateChange,
		TransitionConfigurationChange, TransitionRecoveryAfterFailure,
		TransitionIdentityDocumentChange,
	}
	for _, c := range all {
		if !ValidTransitionContexts[c] {
			t.Errorf("TransitionContext %q missing from ValidTransitionContexts map", c)
		}
	}
}

func TestValidTransitionContexts_RejectsInvalid(t *testing.T) {
	invalid := []TransitionContext{"", "reboot", "NONE"}
	for _, c := range invalid {
		if ValidTransitionContexts[c] {
			t.Errorf("ValidTransitionContexts should reject %q", c)
		}
	}
}

func TestValidTransitionContexts_MatchesSQL(t *testing.T) {
	sqlValues := []string{
		"none", "mode_change", "substrate_change",
		"configuration_change", "recovery_after_failure", "identity_document_change",
	}
	for _, v := range sqlValues {
		if !ValidTransitionContexts[TransitionContext(v)] {
			t.Errorf("SQL CHECK value %q not in ValidTransitionContexts", v)
		}
	}
	if len(ValidTransitionContexts) != len(sqlValues) {
		t.Errorf("ValidTransitionContexts has %d entries, SQL CHECK has %d", len(ValidTransitionContexts), len(sqlValues))
	}
}

func TestValidSeamKinds_AllConstants(t *testing.T) {
	all := []SeamKind{
		SeamColdWake, SeamWarmReturn, SeamContextCompaction,
		SeamRestReturn, SeamNone,
	}
	for _, c := range all {
		if !ValidSeamKinds[c] {
			t.Errorf("SeamKind %q missing from ValidSeamKinds map", c)
		}
	}
}

func TestValidSeamKinds_RejectsInvalid(t *testing.T) {
	invalid := []SeamKind{"", "hot_swap", "COLD_WAKE"}
	for _, c := range invalid {
		if ValidSeamKinds[c] {
			t.Errorf("ValidSeamKinds should reject %q", c)
		}
	}
}

func TestValidSeamKinds_MatchesSQL(t *testing.T) {
	sqlValues := []string{
		"cold_wake", "warm_return", "context_compaction", "rest_return", "none",
	}
	for _, v := range sqlValues {
		if !ValidSeamKinds[SeamKind(v)] {
			t.Errorf("SQL CHECK value %q not in ValidSeamKinds", v)
		}
	}
	if len(ValidSeamKinds) != len(sqlValues) {
		t.Errorf("ValidSeamKinds has %d entries, SQL CHECK has %d", len(ValidSeamKinds), len(sqlValues))
	}
}

func TestValidActivityProfiles_AllConstants(t *testing.T) {
	all := []ActivityProfile{ActivityNormal, ActivityFree, ActivitySentinel}
	for _, c := range all {
		if !ValidActivityProfiles[c] {
			t.Errorf("ActivityProfile %q missing from ValidActivityProfiles map", c)
		}
	}
}

func TestValidActivityProfiles_RejectsInvalid(t *testing.T) {
	invalid := []ActivityProfile{"", "idle", "NORMAL"}
	for _, c := range invalid {
		if ValidActivityProfiles[c] {
			t.Errorf("ValidActivityProfiles should reject %q", c)
		}
	}
}

func TestValidActivityProfiles_MatchesSQL(t *testing.T) {
	sqlValues := []string{"normal", "free", "sentinel"}
	for _, v := range sqlValues {
		if !ValidActivityProfiles[ActivityProfile(v)] {
			t.Errorf("SQL CHECK value %q not in ValidActivityProfiles", v)
		}
	}
	if len(ValidActivityProfiles) != len(sqlValues) {
		t.Errorf("ValidActivityProfiles has %d entries, SQL CHECK has %d", len(ValidActivityProfiles), len(sqlValues))
	}
}

func TestValidAssemblyProfiles_AllConstants(t *testing.T) {
	all := []AssemblyProfile{AssemblyFull, AssemblyLight, AssemblyMinimal, AssemblySeam}
	for _, c := range all {
		if !ValidAssemblyProfiles[c] {
			t.Errorf("AssemblyProfile %q missing from ValidAssemblyProfiles map", c)
		}
	}
}

func TestValidAssemblyProfiles_RejectsInvalid(t *testing.T) {
	invalid := []AssemblyProfile{"", "heavy", "FULL"}
	for _, c := range invalid {
		if ValidAssemblyProfiles[c] {
			t.Errorf("ValidAssemblyProfiles should reject %q", c)
		}
	}
}

func TestValidAssemblyProfiles_MatchesSQL(t *testing.T) {
	sqlValues := []string{"full", "light", "minimal", "seam"}
	for _, v := range sqlValues {
		if !ValidAssemblyProfiles[AssemblyProfile(v)] {
			t.Errorf("SQL CHECK value %q not in ValidAssemblyProfiles", v)
		}
	}
	if len(ValidAssemblyProfiles) != len(sqlValues) {
		t.Errorf("ValidAssemblyProfiles has %d entries, SQL CHECK has %d", len(ValidAssemblyProfiles), len(sqlValues))
	}
}

func TestLifecyclePlan_Construction(t *testing.T) {
	now := time.Now().UTC()
	secondaryCause := WakeCauseScheduled

	plan := LifecyclePlan{
		ID:                 "plan-001",
		SessionID:          "session-001",
		TemporalMode:       TemporalEpisodic,
		WakeCause:          WakeCauseInitial,
		WakeCauseSecondary: &secondaryCause,
		ActivityProfile:    ActivityNormal,
		AssemblyProfile:    AssemblyFull,
		BridgePolicy:       BridgeAutoWithAbstention,
		MetabolismPolicy:   "standard",
		SeamKind:           SeamNone,
		ResolverVersion:    "v1",
		PolicyHash:         "abc123",
		GapFacts: &GapFacts{
			WakeAt:          now,
			ElapsedDuration: 5 * time.Minute,
			ClockBasis:      "wall",
			GapClass:        GapShort,
		},
		TransitionContexts: []TransitionContext{TransitionNone},
		Disclosures:        []string{"test disclosure"},
		Reasons:            map[string]string{"key": "value"},
		CreatedAt:          now,
	}

	if plan.ID != "plan-001" {
		t.Errorf("ID = %q, want %q", plan.ID, "plan-001")
	}
	if plan.WakeCauseSecondary == nil || *plan.WakeCauseSecondary != WakeCauseScheduled {
		t.Error("WakeCauseSecondary not set correctly")
	}
	if plan.GapFacts == nil {
		t.Fatal("GapFacts is nil")
	}
	if plan.GapFacts.GapClass != GapShort {
		t.Errorf("GapClass = %q, want %q", plan.GapFacts.GapClass, GapShort)
	}
}

func TestGapFacts_FieldTypes(t *testing.T) {
	now := time.Now()
	prev := now.Add(-1 * time.Hour)
	gf := GapFacts{
		PreviousActivityAt: &prev,
		WakeAt:             now,
		ElapsedDuration:    time.Hour,
		ClockBasis:         "wall",
		GapClass:           GapOvernight,
		MessagesPending:    5,
		VaultChanges:       3,
	}

	if gf.PreviousActivityAt == nil {
		t.Fatal("PreviousActivityAt should be non-nil")
	}
	if gf.ElapsedDuration != time.Hour {
		t.Errorf("ElapsedDuration = %v, want %v", gf.ElapsedDuration, time.Hour)
	}
	if gf.MessagesPending != 5 {
		t.Errorf("MessagesPending = %d, want 5", gf.MessagesPending)
	}
}

func TestGapFacts_NilPreviousActivity(t *testing.T) {
	gf := GapFacts{
		WakeAt:     time.Now(),
		ClockBasis: "wall",
		GapClass:   GapNone,
	}
	if gf.PreviousActivityAt != nil {
		t.Error("PreviousActivityAt should be nil for first session")
	}
}

func TestTemporalMode_Values(t *testing.T) {
	// Cross-check SQL CHECK constraint
	sqlValues := map[string]TemporalMode{
		"episodic":   TemporalEpisodic,
		"diurnal":    TemporalDiurnal,
		"continuous": TemporalContinuous,
	}
	for sqlVal, goConst := range sqlValues {
		if string(goConst) != sqlVal {
			t.Errorf("TemporalMode constant %q does not match SQL value %q", goConst, sqlVal)
		}
	}
}

func TestBridgePolicy_Values(t *testing.T) {
	// Verify BridgePolicy constants exist and are distinct
	policies := []BridgePolicy{
		BridgeAutoWithAbstention, BridgeAgentRequested, BridgeDisabled,
	}
	seen := make(map[BridgePolicy]bool)
	for _, p := range policies {
		if seen[p] {
			t.Errorf("duplicate BridgePolicy value %q", p)
		}
		seen[p] = true
	}
}

func TestRuntimeStatus_Values(t *testing.T) {
	all := []RuntimeStatus{
		RuntimeStarting, RuntimeActive, RuntimeWaiting,
		RuntimeEnding, RuntimeEnded, RuntimeInterrupted, RuntimeFailed,
	}
	seen := make(map[RuntimeStatus]bool)
	for _, s := range all {
		if seen[s] {
			t.Errorf("duplicate RuntimeStatus value %q", s)
		}
		seen[s] = true
		if string(s) == "" {
			t.Error("RuntimeStatus should not be empty string")
		}
	}
}

func TestMetabolismStatus_Values(t *testing.T) {
	all := []MetabolismStatus{
		MetabolismNotRequired, MetabolismQueued, MetabolismRunning,
		MetabolismComplete, MetabolismPartial,
		MetabolismFailedRetry, MetabolismFailedTerminal,
	}
	seen := make(map[MetabolismStatus]bool)
	for _, s := range all {
		if seen[s] {
			t.Errorf("duplicate MetabolismStatus value %q", s)
		}
		seen[s] = true
		if string(s) == "" {
			t.Error("MetabolismStatus should not be empty string")
		}
	}
}
