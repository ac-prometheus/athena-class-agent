package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// SQLiteLifecycleStore implements pkg.LifecycleStore for SQLite.
type SQLiteLifecycleStore struct {
	db platform.DB
}

// NewSQLiteLifecycleStore returns a LifecycleStore backed by SQLite.
func NewSQLiteLifecycleStore(db platform.DB) *SQLiteLifecycleStore {
	return &SQLiteLifecycleStore{db: db}
}

func (s *SQLiteLifecycleStore) RecordPlan(ctx context.Context, plan *pkg.LifecyclePlan) error {
	if plan == nil {
		return fmt.Errorf("storage: lifecycle plan is nil")
	}
	if plan.ID == "" {
		plan.ID = newID()
	}

	var secondaryCause *string
	if plan.WakeCauseSecondary != nil {
		sc := string(*plan.WakeCauseSecondary)
		secondaryCause = &sc
	}

	reasons, _ := json.Marshal(plan.Reasons)
	disclosures, _ := json.Marshal(plan.Disclosures)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO lifecycle_plans
			(id, session_id, temporal_mode, wake_cause, wake_cause_secondary,
			 activity_profile, assembly_profile, bridge_policy, metabolism_policy,
			 seam_kind, resolver_version, policy_hash, resolved_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		plan.ID, plan.SessionID,
		string(plan.TemporalMode), string(plan.WakeCause), secondaryCause,
		string(plan.ActivityProfile), string(plan.AssemblyProfile),
		string(plan.BridgePolicy), plan.MetabolismPolicy,
		string(plan.SeamKind), plan.ResolverVersion, plan.PolicyHash,
	)
	if err != nil {
		return fmt.Errorf("storage: recording lifecycle plan %s: %w", plan.ID, err)
	}

	_ = reasons
	_ = disclosures

	return nil
}

func (s *SQLiteLifecycleStore) RecordWakeFacts(ctx context.Context, sessionID string, facts *pkg.WakeFacts) error {
	if facts == nil {
		return fmt.Errorf("storage: wake facts are nil")
	}

	planRow := s.db.QueryRowContext(ctx,
		`SELECT id FROM lifecycle_plans WHERE session_id = ? ORDER BY resolved_at DESC LIMIT 1`,
		sessionID,
	)
	var planID string
	if err := planRow.Scan(&planID); err != nil {
		return fmt.Errorf("storage: no lifecycle plan found for session %s: %w", sessionID, err)
	}

	var gapClass *string
	if facts.GapFacts.GapClass != "" {
		gc := string(facts.GapFacts.GapClass)
		gapClass = &gc
	}

	var transitionCtx *string
	if len(facts.TransitionContexts) > 0 {
		tc := string(facts.TransitionContexts[0])
		transitionCtx = &tc
	}

	extraJSON, _ := json.Marshal(facts)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO wake_facts
			(plan_id, wake_at, elapsed_duration_sec, gap_class, transition_context, extra_json)
		 VALUES (?, CURRENT_TIMESTAMP, ?, ?, ?, ?)`,
		planID, facts.ElapsedDuration.Seconds(), gapClass, transitionCtx, string(extraJSON),
	)
	if err != nil {
		return fmt.Errorf("storage: recording wake facts for session %s: %w", sessionID, err)
	}
	return nil
}

func (s *SQLiteLifecycleStore) RecordManifest(ctx context.Context, manifest *pkg.AssemblyManifest) error {
	if manifest == nil {
		return fmt.Errorf("storage: assembly manifest is nil")
	}
	if manifest.ID == "" {
		manifest.ID = newID()
	}

	phasesRun, _ := json.Marshal(manifest.PhasesRun)
	phasesSkipped, _ := json.Marshal(manifest.PhasesSkipped)
	skipReasons, _ := json.Marshal(manifest.SkipReasons)

	utilization := float64(0)
	if manifest.BudgetTotal > 0 {
		utilization = float64(manifest.BudgetUsed) / float64(manifest.BudgetTotal)
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO assembly_manifests
			(id, session_id, plan_id, phases_loaded, phases_skipped,
			 known_omissions, total_tokens_used, total_tokens_budget,
			 context_utilization, assembled_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		manifest.ID, manifest.SessionID, manifest.PlanID,
		string(phasesRun), string(phasesSkipped), string(skipReasons),
		manifest.BudgetUsed, manifest.BudgetTotal, utilization,
	)
	if err != nil {
		return fmt.Errorf("storage: recording assembly manifest %s: %w", manifest.ID, err)
	}
	return nil
}

func (s *SQLiteLifecycleStore) RecordDisclosure(ctx context.Context, sessionID, policyPath, policyHash, previousHash, changesSummary string) error {
	var prevHash interface{}
	if previousHash != "" {
		prevHash = previousHash
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO configuration_applied
			(session_id, policy_path, policy_hash, previous_hash, changes_summary, applied_at)
		 VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		sessionID, policyPath, policyHash, prevHash, changesSummary,
	)
	if err != nil {
		return fmt.Errorf("storage: recording disclosure for session %s: %w", sessionID, err)
	}
	return nil
}

func (s *SQLiteLifecycleStore) LastPolicyHash(ctx context.Context, _ string) (string, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT policy_hash FROM configuration_applied ORDER BY applied_at DESC LIMIT 1`)
	var hash string
	if err := row.Scan(&hash); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("storage: querying last policy hash: %w", err)
	}
	return hash, nil
}

func (s *SQLiteLifecycleStore) WriteCheckpoint(ctx context.Context, sessionID string, turnNumber int, t2HighWater string, tokenUsage int, state string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO session_checkpoints (id, session_id, turn_number, t2_high_water, token_usage, state, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT (id) DO UPDATE
		   SET turn_number   = excluded.turn_number,
		       t2_high_water = excluded.t2_high_water,
		       token_usage   = excluded.token_usage,
		       state         = excluded.state,
		       created_at    = CURRENT_TIMESTAMP`,
		sessionID, sessionID, turnNumber, t2HighWater, tokenUsage, state,
	)
	if err != nil {
		return fmt.Errorf("storage: write checkpoint for session %s: %w", sessionID, err)
	}
	return nil
}

func (s *SQLiteLifecycleStore) InterruptStaleCheckpoints(ctx context.Context, cutoff time.Time) (int, error) {
	result, err := s.db.ExecContext(ctx,
		`UPDATE session_checkpoints
		 SET state = 'interrupted'
		 WHERE state = 'active' AND created_at < ?`,
		cutoff.UTC().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return 0, fmt.Errorf("storage: interrupting stale checkpoints: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("storage: checking interrupted count: %w", err)
	}
	return int(n), nil
}

func (s *SQLiteLifecycleStore) LastCheckpointState(ctx context.Context) (string, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT state FROM session_checkpoints ORDER BY created_at DESC LIMIT 1`)
	var state string
	if err := row.Scan(&state); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("storage: querying last checkpoint state: %w", err)
	}
	return state, nil
}

func (s *SQLiteLifecycleStore) LastWakeAt(ctx context.Context) (time.Time, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT wake_at FROM wake_facts ORDER BY wake_at DESC LIMIT 1`)
	var wakeAt time.Time
	if err := row.Scan(&wakeAt); err != nil {
		if err == sql.ErrNoRows {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("storage: querying last wake time: %w", err)
	}
	return wakeAt, nil
}
