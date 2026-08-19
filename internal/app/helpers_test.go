package app

import (
	"context"
	"strings"
	"testing"

	"github.com/ac-prometheus/athena-class-agent/internal/assembly"
	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/internal/platform/storage"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// ---------------------------------------------------------------------------
// Unit tests for jobStatusToMetabolism — the vocabulary translation boundary
// ---------------------------------------------------------------------------

func TestJobStatusToMetabolism_AllMappings(t *testing.T) {
	cases := []struct {
		raw  string
		want pkg.MetabolismStatus
	}{
		{"pending", pkg.MetabolismQueued},
		{"running", pkg.MetabolismRunning},
		{"completed", pkg.MetabolismComplete},
		{"partial", pkg.MetabolismPartial},
		{"failed", pkg.MetabolismFailedRetry},
		{"review_required", pkg.MetabolismFailedTerminal},
		{"", pkg.MetabolismNotRequired},
		{"unknown_value", pkg.MetabolismNotRequired},
	}
	for _, tc := range cases {
		got := jobStatusToMetabolism(tc.raw)
		if got != tc.want {
			t.Errorf("jobStatusToMetabolism(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Integration test: pending job row → queryOperationalState → disclosure
//
// Before the fix, pkg.MetabolismStatus("pending") fell through all equality
// checks in phase_continuity (which compares against "queued", "running",
// etc.) and produced no disclosure. After the fix, queryOperationalState
// translates "pending" → MetabolismQueued, which is the value the continuity
// phase tests for isPending.
// ---------------------------------------------------------------------------

func TestQueryOperationalState_PendingJob_MapsToQueued(t *testing.T) {
	_, pdb := acceptanceDB(t)
	deps := acceptanceDeps(t, pdb)
	ctx := context.Background()

	// Insert a pending metabolism job — the state left by a prior session that
	// enqueued work but whose process restarted before metabolism ran.
	if _, err := deps.DB.ExecContext(ctx,
		`INSERT INTO metabolism_jobs (id, session_id, status, job_type, created_at)
		 VALUES ('pending-prior-job', 'session-prior', 'pending', 'standard', CURRENT_TIMESTAMP)`,
	); err != nil {
		t.Fatalf("insert pending job: %v", err)
	}

	state := queryOperationalState(ctx, deps)

	if state.PriorMetabolismStatus != pkg.MetabolismQueued {
		t.Errorf("PriorMetabolismStatus = %q, want %q (MetabolismQueued); "+
			"raw 'pending' must be translated, not cast directly",
			state.PriorMetabolismStatus, pkg.MetabolismQueued)
	}
}

// TestQueryOperationalState_PendingJob_ProducesDisclosure is the end-to-end
// path: pending DB row → queryOperationalState → lifecycle plan → assembly.
// The continuity phase must inject a [Metabolism Note] block when the prior
// metabolism status is queued (formerly: this was silently dropped because
// MetabolismStatus("pending") ≠ MetabolismQueued).
func TestQueryOperationalState_PendingJob_ProducesDisclosure(t *testing.T) {
	rawDB, pdb := acceptanceDB(t)
	deps := acceptanceDeps(t, pdb)
	ctx := context.Background()

	// Seed a pending job from a prior session.
	if _, err := deps.DB.ExecContext(ctx,
		`INSERT INTO metabolism_jobs (id, session_id, status, job_type, created_at)
		 VALUES ('pending-prior-job2', 'session-prior2', 'pending', 'standard', CURRENT_TIMESTAMP)`,
	); err != nil {
		t.Fatalf("insert pending job: %v", err)
	}

	// Resolve operational state — must produce MetabolismQueued.
	opState := queryOperationalState(ctx, deps)
	if opState.PriorMetabolismStatus != pkg.MetabolismQueued {
		t.Fatalf("prerequisite failed: PriorMetabolismStatus = %q, want %q",
			opState.PriorMetabolismStatus, pkg.MetabolismQueued)
	}

	// Wire the plan with the resolved status so assembly sees it.
	plan := &pkg.LifecyclePlan{
		ID:                    "plan-disclosure-test",
		SessionID:             "session-disclosure",
		PriorMetabolismStatus: opState.PriorMetabolismStatus,
		TemporalMode:          pkg.TemporalEpisodic,
		WakeCause:             pkg.WakeCauseScheduled,
		BridgePolicy:          pkg.BridgeDisabled,
	}

	// Provide a memory store so the continuity phase actually runs instead of
	// short-circuiting on nil store. The store is empty (no T3/T4 data) which
	// is fine: the metabolism note is appended before narrative retrieval.
	memStore := platform.NewSQLiteStoreFromDB(rawDB, "sqlite3://:memory:")

	assembler := assembly.NewContextAssembler(deps.Config.IdentityDir, deps.Config.TokenBudget)
	cfg := assembly.MinimalAssembleConfig()
	cfg.Plan = plan
	cfg.SessionID = plan.SessionID
	cfg.SkipWitnessCheck = true
	cfg.SetStore(memStore)

	result, err := assembler.Assemble(ctx, cfg)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	// The assembled system prompt must contain the metabolism disclosure injected
	// by phase_continuity when isPending is true.
	if !strings.Contains(result.SystemPrompt, "Metabolism Note") {
		t.Errorf("assembled system prompt does not contain [Metabolism Note]; "+
			"pending prior job should trigger disclosure.\nSystemPrompt (first 2000 chars):\n%s",
			truncateStr(result.SystemPrompt, 2000))
	}
}

// Verify that the JobStore path (not raw DB) also translates correctly.
func TestQueryOperationalState_ViaJobStore_PendingMapsToQueued(t *testing.T) {
	_, pdb := acceptanceDB(t)
	ctx := context.Background()

	jobStore := storage.NewSQLiteJobStore(pdb)

	// Commit creates a pending job.
	if _, err := jobStore.Commit(ctx, "session-via-store", "standard"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	deps := &Dependencies{
		DB:       pdb,
		JobStore: jobStore,
	}

	state := queryOperationalState(ctx, deps)
	if state.PriorMetabolismStatus != pkg.MetabolismQueued {
		t.Errorf("via JobStore: PriorMetabolismStatus = %q, want %q",
			state.PriorMetabolismStatus, pkg.MetabolismQueued)
	}
}
