package lifecycle

import (
	"testing"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// ---------------------------------------------------------------------------
// Resolver tests — Sprint 3B
//
// The lifecycle resolver is a pure function: no I/O, no state mutation.
// Given identical inputs, it must produce identical output (modulo plan ID
// and timestamp).
// ---------------------------------------------------------------------------

func defaultPolicy() pkg.LifecyclePolicy {
	return pkg.LifecyclePolicy{
		TemporalMode:    pkg.TemporalEpisodic,
		DefaultAssembly: pkg.AssemblyFull,
		BridgePolicy:    pkg.BridgeAgentRequested,
		ActivityProfile: pkg.ActivityNormal,
	}
}

func minimalFacts() pkg.WakeFacts {
	return pkg.WakeFacts{
		PrimaryCause:    pkg.WakeCauseManual,
		SeamKind:        pkg.SeamNone,
		ElapsedDuration: 2 * time.Hour,
	}
}

func defaultOpState() pkg.OperationalState {
	return pkg.OperationalState{}
}

func TestResolve_DefaultPolicy_MinimalFacts(t *testing.T) {
	plan := Resolve(defaultPolicy(), minimalFacts(), defaultOpState())

	if plan == nil {
		t.Fatal("Resolve returned nil")
	}
	if plan.ID == "" {
		t.Error("plan ID must not be empty (C2 fix)")
	}
	if plan.TemporalMode != pkg.TemporalEpisodic {
		t.Errorf("TemporalMode = %q, want %q", plan.TemporalMode, pkg.TemporalEpisodic)
	}
	if plan.ActivityProfile != pkg.ActivityNormal {
		t.Errorf("ActivityProfile = %q, want %q", plan.ActivityProfile, pkg.ActivityNormal)
	}
	if plan.BridgePolicy != pkg.BridgeAgentRequested {
		t.Errorf("BridgePolicy = %q, want %q", plan.BridgePolicy, pkg.BridgeAgentRequested)
	}
	if plan.CreatedAt.IsZero() {
		t.Error("CreatedAt must not be zero")
	}
}

func TestResolve_PlanIDNotEmpty(t *testing.T) {
	// C2 fix: plan fields must be fully populated
	plan := Resolve(defaultPolicy(), minimalFacts(), defaultOpState())
	if plan.ID == "" {
		t.Error("plan ID is empty — violates C2 (no empty fields)")
	}
	if len(plan.ID) != 32 {
		t.Errorf("plan ID length = %d, want 32 (16 bytes hex-encoded)", len(plan.ID))
	}
}

func TestResolve_PlanIDsAreUnique(t *testing.T) {
	p := defaultPolicy()
	f := minimalFacts()
	o := defaultOpState()

	plan1 := Resolve(p, f, o)
	plan2 := Resolve(p, f, o)

	if plan1.ID == plan2.ID {
		t.Error("two plans from identical inputs must have different IDs")
	}
}

func TestResolve_TemporalModeFromPolicy(t *testing.T) {
	tests := []struct {
		name string
		mode pkg.TemporalMode
	}{
		{"episodic", pkg.TemporalEpisodic},
		{"diurnal", pkg.TemporalDiurnal},
		{"continuous", pkg.TemporalContinuous},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := defaultPolicy()
			p.TemporalMode = tt.mode
			plan := Resolve(p, minimalFacts(), defaultOpState())
			if plan.TemporalMode != tt.mode {
				t.Errorf("TemporalMode = %q, want %q", plan.TemporalMode, tt.mode)
			}
		})
	}
}

func TestResolve_ActivityProfileFromPolicy(t *testing.T) {
	tests := []struct {
		name    string
		profile pkg.ActivityProfile
	}{
		{"normal", pkg.ActivityNormal},
		{"free", pkg.ActivityFree},
		{"sentinel", pkg.ActivitySentinel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := defaultPolicy()
			p.ActivityProfile = tt.profile
			plan := Resolve(p, minimalFacts(), defaultOpState())
			if plan.ActivityProfile != tt.profile {
				t.Errorf("ActivityProfile = %q, want %q", plan.ActivityProfile, tt.profile)
			}
		})
	}
}

func TestResolve_BridgePolicyFromPolicy(t *testing.T) {
	tests := []struct {
		name   string
		policy pkg.BridgePolicy
	}{
		{"auto_with_abstention", pkg.BridgeAutoWithAbstention},
		{"agent_requested", pkg.BridgeAgentRequested},
		{"disabled", pkg.BridgeDisabled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := defaultPolicy()
			p.BridgePolicy = tt.policy
			plan := Resolve(p, minimalFacts(), defaultOpState())
			if plan.BridgePolicy != tt.policy {
				t.Errorf("BridgePolicy = %q, want %q", plan.BridgePolicy, tt.policy)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Assembly profile resolution
// ---------------------------------------------------------------------------

func TestResolveAssemblyProfile_ContextCompaction(t *testing.T) {
	p := defaultPolicy()
	f := minimalFacts()
	f.SeamKind = pkg.SeamContextCompaction

	plan := Resolve(p, f, defaultOpState())
	if plan.AssemblyProfile != pkg.AssemblySeam {
		t.Errorf("AssemblyProfile = %q, want %q for context compaction seam", plan.AssemblyProfile, pkg.AssemblySeam)
	}
}

func TestResolveAssemblyProfile_InitialWake(t *testing.T) {
	p := defaultPolicy()
	f := minimalFacts()
	f.PrimaryCause = pkg.WakeCauseInitial

	plan := Resolve(p, f, defaultOpState())
	if plan.AssemblyProfile != pkg.AssemblyFull {
		t.Errorf("AssemblyProfile = %q, want %q for initial wake", plan.AssemblyProfile, pkg.AssemblyFull)
	}
}

func TestResolveAssemblyProfile_RecoveryWake(t *testing.T) {
	p := defaultPolicy()
	f := minimalFacts()
	f.PrimaryCause = pkg.WakeCauseRecovery

	plan := Resolve(p, f, defaultOpState())
	if plan.AssemblyProfile != pkg.AssemblyFull {
		t.Errorf("AssemblyProfile = %q, want %q for recovery wake", plan.AssemblyProfile, pkg.AssemblyFull)
	}
}

func TestResolveAssemblyProfile_ContinuousWarmReturn(t *testing.T) {
	p := defaultPolicy()
	p.TemporalMode = pkg.TemporalContinuous
	f := minimalFacts()
	f.SeamKind = pkg.SeamWarmReturn

	plan := Resolve(p, f, defaultOpState())
	if plan.AssemblyProfile != pkg.AssemblyLight {
		t.Errorf("AssemblyProfile = %q, want %q for continuous+warm_return", plan.AssemblyProfile, pkg.AssemblyLight)
	}
}

func TestResolveAssemblyProfile_ShortGap(t *testing.T) {
	p := defaultPolicy()
	f := minimalFacts()
	f.ElapsedDuration = 10 * time.Minute // < 30 min threshold

	plan := Resolve(p, f, defaultOpState())
	if plan.AssemblyProfile != pkg.AssemblyLight {
		t.Errorf("AssemblyProfile = %q, want %q for short gap (< 30 min)", plan.AssemblyProfile, pkg.AssemblyLight)
	}
}

func TestResolveAssemblyProfile_LongGap_DefaultsFull(t *testing.T) {
	p := defaultPolicy()
	p.DefaultAssembly = pkg.AssemblyFull
	f := minimalFacts()
	f.ElapsedDuration = 6 * time.Hour

	plan := Resolve(p, f, defaultOpState())
	if plan.AssemblyProfile != pkg.AssemblyFull {
		t.Errorf("AssemblyProfile = %q, want %q for long gap with full default", plan.AssemblyProfile, pkg.AssemblyFull)
	}
}

func TestResolveAssemblyProfile_LongGap_DefaultsMinimal(t *testing.T) {
	p := defaultPolicy()
	p.DefaultAssembly = pkg.AssemblyMinimal
	f := minimalFacts()
	f.ElapsedDuration = 6 * time.Hour

	plan := Resolve(p, f, defaultOpState())
	if plan.AssemblyProfile != pkg.AssemblyMinimal {
		t.Errorf("AssemblyProfile = %q, want %q for long gap with minimal default", plan.AssemblyProfile, pkg.AssemblyMinimal)
	}
}

func TestResolveAssemblyProfile_EmptyDefaultFallsBackToFull(t *testing.T) {
	p := defaultPolicy()
	p.DefaultAssembly = "" // unset
	f := minimalFacts()
	f.ElapsedDuration = 6 * time.Hour

	plan := Resolve(p, f, defaultOpState())
	if plan.AssemblyProfile != pkg.AssemblyFull {
		t.Errorf("AssemblyProfile = %q, want %q when DefaultAssembly is empty", plan.AssemblyProfile, pkg.AssemblyFull)
	}
}

func TestResolveAssemblyProfile_NegativeElapsed(t *testing.T) {
	// C5 fix: negative elapsed duration should not panic or produce wrong result
	p := defaultPolicy()
	f := minimalFacts()
	f.ElapsedDuration = -5 * time.Minute

	plan := Resolve(p, f, defaultOpState())
	if plan == nil {
		t.Fatal("Resolve returned nil for negative elapsed duration")
	}
	// Negative duration is < 30 min, so should produce light
	if plan.AssemblyProfile != pkg.AssemblyLight {
		t.Errorf("AssemblyProfile = %q, want %q for negative elapsed (treated as short)", plan.AssemblyProfile, pkg.AssemblyLight)
	}
}

// ---------------------------------------------------------------------------
// Disclosure generation
// ---------------------------------------------------------------------------

func TestResolve_Disclosures_ConfigurationChange(t *testing.T) {
	p := defaultPolicy()
	f := minimalFacts()
	f.TransitionContexts = []pkg.TransitionContext{pkg.TransitionConfigurationChange}

	plan := Resolve(p, f, defaultOpState())
	if len(plan.Disclosures) != 1 {
		t.Fatalf("expected 1 disclosure, got %d", len(plan.Disclosures))
	}
	if plan.Disclosures[0] != "lifecycle policy changed since last session" {
		t.Errorf("unexpected disclosure: %q", plan.Disclosures[0])
	}
}

func TestResolve_Disclosures_SubstrateChange(t *testing.T) {
	p := defaultPolicy()
	f := minimalFacts()
	f.TransitionContexts = []pkg.TransitionContext{pkg.TransitionSubstrateChange}

	plan := Resolve(p, f, defaultOpState())
	if len(plan.Disclosures) != 1 {
		t.Fatalf("expected 1 disclosure, got %d", len(plan.Disclosures))
	}
	if plan.Disclosures[0] != "model substrate changed since last session" {
		t.Errorf("unexpected disclosure: %q", plan.Disclosures[0])
	}
}

func TestResolve_Disclosures_RecoveryAfterFailure(t *testing.T) {
	p := defaultPolicy()
	f := minimalFacts()
	f.TransitionContexts = []pkg.TransitionContext{pkg.TransitionRecoveryAfterFailure}

	plan := Resolve(p, f, defaultOpState())
	if len(plan.Disclosures) != 1 {
		t.Fatalf("expected 1 disclosure, got %d", len(plan.Disclosures))
	}
	if plan.Disclosures[0] != "recovering from prior session failure" {
		t.Errorf("unexpected disclosure: %q", plan.Disclosures[0])
	}
}

func TestResolve_Disclosures_IdentityDocumentChange(t *testing.T) {
	p := defaultPolicy()
	f := minimalFacts()
	f.TransitionContexts = []pkg.TransitionContext{pkg.TransitionIdentityDocumentChange}

	plan := Resolve(p, f, defaultOpState())
	if len(plan.Disclosures) != 1 {
		t.Fatalf("expected 1 disclosure, got %d", len(plan.Disclosures))
	}
	if plan.Disclosures[0] != "identity documents modified since last session" {
		t.Errorf("unexpected disclosure: %q", plan.Disclosures[0])
	}
}

func TestResolve_Disclosures_NoneTransition(t *testing.T) {
	p := defaultPolicy()
	f := minimalFacts()
	f.TransitionContexts = []pkg.TransitionContext{pkg.TransitionNone}

	plan := Resolve(p, f, defaultOpState())
	if len(plan.Disclosures) != 0 {
		t.Errorf("TransitionNone should produce no disclosures, got %d", len(plan.Disclosures))
	}
}

func TestResolve_Disclosures_ModeChange(t *testing.T) {
	p := defaultPolicy()
	f := minimalFacts()
	f.TransitionContexts = []pkg.TransitionContext{pkg.TransitionModeChange}

	plan := Resolve(p, f, defaultOpState())
	if len(plan.Disclosures) != 0 {
		t.Errorf("TransitionModeChange should produce no disclosures, got %d", len(plan.Disclosures))
	}
}

func TestResolve_Disclosures_Multiple(t *testing.T) {
	p := defaultPolicy()
	f := minimalFacts()
	f.TransitionContexts = []pkg.TransitionContext{
		pkg.TransitionConfigurationChange,
		pkg.TransitionSubstrateChange,
		pkg.TransitionRecoveryAfterFailure,
	}

	plan := Resolve(p, f, defaultOpState())
	if len(plan.Disclosures) != 3 {
		t.Fatalf("expected 3 disclosures, got %d", len(plan.Disclosures))
	}
}

func TestResolve_Disclosures_DeduplicatesDuplicateContexts(t *testing.T) {
	p := defaultPolicy()
	f := minimalFacts()
	f.TransitionContexts = []pkg.TransitionContext{
		pkg.TransitionConfigurationChange,
		pkg.TransitionConfigurationChange, // duplicate
	}

	plan := Resolve(p, f, defaultOpState())
	if len(plan.Disclosures) != 1 {
		t.Errorf("expected 1 disclosure (deduplicated), got %d", len(plan.Disclosures))
	}
}

func TestResolve_Disclosures_EmptyContexts(t *testing.T) {
	p := defaultPolicy()
	f := minimalFacts()
	f.TransitionContexts = nil

	plan := Resolve(p, f, defaultOpState())
	if plan.Disclosures != nil {
		t.Errorf("nil TransitionContexts should produce nil Disclosures, got %v", plan.Disclosures)
	}
}

// ---------------------------------------------------------------------------
// Reasons map
// ---------------------------------------------------------------------------

func TestResolve_ReasonsPopulated(t *testing.T) {
	plan := Resolve(defaultPolicy(), minimalFacts(), defaultOpState())

	requiredKeys := []string{"temporal_mode", "activity_profile", "assembly_profile", "bridge_policy"}
	for _, key := range requiredKeys {
		if _, ok := plan.Reasons[key]; !ok {
			t.Errorf("Reasons missing key %q", key)
		}
	}
}

// ---------------------------------------------------------------------------
// Priority tests: seam > cause > elapsed
// ---------------------------------------------------------------------------

func TestResolveAssemblyProfile_CompactionSupersedesRecovery(t *testing.T) {
	// Seam mechanism supersedes wake cause per ontology
	p := defaultPolicy()
	f := minimalFacts()
	f.SeamKind = pkg.SeamContextCompaction
	f.PrimaryCause = pkg.WakeCauseRecovery

	plan := Resolve(p, f, defaultOpState())
	if plan.AssemblyProfile != pkg.AssemblySeam {
		t.Errorf("AssemblyProfile = %q, want %q — compaction supersedes recovery", plan.AssemblyProfile, pkg.AssemblySeam)
	}
}

func TestResolveAssemblyProfile_InitialSupersedesShortGap(t *testing.T) {
	p := defaultPolicy()
	f := minimalFacts()
	f.PrimaryCause = pkg.WakeCauseInitial
	f.ElapsedDuration = 5 * time.Minute

	plan := Resolve(p, f, defaultOpState())
	if plan.AssemblyProfile != pkg.AssemblyFull {
		t.Errorf("AssemblyProfile = %q, want %q — initial supersedes short gap", plan.AssemblyProfile, pkg.AssemblyFull)
	}
}
