package session

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// policyDocument is the JSON file representation. Fields are strings that get
// validated and converted to typed enums in pkg.LifecyclePolicy.
type policyDocument struct {
	TemporalMode     string `json:"temporal_mode"`
	BridgePolicy     string `json:"bridge_policy"`
	MetabolismPolicy string `json:"metabolism_policy"`
	AssemblyProfile  string `json:"assembly_profile"`
}

func (d *policyDocument) validate() error {
	if d.TemporalMode != "" {
		if _, ok := pkg.ValidTemporalModes[pkg.TemporalMode(d.TemporalMode)]; !ok {
			return fmt.Errorf("policy: invalid temporal_mode %q", d.TemporalMode)
		}
	}
	if d.BridgePolicy != "" {
		if !pkg.ValidBridgePolicies[pkg.BridgePolicy(d.BridgePolicy)] {
			return fmt.Errorf("policy: invalid bridge_policy %q", d.BridgePolicy)
		}
	}
	if d.MetabolismPolicy != "" {
		if !pkg.ValidMetabolismPolicies[d.MetabolismPolicy] {
			return fmt.Errorf("policy: invalid metabolism_policy %q", d.MetabolismPolicy)
		}
	}
	if d.AssemblyProfile != "" {
		if !pkg.ValidAssemblyProfiles[pkg.AssemblyProfile(d.AssemblyProfile)] {
			return fmt.Errorf("policy: invalid assembly_profile %q", d.AssemblyProfile)
		}
	}
	return nil
}

func (d *policyDocument) toCanonical(hash string) pkg.LifecyclePolicy {
	p := pkg.LifecyclePolicy{
		TemporalMode:    pkg.TemporalEpisodic,
		DefaultAssembly: pkg.AssemblyFull,
		BridgePolicy:    pkg.BridgeAgentRequested,
		ActivityProfile: pkg.ActivityNormal,
		CommitHash:      hash,
	}
	if d.TemporalMode != "" {
		p.TemporalMode = pkg.TemporalMode(d.TemporalMode)
	}
	if d.AssemblyProfile != "" {
		p.DefaultAssembly = pkg.AssemblyProfile(d.AssemblyProfile)
	}
	if d.BridgePolicy != "" {
		p.BridgePolicy = pkg.BridgePolicy(d.BridgePolicy)
	}
	if d.MetabolismPolicy != "" {
		p.MetabolismPolicy = d.MetabolismPolicy
	}
	return p
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

// Read reads and validates the lifecycle policy file, returning the canonical
// pkg.LifecyclePolicy type directly. Returns the policy, its SHA-256 hash,
// and any error. If the file does not exist, returns (zero-value, "", nil).
func (r *PolicyReader) Read() (pkg.LifecyclePolicy, string, error) {
	path := r.PolicyPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return pkg.LifecyclePolicy{}, "", nil
		}
		return pkg.LifecyclePolicy{}, "", fmt.Errorf("policy: reading %s: %w", path, err)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var doc policyDocument
	if err := dec.Decode(&doc); err != nil {
		return pkg.LifecyclePolicy{}, "", fmt.Errorf("policy: parsing %s: %w", path, err)
	}

	if err := doc.validate(); err != nil {
		return pkg.LifecyclePolicy{}, "", err
	}

	hash := sha256.Sum256(data)
	hexHash := hex.EncodeToString(hash[:])

	return doc.toCanonical(hexHash), hexHash, nil
}

// HasChanged compares the given hash against the last applied configuration hash.
// Returns (changed, previousHash, error).
// If no previous record exists, returns (true, "", nil).
// If db is nil, returns (false, "", nil) — no comparison possible.
func (r *PolicyReader) HasChanged(ctx context.Context, currentHash string) (bool, string, error) {
	if r.db == nil {
		return false, "", nil
	}

	row := r.db.QueryRowContext(ctx,
		`SELECT policy_hash FROM configuration_applied
		 ORDER BY applied_at DESC LIMIT 1`,
	)

	var previousHash string
	if err := row.Scan(&previousHash); err != nil {
		if err == sql.ErrNoRows {
			return true, "", nil
		}
		return false, "", fmt.Errorf("policy: querying last applied hash: %w", err)
	}

	return currentHash != previousHash, previousHash, nil
}
