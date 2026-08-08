package session

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
)

// LifecyclePolicy is the git-tracked lifecycle configuration for an agent.
// Stored as JSON (or YAML when gopkg.in/yaml.v3 is available) at a configurable
// workspace path (e.g. workspace/lifecycle.json).
type LifecyclePolicy struct {
	TemporalMode     string `json:"temporal_mode"`
	BridgePolicy     string `json:"bridge_policy"`
	MetabolismPolicy string `json:"metabolism_policy"`
	AssemblyProfile  string `json:"assembly_profile"`
}

// validTemporalModes is the set of valid temporal_mode values.
var validTemporalModes = map[string]bool{
	"episodic": true, "diurnal": true, "continuous": true,
}

// validBridgePolicies is the set of valid bridge_policy values.
var validBridgePolicies = map[string]bool{
	"opt_in": true, "always": true, "never": true, "abstain": true,
}

// validMetabolismPolicies is the set of valid metabolism_policy values.
var validMetabolismPolicies = map[string]bool{
	"standard": true, "deferred": true, "skip": true,
}

// validAssemblyProfiles is the set of valid assembly_profile values.
var validAssemblyProfiles = map[string]bool{
	"full": true, "light": true, "minimal": true, "seam": true,
}

// Validate checks that all policy fields are valid values.
func (p *LifecyclePolicy) Validate() error {
	if p.TemporalMode != "" && !validTemporalModes[p.TemporalMode] {
		return fmt.Errorf("policy: invalid temporal_mode %q", p.TemporalMode)
	}
	if p.BridgePolicy != "" && !validBridgePolicies[p.BridgePolicy] {
		return fmt.Errorf("policy: invalid bridge_policy %q", p.BridgePolicy)
	}
	if p.MetabolismPolicy != "" && !validMetabolismPolicies[p.MetabolismPolicy] {
		return fmt.Errorf("policy: invalid metabolism_policy %q", p.MetabolismPolicy)
	}
	if p.AssemblyProfile != "" && !validAssemblyProfiles[p.AssemblyProfile] {
		return fmt.Errorf("policy: invalid assembly_profile %q", p.AssemblyProfile)
	}
	return nil
}

// PolicyReader reads lifecycle policy from a workspace file and compares
// against the last applied hash stored in the configuration_applied table.
type PolicyReader struct {
	workspacePath string
	db            platform.DB
}

// NewPolicyReader creates a PolicyReader that looks for the policy file at
// filepath.Join(workspacePath, "lifecycle.json").
func NewPolicyReader(workspacePath string, db platform.DB) *PolicyReader {
	return &PolicyReader{
		workspacePath: workspacePath,
		db:            db,
	}
}

// PolicyPath returns the full path to the lifecycle policy file.
func (r *PolicyReader) PolicyPath() string {
	return filepath.Join(r.workspacePath, "lifecycle.json")
}

// Read reads and validates the lifecycle policy file.
// Returns the policy, its SHA-256 hash, and any error.
// If the file does not exist, returns (nil, "", nil).
func (r *PolicyReader) Read() (*LifecyclePolicy, string, error) {
	path := r.PolicyPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("policy: reading %s: %w", path, err)
	}

	var policy LifecyclePolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		return nil, "", fmt.Errorf("policy: parsing %s: %w", path, err)
	}

	if err := policy.Validate(); err != nil {
		return nil, "", err
	}

	hash := sha256.Sum256(data)
	hexHash := hex.EncodeToString(hash[:])

	return &policy, hexHash, nil
}

// HasChanged compares the given hash against the last applied configuration hash.
// Returns (changed, previousHash, error).
// If no previous record exists, returns (true, "", nil).
// If db is nil, returns (false, "", nil) — no comparison possible.
func (r *PolicyReader) HasChanged(currentHash string) (bool, string, error) {
	if r.db == nil {
		return false, "", nil
	}

	row := r.db.QueryRowContext(context.Background(),
		`SELECT policy_hash FROM configuration_applied
		 ORDER BY applied_at DESC LIMIT 1`,
	)

	var previousHash string
	if err := row.Scan(&previousHash); err != nil {
		if err == sql.ErrNoRows {
			// No previous record — this is the first application.
			return true, "", nil
		}
		return false, "", fmt.Errorf("policy: querying last applied hash: %w", err)
	}

	return currentHash != previousHash, previousHash, nil
}
