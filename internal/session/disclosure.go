package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
)

// ConfigDisclosure records a configuration change and provides a human-readable
// summary for injection into context assembly.
type ConfigDisclosure struct {
	PolicyPath string
	OldHash    string
	NewHash    string
	Changes    []string // human-readable change descriptions
	AppliedAt  time.Time
}

// GenerateDisclosure compares two lifecycle policies and produces a disclosure
// listing what changed. If old is nil, all fields in new are reported as initial values.
func GenerateDisclosure(old, new *LifecyclePolicy) *ConfigDisclosure {
	d := &ConfigDisclosure{
		AppliedAt: time.Now().UTC(),
	}

	if old == nil {
		// First application — report all non-empty fields as initial configuration.
		if new.TemporalMode != "" {
			d.Changes = append(d.Changes, fmt.Sprintf("temporal_mode set to %q", new.TemporalMode))
		}
		if new.BridgePolicy != "" {
			d.Changes = append(d.Changes, fmt.Sprintf("bridge_policy set to %q", new.BridgePolicy))
		}
		if new.MetabolismPolicy != "" {
			d.Changes = append(d.Changes, fmt.Sprintf("metabolism_policy set to %q", new.MetabolismPolicy))
		}
		if new.AssemblyProfile != "" {
			d.Changes = append(d.Changes, fmt.Sprintf("assembly_profile set to %q", new.AssemblyProfile))
		}
		if len(d.Changes) == 0 {
			d.Changes = append(d.Changes, "initial policy applied (all defaults)")
		}
		return d
	}

	// Compare each field.
	if old.TemporalMode != new.TemporalMode {
		d.Changes = append(d.Changes,
			fmt.Sprintf("temporal_mode changed from %q to %q", old.TemporalMode, new.TemporalMode))
	}
	if old.BridgePolicy != new.BridgePolicy {
		d.Changes = append(d.Changes,
			fmt.Sprintf("bridge_policy changed from %q to %q", old.BridgePolicy, new.BridgePolicy))
	}
	if old.MetabolismPolicy != new.MetabolismPolicy {
		d.Changes = append(d.Changes,
			fmt.Sprintf("metabolism_policy changed from %q to %q", old.MetabolismPolicy, new.MetabolismPolicy))
	}
	if old.AssemblyProfile != new.AssemblyProfile {
		d.Changes = append(d.Changes,
			fmt.Sprintf("assembly_profile changed from %q to %q", old.AssemblyProfile, new.AssemblyProfile))
	}

	if len(d.Changes) == 0 {
		d.Changes = append(d.Changes, "no field-level changes detected (file content changed)")
	}

	return d
}

// ForContext formats the disclosure for injection into context assembly.
// The output is a concise, multi-line block suitable for the orientation phase.
func (d *ConfigDisclosure) ForContext() string {
	var b strings.Builder
	b.WriteString("[configuration change]\n")
	if d.PolicyPath != "" {
		b.WriteString(fmt.Sprintf("Policy: %s\n", d.PolicyPath))
	}
	for _, change := range d.Changes {
		b.WriteString(fmt.Sprintf("  - %s\n", change))
	}
	if d.OldHash != "" {
		hashPreview := d.OldHash
		if len(hashPreview) > 12 {
			hashPreview = hashPreview[:12] + "..."
		}
		b.WriteString(fmt.Sprintf("Previous hash: %s\n", hashPreview))
	}
	b.WriteString(fmt.Sprintf("Applied at: %s", d.AppliedAt.Format(time.RFC3339)))
	return b.String()
}

// RecordDisclosure persists the configuration change in the configuration_applied table.
// Returns an error if the database write fails. Requires a non-nil db.
func RecordDisclosure(ctx context.Context, db platform.DB, sessionID string, d *ConfigDisclosure) error {
	if db == nil {
		return nil
	}

	summary := strings.Join(d.Changes, "; ")

	_, err := db.ExecContext(ctx,
		`INSERT INTO configuration_applied
			(session_id, policy_path, policy_hash, previous_hash, changes_summary, applied_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		sessionID, d.PolicyPath, d.NewHash, nilIfEmpty(d.OldHash), summary, d.AppliedAt,
	)
	if err != nil {
		return fmt.Errorf("disclosure: recording config change: %w", err)
	}
	return nil
}

// nilIfEmpty returns nil for empty strings, otherwise a pointer-compatible value.
func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
