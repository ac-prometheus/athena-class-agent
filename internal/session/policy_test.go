package session

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// ---------------------------------------------------------------------------
// Policy tests — Sprint 3B, updated for HARN-84 policy unification
// ---------------------------------------------------------------------------

func writePolicyFile(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "lifecycle.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestPolicyReader_Read_ValidFile_AllFields(t *testing.T) {
	dir := t.TempDir()
	writePolicyFile(t, dir, `{
		"temporal_mode": "episodic",
		"bridge_policy": "agent_requested",
		"metabolism_policy": "standard",
		"assembly_profile": "full"
	}`)

	reader := NewPolicyReader(dir, nil)
	got, hash, err := reader.Read()
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if got.TemporalMode != pkg.TemporalEpisodic {
		t.Errorf("TemporalMode = %q, want %q", got.TemporalMode, pkg.TemporalEpisodic)
	}
	if got.DefaultAssembly != pkg.AssemblyFull {
		t.Errorf("DefaultAssembly = %q, want %q", got.DefaultAssembly, pkg.AssemblyFull)
	}
	if got.BridgePolicy != pkg.BridgeAgentRequested {
		t.Errorf("BridgePolicy = %q, want %q", got.BridgePolicy, pkg.BridgeAgentRequested)
	}
	if got.MetabolismPolicy != "standard" {
		t.Errorf("MetabolismPolicy = %q, want %q", got.MetabolismPolicy, "standard")
	}
	if hash == "" {
		t.Error("hash should not be empty")
	}
}

func TestPolicyReader_Read_ValidCombinations(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{"diurnal/auto_abstention/deferred/light", `{"temporal_mode":"diurnal","bridge_policy":"automatic_with_abstention","metabolism_policy":"deferred","assembly_profile":"light"}`},
		{"continuous/disabled/skip/minimal", `{"temporal_mode":"continuous","bridge_policy":"disabled","metabolism_policy":"skip","assembly_profile":"minimal"}`},
		{"episodic/agent_requested/standard/seam", `{"temporal_mode":"episodic","bridge_policy":"agent_requested","metabolism_policy":"standard","assembly_profile":"seam"}`},
		{"empty defaults", `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writePolicyFile(t, dir, tt.json)
			reader := NewPolicyReader(dir, nil)
			_, _, err := reader.Read()
			if err != nil {
				t.Errorf("Read() returned error for valid policy: %v", err)
			}
		})
	}
}

func TestPolicyReader_Read_InvalidTemporalMode(t *testing.T) {
	dir := t.TempDir()
	writePolicyFile(t, dir, `{"temporal_mode": "always_on"}`)
	reader := NewPolicyReader(dir, nil)
	_, _, err := reader.Read()
	if err == nil {
		t.Error("expected error for invalid temporal_mode")
	}
}

func TestPolicyReader_Read_InvalidBridgePolicy(t *testing.T) {
	dir := t.TempDir()
	writePolicyFile(t, dir, `{"bridge_policy": "automatic"}`)
	reader := NewPolicyReader(dir, nil)
	_, _, err := reader.Read()
	if err == nil {
		t.Error("expected error for invalid bridge_policy")
	}
}

func TestPolicyReader_Read_InvalidMetabolismPolicy(t *testing.T) {
	dir := t.TempDir()
	writePolicyFile(t, dir, `{"metabolism_policy": "fast"}`)
	reader := NewPolicyReader(dir, nil)
	_, _, err := reader.Read()
	if err == nil {
		t.Error("expected error for invalid metabolism_policy")
	}
}

func TestPolicyReader_Read_InvalidAssemblyProfile(t *testing.T) {
	dir := t.TempDir()
	writePolicyFile(t, dir, `{"assembly_profile": "heavy"}`)
	reader := NewPolicyReader(dir, nil)
	_, _, err := reader.Read()
	if err == nil {
		t.Error("expected error for invalid assembly_profile")
	}
}

func TestPolicyReader_Read_CaseSensitive(t *testing.T) {
	invalids := []struct {
		field string
		json  string
	}{
		{"temporal_mode", `{"temporal_mode":"EPISODIC"}`},
		{"temporal_mode", `{"temporal_mode":"Episodic"}`},
		{"bridge_policy", `{"bridge_policy":"OPT_IN"}`},
		{"metabolism_policy", `{"metabolism_policy":"STANDARD"}`},
		{"assembly_profile", `{"assembly_profile":"FULL"}`},
	}
	for _, tt := range invalids {
		t.Run(tt.field, func(t *testing.T) {
			dir := t.TempDir()
			writePolicyFile(t, dir, tt.json)
			reader := NewPolicyReader(dir, nil)
			_, _, err := reader.Read()
			if err == nil {
				t.Errorf("expected error for %s with uppercase value", tt.field)
			}
		})
	}
}

func TestPolicyReader_Read_UnknownFields(t *testing.T) {
	dir := t.TempDir()
	writePolicyFile(t, dir, `{"temporal_mode":"episodic","unknown_field":"value"}`)
	reader := NewPolicyReader(dir, nil)
	_, _, err := reader.Read()
	if err == nil {
		t.Error("expected error for unknown JSON field")
	}
}

func TestPolicyReader_Read_MissingFile(t *testing.T) {
	dir := t.TempDir()
	reader := NewPolicyReader(dir, nil)

	got, hash, err := reader.Read()
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if got != (pkg.LifecyclePolicy{}) {
		t.Error("expected zero-value policy for missing file")
	}
	if hash != "" {
		t.Errorf("expected empty hash for missing file, got %q", hash)
	}
}

func TestPolicyReader_Read_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	writePolicyFile(t, dir, "{bad json")
	reader := NewPolicyReader(dir, nil)
	_, _, err := reader.Read()
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestPolicyReader_Read_DefaultsApplied(t *testing.T) {
	dir := t.TempDir()
	writePolicyFile(t, dir, `{}`)
	reader := NewPolicyReader(dir, nil)
	got, _, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if got.TemporalMode != pkg.TemporalEpisodic {
		t.Errorf("default TemporalMode = %q, want %q", got.TemporalMode, pkg.TemporalEpisodic)
	}
	if got.DefaultAssembly != pkg.AssemblyFull {
		t.Errorf("default DefaultAssembly = %q, want %q", got.DefaultAssembly, pkg.AssemblyFull)
	}
	if got.BridgePolicy != pkg.BridgeAgentRequested {
		t.Errorf("default BridgePolicy = %q, want %q", got.BridgePolicy, pkg.BridgeAgentRequested)
	}
}

func TestPolicyReader_SHA256_Deterministic(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{"temporal_mode":"episodic","bridge_policy":"agent_requested"}`)
	writePolicyFile(t, dir, string(data))

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

	expected := sha256.Sum256(data)
	expectedHex := hex.EncodeToString(expected[:])
	if hash1 != expectedHex {
		t.Errorf("hash = %q, want %q", hash1, expectedHex)
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

// Cross-check: pkg validation map values match SQL CHECK constraints
func TestValidationMaps_MatchSQL(t *testing.T) {
	t.Run("temporal_modes", func(t *testing.T) {
		sqlValues := []pkg.TemporalMode{"episodic", "diurnal", "continuous"}
		for _, v := range sqlValues {
			if !pkg.ValidTemporalModes[v] {
				t.Errorf("SQL temporal_mode %q not in ValidTemporalModes", v)
			}
		}
		if len(pkg.ValidTemporalModes) != len(sqlValues) {
			t.Errorf("ValidTemporalModes has %d entries, SQL CHECK has %d", len(pkg.ValidTemporalModes), len(sqlValues))
		}
	})

	t.Run("bridge_policies", func(t *testing.T) {
		sqlValues := []pkg.BridgePolicy{"automatic_with_abstention", "agent_requested", "disabled"}
		for _, v := range sqlValues {
			if !pkg.ValidBridgePolicies[v] {
				t.Errorf("SQL bridge_policy %q not in ValidBridgePolicies", v)
			}
		}
		if len(pkg.ValidBridgePolicies) != len(sqlValues) {
			t.Errorf("ValidBridgePolicies has %d entries, SQL CHECK has %d", len(pkg.ValidBridgePolicies), len(sqlValues))
		}
	})

	t.Run("metabolism_policies", func(t *testing.T) {
		sqlValues := []string{"standard", "deferred", "skip"}
		for _, v := range sqlValues {
			if !pkg.ValidMetabolismPolicies[v] {
				t.Errorf("SQL metabolism_policy %q not in ValidMetabolismPolicies", v)
			}
		}
		if len(pkg.ValidMetabolismPolicies) != len(sqlValues) {
			t.Errorf("ValidMetabolismPolicies has %d entries, SQL CHECK has %d", len(pkg.ValidMetabolismPolicies), len(sqlValues))
		}
	})

	t.Run("assembly_profiles", func(t *testing.T) {
		sqlValues := []pkg.AssemblyProfile{"full", "light", "minimal", "seam"}
		for _, v := range sqlValues {
			if !pkg.ValidAssemblyProfiles[v] {
				t.Errorf("SQL assembly_profile %q not in ValidAssemblyProfiles", v)
			}
		}
		if len(pkg.ValidAssemblyProfiles) != len(sqlValues) {
			t.Errorf("ValidAssemblyProfiles has %d entries, SQL CHECK has %d", len(pkg.ValidAssemblyProfiles), len(sqlValues))
		}
	})
}
