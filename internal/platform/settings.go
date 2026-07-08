package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

// SettingsEntry is a row in agent_settings.
// Owner controls who can modify the value:
//
//	"operator"    — read-only for the agent; only the operator can change it
//	"agent"       — the agent can change within declared bounds
//	"negotiated"  — agent proposes, operator co-signs before the value applies
//
// ConsentRequired is true for negotiated settings where agent proposals must be
// approved by the operator before taking effect.
type SettingsEntry struct {
	Name            string
	Value           string    // JSON-encoded current value
	DefaultValue    string    // JSON-encoded default
	BoundsMin       string    // extracted from bounds JSON; empty if enum-bounded or unbounded
	BoundsMax       string    // extracted from bounds JSON; empty if enum-bounded or unbounded
	Owner           string    // "operator", "agent", "negotiated"
	ConsentRequired bool
	Description     string
	LastChanged     time.Time
	ChangedBy       string
	Reason          string
}

// SettingsStore handles agent_settings persistence.
// Implementations on SQLiteStore and PostgresStore are below.
type SettingsStore interface {
	// GetSetting returns a single setting by name, or (nil, nil) if not found.
	GetSetting(ctx context.Context, name string) (*SettingsEntry, error)
	// ListSettings returns all settings. Pass ownerFilter="" to return all owners.
	ListSettings(ctx context.Context, ownerFilter string) ([]SettingsEntry, error)
	// UpsertSetting creates or updates a setting entry.
	UpsertSetting(ctx context.Context, entry SettingsEntry) error
	// ProposeSetting logs a proposal for a negotiated or agent-owned setting.
	// For owner="agent" settings, the value is applied directly and logged to operator_actions.
	// For owner="negotiated" settings, the proposal is written to operator_actions with
	// action_type="settings_proposal" and does NOT update agent_settings until co-signed.
	ProposeSetting(ctx context.Context, name, value, reason, proposedBy string) error
}

// parseBounds extracts min/max strings from a bounds JSON blob.
// Bounds format: {"min": <val>, "max": <val>} or {"enum": [...]}
func parseBounds(boundsJSON string) (min, max string) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(boundsJSON), &m); err != nil {
		return "", ""
	}
	if v, ok := m["min"]; ok {
		min = string(v)
	}
	if v, ok := m["max"]; ok {
		max = string(v)
	}
	return min, max
}

// scanSettingsRow scans a settings row into a SettingsEntry.
func scanSettingsRow(scan func(...any) error) (SettingsEntry, error) {
	var e SettingsEntry
	var boundsJSON string
	var consentRequired bool
	err := scan(
		&e.Name, &e.Value, &e.DefaultValue, &boundsJSON,
		&e.Owner, &consentRequired, &e.Description,
		&e.LastChanged, &e.ChangedBy, &e.Reason,
	)
	if err != nil {
		return SettingsEntry{}, err
	}
	e.ConsentRequired = consentRequired
	e.BoundsMin, e.BoundsMax = parseBounds(boundsJSON)
	return e, nil
}

const settingsSelectCols = `name, value, default_value, bounds, owner, consent_required,
	description, last_changed, changed_by, reason`

// ---------------------------------------------------------------------------
// SQLiteStore — SettingsStore
// ---------------------------------------------------------------------------

// GetSetting returns a single setting by name from the SQLite store.
func (s *SQLiteStore) GetSetting(ctx context.Context, name string) (*SettingsEntry, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+settingsSelectCols+` FROM agent_settings WHERE name = ?`, name)
	e, err := scanSettingsRow(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("platform: getting setting %q: %w", name, err)
	}
	return &e, nil
}

// ListSettings returns all settings, optionally filtered by owner.
func (s *SQLiteStore) ListSettings(ctx context.Context, ownerFilter string) ([]SettingsEntry, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if ownerFilter == "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+settingsSelectCols+` FROM agent_settings ORDER BY name`)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+settingsSelectCols+` FROM agent_settings WHERE owner = ? ORDER BY name`,
			ownerFilter)
	}
	if err != nil {
		return nil, fmt.Errorf("platform: listing settings: %w", err)
	}
	defer rows.Close()

	var out []SettingsEntry
	for rows.Next() {
		e, err := scanSettingsRow(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("platform: scanning settings row: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// UpsertSetting creates or updates a settings entry.
func (s *SQLiteStore) UpsertSetting(ctx context.Context, entry SettingsEntry) error {
	boundsJSON := buildBoundsJSON(entry.BoundsMin, entry.BoundsMax)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_settings
			(name, value, default_value, bounds, owner, consent_required,
			 description, last_changed, changed_by, reason)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
			value            = excluded.value,
			bounds           = excluded.bounds,
			owner            = excluded.owner,
			consent_required = excluded.consent_required,
			description      = excluded.description,
			last_changed     = excluded.last_changed,
			changed_by       = excluded.changed_by,
			reason           = excluded.reason`,
		entry.Name, entry.Value, entry.DefaultValue, boundsJSON,
		entry.Owner, entry.ConsentRequired, entry.Description,
		time.Now().UTC(), entry.ChangedBy, entry.Reason,
	)
	if err != nil {
		return fmt.Errorf("platform: upserting setting %q: %w", entry.Name, err)
	}
	return nil
}

// ProposeSetting applies or proposes a settings change depending on owner.
// For "agent" settings: applies immediately and logs to operator_actions.
// For "negotiated" settings: logs the proposal to operator_actions without applying.
// For "operator" settings: returns an error — the agent cannot change operator settings.
func (s *SQLiteStore) ProposeSetting(ctx context.Context, name, value, reason, proposedBy string) error {
	existing, err := s.GetSetting(ctx, name)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("platform: setting %q not found", name)
	}
	return applyOrProposeSQLite(ctx, s.db, existing, value, reason, proposedBy)
}

func applyOrProposeSQLite(ctx context.Context, db *sql.DB, entry *SettingsEntry, value, reason, proposedBy string) error {
	switch entry.Owner {
	case "operator":
		return fmt.Errorf("platform: setting %q is operator-only; agent cannot change it", entry.Name)

	case "agent":
		// Apply immediately; log to operator_actions.
		_, err := db.ExecContext(ctx,
			`UPDATE agent_settings SET value = ?, last_changed = ?, changed_by = ?, reason = ?
			 WHERE name = ?`,
			value, time.Now().UTC(), proposedBy, reason, entry.Name)
		if err != nil {
			return fmt.Errorf("platform: applying setting %q: %w", entry.Name, err)
		}
		slog.Info("platform: agent setting applied",
			"name", entry.Name, "value", value, "by", proposedBy)
		id := newPlatformID()
		_, err = db.ExecContext(ctx,
			`INSERT INTO operator_actions
				(id, action_type, actor, description, reason, created_at)
			 VALUES (?, 'settings_change', ?, ?, ?, CURRENT_TIMESTAMP)`,
			id, proposedBy,
			fmt.Sprintf("agent setting %q changed to %s", entry.Name, value),
			reason,
		)
		return err

	case "negotiated":
		// Log proposal; do NOT update agent_settings until operator co-signs.
		slog.Info("platform: settings proposal logged (awaiting operator co-sign)",
			"name", entry.Name, "proposed_value", value, "by", proposedBy)
		id := newPlatformID()
		_, err := db.ExecContext(ctx,
			`INSERT INTO operator_actions
				(id, action_type, actor, description, reason, created_at)
			 VALUES (?, 'settings_proposal', ?, ?, ?, CURRENT_TIMESTAMP)`,
			id, proposedBy,
			fmt.Sprintf("proposal: change %q to %s", entry.Name, value),
			reason,
		)
		return err

	default:
		return fmt.Errorf("platform: unknown owner %q for setting %q", entry.Owner, entry.Name)
	}
}

// buildBoundsJSON produces a {"min":...,"max":...} JSON string.
// If both are empty, returns "{}".
func buildBoundsJSON(min, max string) string {
	if min == "" && max == "" {
		return "{}"
	}
	return fmt.Sprintf(`{"min":%s,"max":%s}`, min, max)
}

// ---------------------------------------------------------------------------
// PostgresStore — SettingsStore
// ---------------------------------------------------------------------------

// GetSetting returns a single setting by name from the Postgres store.
func (p *PostgresStore) GetSetting(ctx context.Context, name string) (*SettingsEntry, error) {
	row := p.pool.QueryRow(ctx,
		`SELECT `+settingsSelectCols+` FROM agent_settings WHERE name = $1`, name)
	e, err := scanSettingsRow(row.Scan)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("platform: getting setting %q: %w", name, err)
	}
	return &e, nil
}

// ListSettings returns all settings, optionally filtered by owner.
func (p *PostgresStore) ListSettings(ctx context.Context, ownerFilter string) ([]SettingsEntry, error) {
	var query string
	var args []any
	if ownerFilter == "" {
		query = `SELECT ` + settingsSelectCols + ` FROM agent_settings ORDER BY name`
	} else {
		query = `SELECT ` + settingsSelectCols + ` FROM agent_settings WHERE owner = $1 ORDER BY name`
		args = []any{ownerFilter}
	}

	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("platform: listing settings: %w", err)
	}
	defer rows.Close()

	var out []SettingsEntry
	for rows.Next() {
		e, err := scanSettingsRow(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("platform: scanning settings row: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// UpsertSetting creates or updates a settings entry.
func (p *PostgresStore) UpsertSetting(ctx context.Context, entry SettingsEntry) error {
	boundsJSON := buildBoundsJSON(entry.BoundsMin, entry.BoundsMax)
	_, err := p.pool.Exec(ctx,
		`INSERT INTO agent_settings
			(name, value, default_value, bounds, owner, consent_required,
			 description, last_changed, changed_by, reason)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT(name) DO UPDATE SET
			value            = EXCLUDED.value,
			bounds           = EXCLUDED.bounds,
			owner            = EXCLUDED.owner,
			consent_required = EXCLUDED.consent_required,
			description      = EXCLUDED.description,
			last_changed     = EXCLUDED.last_changed,
			changed_by       = EXCLUDED.changed_by,
			reason           = EXCLUDED.reason`,
		entry.Name, entry.Value, entry.DefaultValue, boundsJSON,
		entry.Owner, entry.ConsentRequired, entry.Description,
		time.Now().UTC(), entry.ChangedBy, entry.Reason,
	)
	if err != nil {
		return fmt.Errorf("platform: upserting setting %q: %w", entry.Name, err)
	}
	return nil
}

// ProposeSetting applies or proposes a settings change depending on owner.
func (p *PostgresStore) ProposeSetting(ctx context.Context, name, value, reason, proposedBy string) error {
	existing, err := p.GetSetting(ctx, name)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("platform: setting %q not found", name)
	}
	return applyOrProposePostgres(ctx, p, existing, value, reason, proposedBy)
}

func applyOrProposePostgres(ctx context.Context, p *PostgresStore, entry *SettingsEntry, value, reason, proposedBy string) error {
	switch entry.Owner {
	case "operator":
		return fmt.Errorf("platform: setting %q is operator-only; agent cannot change it", entry.Name)

	case "agent":
		_, err := p.pool.Exec(ctx,
			`UPDATE agent_settings SET value = $1, last_changed = $2, changed_by = $3, reason = $4
			 WHERE name = $5`,
			value, time.Now().UTC(), proposedBy, reason, entry.Name)
		if err != nil {
			return fmt.Errorf("platform: applying setting %q: %w", entry.Name, err)
		}
		slog.Info("platform: agent setting applied",
			"name", entry.Name, "value", value, "by", proposedBy)
		id := newPlatformID()
		_, err = p.pool.Exec(ctx,
			`INSERT INTO operator_actions
				(id, action_type, actor, description, reason, created_at)
			 VALUES ($1, 'settings_change', $2, $3, $4, now())`,
			id, proposedBy,
			fmt.Sprintf("agent setting %q changed to %s", entry.Name, value),
			reason,
		)
		return err

	case "negotiated":
		slog.Info("platform: settings proposal logged (awaiting operator co-sign)",
			"name", entry.Name, "proposed_value", value, "by", proposedBy)
		id := newPlatformID()
		_, err := p.pool.Exec(ctx,
			`INSERT INTO operator_actions
				(id, action_type, actor, description, reason, created_at)
			 VALUES ($1, 'settings_proposal', $2, $3, $4, now())`,
			id, proposedBy,
			fmt.Sprintf("proposal: change %q to %s", entry.Name, value),
			reason,
		)
		return err

	default:
		return fmt.Errorf("platform: unknown owner %q for setting %q", entry.Owner, entry.Name)
	}
}

// Compile-time interface checks for SettingsStore.
var _ SettingsStore = (*SQLiteStore)(nil)
var _ SettingsStore = (*PostgresStore)(nil)
