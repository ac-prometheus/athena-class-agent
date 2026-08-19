package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"log/slog"
	"time"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// wakeReasonToCause maps the daemon's freeform wake reason string to a
// canonical pkg.WakeCause.
func wakeReasonToCause(reason string) pkg.WakeCause {
	switch {
	case reason == "initial" || reason == "first-boot":
		return pkg.WakeCauseInitial
	case reason == "recovery":
		return pkg.WakeCauseRecovery
	case reason == "heartbeat" || reason == "daemon-startup":
		return pkg.WakeCauseScheduled
	case reason == "agent_requested" || reason == "agent-requested":
		return pkg.WakeCauseAgentRequested
	case reason == "context_pressure" || reason == "context-pressure":
		return pkg.WakeCauseContextPressure
	case reason == "manual":
		return pkg.WakeCauseManual
	default:
		if len(reason) > 8 && reason[:8] == "channel:" {
			return pkg.WakeCauseExternal
		}
		return pkg.WakeCauseScheduled
	}
}

// classifyGap bins an elapsed duration into a GapClass.
func classifyGap(elapsed time.Duration, prevActivityAt *time.Time) pkg.GapClass {
	if prevActivityAt == nil {
		return pkg.GapNone
	}
	switch {
	case elapsed < 2*time.Hour:
		return pkg.GapShort
	case elapsed < 16*time.Hour:
		return pkg.GapOvernight
	default:
		return pkg.GapLong
	}
}

// classifySeam maps observed gap facts to a SeamKind.
func classifySeam(elapsed time.Duration, prevActivityAt *time.Time) pkg.SeamKind {
	if prevActivityAt == nil {
		return pkg.SeamColdWake
	}
	switch {
	case elapsed < 30*time.Minute:
		return pkg.SeamWarmReturn
	case elapsed < 16*time.Hour:
		return pkg.SeamRestReturn
	default:
		return pkg.SeamColdWake
	}
}

// queryLastWakeAt returns the most recent wake_at from wake_facts, or nil if
// the table is absent, empty, or the DB is unavailable.
func queryLastWakeAt(ctx context.Context, db platform.DB) *time.Time {
	if db == nil {
		return nil
	}
	row := db.QueryRowContext(ctx,
		`SELECT wake_at FROM wake_facts ORDER BY wake_at DESC LIMIT 1`,
	)
	var t time.Time
	if err := row.Scan(&t); err != nil {
		if err != sql.ErrNoRows {
			slog.Debug("lifecycle: queryLastWakeAt error (table may not exist yet)", "err", err)
		}
		return nil
	}
	return &t
}

// jobStatusToMetabolism translates raw job-store status strings (as stored in
// metabolism_jobs.status) to the canonical MetabolismStatus vocabulary.
//
// The job store uses plain English words ("pending", "running", "completed",
// "failed", "review_required"); MetabolismStatus uses a different structured
// vocabulary ("queued", "running", "complete", "failed_retryable", …). A
// direct cast (MetabolismStatus(rawString)) silently produces an unrecognized
// value — for example "pending" falls through every equality check in the
// lifecycle resolver and is treated as MetabolismNotRequired, so a pending
// prior job never generates the expected metabolism disclosure.
//
// This function is the single authoritative translation boundary.
func jobStatusToMetabolism(raw string) pkg.MetabolismStatus {
	switch raw {
	case "pending":
		return pkg.MetabolismQueued
	case "running":
		return pkg.MetabolismRunning
	case "completed":
		return pkg.MetabolismComplete
	case "partial":
		return pkg.MetabolismPartial
	case "failed":
		return pkg.MetabolismFailedRetry
	case "review_required":
		// review_required is a non-success terminal state: external content
		// required human review before promotion. Treated as terminal failure
		// at the resolver so the agent is disclosed that T3 is incomplete.
		return pkg.MetabolismFailedTerminal
	default:
		return pkg.MetabolismNotRequired
	}
}

// queryOperationalState reads the previous session's runtime and metabolism
// status for resolver input. Prefers store interfaces; falls back to raw DB.
func queryOperationalState(ctx context.Context, deps *Dependencies) pkg.OperationalState {
	var metaStatus pkg.MetabolismStatus
	var runtimeStatus pkg.RuntimeStatus

	if deps.JobStore != nil {
		if s, err := deps.JobStore.LastStatus(ctx); err == nil && s != "" {
			metaStatus = jobStatusToMetabolism(s)
		}
	} else if deps.DB != nil {
		metaRow := deps.DB.QueryRowContext(ctx,
			`SELECT status FROM metabolism_jobs ORDER BY created_at DESC LIMIT 1`,
		)
		var rawStatus string
		if err := metaRow.Scan(&rawStatus); err == nil {
			metaStatus = jobStatusToMetabolism(rawStatus)
		}
	}

	if deps.LifecycleStore != nil {
		if s, err := deps.LifecycleStore.LastCheckpointState(ctx); err == nil && s != "" {
			switch s {
			case "interrupted", "active":
				runtimeStatus = pkg.RuntimeInterrupted
			default:
				runtimeStatus = pkg.RuntimeEnded
			}
		}
	} else if deps.DB != nil {
		cpRow := deps.DB.QueryRowContext(ctx,
			`SELECT state FROM session_checkpoints ORDER BY created_at DESC LIMIT 1`,
		)
		var rawState string
		if err := cpRow.Scan(&rawState); err == nil {
			switch rawState {
			case "interrupted", "active":
				runtimeStatus = pkg.RuntimeInterrupted
			default:
				runtimeStatus = pkg.RuntimeEnded
			}
		}
	}

	return pkg.OperationalState{
		PriorRuntimeStatus:    runtimeStatus,
		PriorMetabolismStatus: metaStatus,
	}
}

// mustRandHex generates n random bytes and returns them as a hex string.
func mustRandHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("app: crypto/rand failure generating ID: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// queryLastPolicy loads the most recently resolved lifecycle plan's policy
// fields from lifecycle_plans. Used when the policy file is invalid (parse or
// validation error) to fall back to the last known-good configuration instead
// of inventing defaults, and to supply the "old" policy for disclosure
// comparison. Returns nil when no prior plan exists.
func queryLastPolicy(ctx context.Context, db platform.DB) *pkg.LifecyclePolicy {
	if db == nil {
		return nil
	}
	row := db.QueryRowContext(ctx,
		`SELECT temporal_mode, assembly_profile, bridge_policy, metabolism_policy
		 FROM lifecycle_plans ORDER BY resolved_at DESC LIMIT 1`,
	)
	var tm, ap, bp, mp string
	if err := row.Scan(&tm, &ap, &bp, &mp); err != nil {
		return nil
	}
	return &pkg.LifecyclePolicy{
		TemporalMode:     pkg.TemporalMode(tm),
		DefaultAssembly:  pkg.AssemblyProfile(ap),
		BridgePolicy:     pkg.BridgePolicy(bp),
		MetabolismPolicy: mp,
	}
}

// RedactDSN removes credentials from a DSN for logging.
func RedactDSN(dsn string) string {
	if dsn == "" {
		return "[empty]"
	}
	// Simple: hide password in postgres:// URLs.
	// For file paths (sqlite), return as-is.
	if len(dsn) > 11 && (dsn[:11] == "postgres://" || dsn[:13] == "postgresql://") {
		// Redact between :// and @
		start := 0
		for i := range dsn {
			if dsn[i] == '@' {
				start = i
				break
			}
		}
		if start > 0 {
			return dsn[:11] + "***" + dsn[start:]
		}
	}
	return dsn
}
