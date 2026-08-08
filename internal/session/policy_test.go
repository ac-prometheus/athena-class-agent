package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// Policy tests — Sprint 3B
// ---------------------------------------------------------------------------

func TestLifecyclePolicy_Validate_AllValid(t *testing.T) {
	tests := []struct {
		name   string
		policy LifecyclePolicy
	}{
		{
			name: "all fields set",
			policy: LifecyclePolicy{
				TemporalMode:     "episodic",
				BridgePolicy:     "opt_in",
				MetabolismPolicy: "standard",
				AssemblyProfile:  "full",
			},
		},
		{
			name: "diurnal/always/deferred/light",
			policy: LifecyclePolicy{
				TemporalMode:     "diurnal",
				BridgePolicy:     "always",
				MetabolismPolicy: "deferred",
				AssemblyProfile:  "light",
			},
		},
		{
			name: "continuous/never/skip/minimal",
			policy: LifecyclePolicy{
				TemporalMode:     "continuous",
				BridgePolicy:     "never",
				MetabolismPolicy: "skip",
				AssemblyProfile:  "minimal",
			},
		},
		{
			name: "abstain bridge/seam assembly",
			policy: LifecyclePolicy{
				TemporalMode:     "episodic",
				BridgePolicy:     "abstain",
				MetabolismPolicy: "standard",
				AssemblyProfile:  "seam",
			},
		},
		{
			name:   "all empty (defaults)",
			policy: LifecyclePolicy{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.policy.Validate(); err != nil {
				t.Errorf("Validate() returned error for valid policy: %v", err)
			}
		})
	}
}

func TestLifecyclePolicy_Validate_InvalidTemporalMode(t *testing.T) {
	p := LifecyclePolicy{TemporalMode: "always_on"}
	err := p.Validate()
	if err == nil {
		t.Error("expected error for invalid temporal_mode")
	}
}

func TestLifecyclePolicy_Validate_InvalidBridgePolicy(t *testing.T) {
	p := LifecyclePolicy{BridgePolicy: "automatic"}
	err := p.Validate()
	if err == nil {
		t.Error("expected error for invalid bridge_policy")
	}
}

func TestLifecyclePolicy_Validate_InvalidMetabolismPolicy(t *testing.T) {
	p := LifecyclePolicy{MetabolismPolicy: "fast"}
	err := p.Validate()
	if err == nil {
		t.Error("expected error for invalid metabolism_policy")
	}
}

func TestLifecyclePolicy_Validate_InvalidAssemblyProfile(t *testing.T) {
	p := LifecyclePolicy{AssemblyProfile: "heavy"}
	err := p.Validate()
	if err == nil {
		t.Error("expected error for invalid assembly_profile")
	}
}

func TestLifecyclePolicy_Validate_AllInvalid(t *testing.T) {
	invalids := []struct {
		field string
		p     LifecyclePolicy
	}{
		{"temporal_mode", LifecyclePolicy{TemporalMode: "EPISODIC"}},
		{"temporal_mode", LifecyclePolicy{TemporalMode: "Episodic"}},
		{"bridge_policy", LifecyclePolicy{BridgePolicy: "OPT_IN"}},
		{"metabolism_policy", LifecyclePolicy{MetabolismPolicy: "STANDARD"}},
		{"assembly_profile", LifecyclePolicy{AssemblyProfile: "FULL"}},
	}
	for _, tt := range invalids {
		t.Run(tt.field+"_"+tt.p.TemporalMode+tt.p.BridgePolicy+tt.p.MetabolismPolicy+tt.p.AssemblyProfile, func(t *testing.T) {
			if err := tt.p.Validate(); err == nil {
				t.Errorf("expected error for %s with uppercase value", tt.field)
			}
		})
	}
}

func TestPolicyReader_Read_ValidFile(t *testing.T) {
	dir := t.TempDir()
	policy := LifecyclePolicy{
		TemporalMode:     "episodic",
		BridgePolicy:     "opt_in",
		MetabolismPolicy: "standard",
		AssemblyProfile:  "full",
	}
	data, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lifecycle.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	reader := NewPolicyReader(dir, nil)
	got, hash, err := reader.Read()
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if got == nil {
		t.Fatal("Read() returned nil policy for valid file")
	}
	if got.TemporalMode != "episodic" {
		t.Errorf("TemporalMode = %q, want %q", got.TemporalMode, "episodic")
	}
	if hash == "" {
		t.Error("hash should not be empty")
	}

	// Verify hash is deterministic
	expected := sha256.Sum256(data)
	expectedHex := hex.EncodeToString(expected[:])
	if hash != expectedHex {
		t.Errorf("hash = %q, want %q", hash, expectedHex)
	}
}

func TestPolicyReader_Read_MissingFile(t *testing.T) {
	dir := t.TempDir()
	reader := NewPolicyReader(dir, nil)

	got, hash, err := reader.Read()
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if got != nil {
		t.Error("expected nil policy for missing file")
	}
	if hash != "" {
		t.Errorf("expected empty hash for missing file, got %q", hash)
	}
}

func TestPolicyReader_Read_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lifecycle.json"), []byte("{bad json"), 0644); err != nil {
		t.Fatal(err)
	}

	reader := NewPolicyReader(dir, nil)
	_, _, err := reader.Read()
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestPolicyReader_Read_InvalidValues(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{"temporal_mode": "invalid_mode"}`)
	if err := os.WriteFile(filepath.Join(dir, "lifecycle.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	reader := NewPolicyReader(dir, nil)
	_, _, err := reader.Read()
	if err == nil {
		t.Error("expected error for invalid temporal_mode value")
	}
}

func TestPolicyReader_SHA256_Deterministic(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{"temporal_mode":"episodic","bridge_policy":"opt_in"}`)
	if err := os.WriteFile(filepath.Join(dir, "lifecycle.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	reader := NewPolicyReader(dir, nil)
	_, hash1, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	_, hash2, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}

	if hash1 != hash2 {
		t.Errorf("SHA-256 not deterministic: %q vs %q", hash1, hash2)
	}
}

func TestPolicyReader_PolicyPath(t *testing.T) {
	reader := NewPolicyReader("/workspace/agent", nil)
	expected := filepath.Join("/workspace/agent", "lifecycle.json")
	if reader.PolicyPath() != expected {
		t.Errorf("PolicyPath() = %q, want %q", reader.PolicyPath(), expected)
	}
}

func TestPolicyReader_HasChanged_NilDB(t *testing.T) {
	reader := NewPolicyReader(".", nil)
	changed, prev, err := reader.HasChanged(nil, "somehash")
	if err != nil {
		t.Fatalf("HasChanged error: %v", err)
	}
	if changed {
		t.Error("expected changed=false when db is nil")
	}
	if prev != "" {
		t.Errorf("expected empty previous hash when db is nil, got %q", prev)
	}
}

// Cross-check: policy validation map values match SQL CHECK constraints
func TestValidationMaps_MatchSQL(t *testing.T) {
	t.Run("temporal_modes", func(t *testing.T) {
		sqlValues := []string{"episodic", "diurnal", "continuous"}
		for _, v := range sqlValues {
			if !validTemporalModes[v] {
				t.Errorf("SQL temporal_mode %q not in validTemporalModes", v)
			}
		}
		if len(validTemporalModes) != len(sqlValues) {
			t.Errorf("validTemporalModes has %d entries, SQL CHECK has %d", len(validTemporalModes), len(sqlValues))
		}
	})

	t.Run("bridge_policies", func(t *testing.T) {
		sqlValues := []string{"opt_in", "always", "never", "abstain"}
		for _, v := range sqlValues {
			if !validBridgePolicies[v] {
				t.Errorf("SQL bridge_policy %q not in validBridgePolicies", v)
			}
		}
		if len(validBridgePolicies) != len(sqlValues) {
			t.Errorf("validBridgePolicies has %d entries, SQL CHECK has %d", len(validBridgePolicies), len(sqlValues))
		}
	})

	t.Run("metabolism_policies", func(t *testing.T) {
		sqlValues := []string{"standard", "deferred", "skip"}
		for _, v := range sqlValues {
			if !validMetabolismPolicies[v] {
				t.Errorf("SQL metabolism_policy %q not in validMetabolismPolicies", v)
			}
		}
		if len(validMetabolismPolicies) != len(sqlValues) {
			t.Errorf("validMetabolismPolicies has %d entries, SQL CHECK has %d", len(validMetabolismPolicies), len(sqlValues))
		}
	})

	t.Run("assembly_profiles", func(t *testing.T) {
		sqlValues := []string{"full", "light", "minimal", "seam"}
		for _, v := range sqlValues {
			if !validAssemblyProfiles[v] {
				t.Errorf("SQL assembly_profile %q not in validAssemblyProfiles", v)
			}
		}
		if len(validAssemblyProfiles) != len(sqlValues) {
			t.Errorf("validAssemblyProfiles has %d entries, SQL CHECK has %d", len(validAssemblyProfiles), len(sqlValues))
		}
	})
}
