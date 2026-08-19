package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/pkg"
	_ "github.com/mattn/go-sqlite3"
)

// schemaDir is the path to migration files.
const schemaDir = "/opt/athena-class-agent/schema"

// setupTestDB opens an in-memory SQLite database and runs all migrations.
func setupTestDB(t *testing.T) (platform.DB, *sql.DB) {
	t.Helper()
	raw, err := sql.Open("sqlite3", "file::memory:?_loc=UTC")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { raw.Close() })

	// Enable WAL and foreign keys.
	if _, err := raw.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("pragma wal: %v", err)
	}
	if _, err := raw.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("pragma fk: %v", err)
	}

	files, err := collectMigrations(schemaDir)
	if err != nil {
		t.Fatalf("collect migrations: %v", err)
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		adapted := adaptForSQLite(string(data))
		for _, stmt := range splitStatements(adapted) {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err := raw.Exec(stmt); err != nil {
				t.Fatalf("exec migration %s stmt %.80s: %v", filepath.Base(f), stmt, err)
			}
		}
	}

	return platform.WrapSQLDB(raw), raw
}

func collectMigrations(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	sort.Strings(files)
	return files, nil
}

var (
	reVectorType      = regexp.MustCompile(`(?i)\bvector\(\d+\)`)
	reTimestamptz     = regexp.MustCompile(`(?i)\bTIMESTAMPTZ\b`)
	reNow             = regexp.MustCompile(`(?i)\bnow\(\)`)
	reConcurrently    = regexp.MustCompile(`(?i)CREATE\s+INDEX\s+CONCURRENTLY`)
	reJSONB           = regexp.MustCompile(`(?i)\bJSONB\b`)
	reBooleanType     = regexp.MustCompile(`(?i)\bBOOLEAN\b`)
	reTrue            = regexp.MustCompile(`(?i)\bTRUE\b`)
	reFalse           = regexp.MustCompile(`(?i)\bFALSE\b`)
	reBigSerial       = regexp.MustCompile(`(?i)\bBIGSERIAL\b`)
	reSerialOnly      = regexp.MustCompile(`(?i)\bSERIAL\b`)
	reAddColIfNotExist = regexp.MustCompile(`(?i)\bADD\s+COLUMN\s+IF\s+NOT\s+EXISTS\b`)
)

func adaptForSQLite(sql string) string {
	sql = reBigSerial.ReplaceAllString(sql, "INTEGER PRIMARY KEY AUTOINCREMENT")
	sql = reSerialOnly.ReplaceAllString(sql, "INTEGER PRIMARY KEY AUTOINCREMENT")
	sql = reVectorType.ReplaceAllString(sql, "TEXT")
	sql = reTimestamptz.ReplaceAllString(sql, "TIMESTAMP")
	sql = reNow.ReplaceAllString(sql, "CURRENT_TIMESTAMP")
	sql = reConcurrently.ReplaceAllString(sql, "CREATE INDEX")
	sql = reJSONB.ReplaceAllString(sql, "TEXT")
	sql = reBooleanType.ReplaceAllString(sql, "INTEGER")
	sql = reTrue.ReplaceAllString(sql, "1")
	sql = reFalse.ReplaceAllString(sql, "0")
	sql = reAddColIfNotExist.ReplaceAllString(sql, "ADD COLUMN")
	return sql
}

func splitStatements(sql string) []string {
	var stmts []string
	var cur strings.Builder
	inString := false
	inLineComment := false

	for i := 0; i < len(sql); i++ {
		ch := sql[i]
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			cur.WriteByte(ch)
			continue
		}
		if !inString && i+1 < len(sql) && ch == '-' && sql[i+1] == '-' {
			inLineComment = true
			cur.WriteByte(ch)
			continue
		}
		if ch == '\'' {
			inString = !inString
		}
		if !inString && ch == ';' {
			stmts = append(stmts, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(ch)
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		stmts = append(stmts, s)
	}
	return stmts
}

// ---------------------------------------------------------------------------
// MetabolismJobStore
// ---------------------------------------------------------------------------

func TestJobStore_CommitClaimComplete(t *testing.T) {
	db, raw := setupTestDB(t)
	ctx := context.Background()
	store := NewSQLiteJobStore(db)

	jobID, err := store.Commit(ctx, "sess-1", "standard")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if jobID == "" {
		t.Fatal("Commit returned empty job ID")
	}

	// Verify pending.
	var status string
	if err := raw.QueryRow("SELECT status FROM metabolism_jobs WHERE id = ?", jobID).Scan(&status); err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != "pending" {
		t.Errorf("status after Commit = %q, want pending", status)
	}

	// Claim.
	claimToken, err := store.Claim(ctx, jobID, 5*time.Minute)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if claimToken == "" {
		t.Fatal("Claim returned empty token")
	}
	if err := raw.QueryRow("SELECT status FROM metabolism_jobs WHERE id = ?", jobID).Scan(&status); err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != "running" {
		t.Errorf("status after Claim = %q, want running", status)
	}

	// Complete.
	if err := store.Complete(ctx, jobID, claimToken); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if err := raw.QueryRow("SELECT status FROM metabolism_jobs WHERE id = ?", jobID).Scan(&status); err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != "completed" {
		t.Errorf("status after Complete = %q, want completed", status)
	}
}

func TestJobStore_CommitFail(t *testing.T) {
	db, raw := setupTestDB(t)
	ctx := context.Background()
	store := NewSQLiteJobStore(db)

	jobID, err := store.Commit(ctx, "sess-fail", "standard")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	claimToken, err := store.Claim(ctx, jobID, 5*time.Minute)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}

	if err := store.Fail(ctx, jobID, claimToken, "LLM timeout"); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	var status, errMsg string
	var retryCount int
	if err := raw.QueryRow(
		"SELECT status, error_message, retry_count FROM metabolism_jobs WHERE id = ?", jobID,
	).Scan(&status, &errMsg, &retryCount); err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "failed" {
		t.Errorf("status = %q, want failed", status)
	}
	if errMsg != "LLM timeout" {
		t.Errorf("error_message = %q, want 'LLM timeout'", errMsg)
	}
	if retryCount != 1 {
		t.Errorf("retry_count = %d, want 1", retryCount)
	}
}

func TestJobStore_MarkReviewRequired(t *testing.T) {
	db, raw := setupTestDB(t)
	ctx := context.Background()
	store := NewSQLiteJobStore(db)

	jobID, err := store.Commit(ctx, "sess-review", "standard")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	claimToken, err := store.Claim(ctx, jobID, 5*time.Minute)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}

	if err := store.MarkReviewRequired(ctx, jobID, claimToken, "unannotated external content"); err != nil {
		t.Fatalf("MarkReviewRequired: %v", err)
	}

	var status, errMsg string
	if err := raw.QueryRow(
		"SELECT status, error_message FROM metabolism_jobs WHERE id = ?", jobID,
	).Scan(&status, &errMsg); err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "review_required" {
		t.Errorf("status = %q, want review_required", status)
	}
	if errMsg != "unannotated external content" {
		t.Errorf("error_message = %q, want 'unannotated external content'", errMsg)
	}
}

func TestJobStore_DoubleClaimReturnsErrJobNotPending(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()
	store := NewSQLiteJobStore(db)

	jobID, err := store.Commit(ctx, "sess-double", "standard")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if _, err := store.Claim(ctx, jobID, 5*time.Minute); err != nil {
		t.Fatalf("first Claim: %v", err)
	}

	_, err = store.Claim(ctx, jobID, 5*time.Minute)
	if !errors.Is(err, pkg.ErrJobNotPending) {
		t.Errorf("second Claim = %v, want ErrJobNotPending", err)
	}
}

func TestJobStore_Recoverable(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()
	store := NewSQLiteJobStore(db)

	// Job 1: pending.
	id1, err := store.Commit(ctx, "sess-a", "standard")
	if err != nil {
		t.Fatalf("Commit 1: %v", err)
	}

	// Job 2: running (pending then claimed).
	id2, err := store.Commit(ctx, "sess-b", "standard")
	if err != nil {
		t.Fatalf("Commit 2: %v", err)
	}
	if _, err := store.Claim(ctx, id2, 5*time.Minute); err != nil {
		t.Fatalf("Claim 2: %v", err)
	}

	// Job 3: completed — should NOT appear.
	id3, err := store.Commit(ctx, "sess-c", "standard")
	if err != nil {
		t.Fatalf("Commit 3: %v", err)
	}
	token3, err := store.Claim(ctx, id3, 5*time.Minute)
	if err != nil {
		t.Fatalf("Claim 3: %v", err)
	}
	if err := store.Complete(ctx, id3, token3); err != nil {
		t.Fatalf("Complete 3: %v", err)
	}

	jobs, err := store.Recoverable(ctx, 3)
	if err != nil {
		t.Fatalf("Recoverable: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("Recoverable returned %d jobs, want 2", len(jobs))
	}

	ids := map[string]bool{jobs[0].JobID: true, jobs[1].JobID: true}
	if !ids[id1] {
		t.Errorf("expected job %s (pending) in recoverable set", id1)
	}
	if !ids[id2] {
		t.Errorf("expected job %s (running) in recoverable set", id2)
	}
}

func TestJobStore_LastStatus(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()
	store := NewSQLiteJobStore(db)

	// Empty — should return "".
	s, err := store.LastStatus(ctx)
	if err != nil {
		t.Fatalf("LastStatus empty: %v", err)
	}
	if s != "" {
		t.Errorf("LastStatus empty = %q, want empty", s)
	}

	// Commit → pending.
	if _, err := store.Commit(ctx, "sess-ls", "standard"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	s, err = store.LastStatus(ctx)
	if err != nil {
		t.Fatalf("LastStatus: %v", err)
	}
	if s != "pending" {
		t.Errorf("LastStatus = %q, want pending", s)
	}
}

// ---------------------------------------------------------------------------
// ConsolidationStore
// ---------------------------------------------------------------------------

func insertT2Log(t *testing.T, raw *sql.DB, id, sessionID, content, source string) {
	t.Helper()
	_, err := raw.Exec(
		`INSERT INTO experiential_logs (id, session_id, content, content_source, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		id, sessionID, content, source, time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("insert T2 log %s: %v", id, err)
	}
}

func TestConsolidationStore_CommitNarrative(t *testing.T) {
	db, raw := setupTestDB(t)
	ctx := context.Background()
	store := NewSQLiteConsolidationStore(db)

	sessionID := "sess-consol"
	insertT2Log(t, raw, "log-1", sessionID, "first entry", "self")
	insertT2Log(t, raw, "log-2", sessionID, "second entry", "self")

	narrative := &pkg.NarrativeSummary{
		ID:        "narr-1",
		SessionID: sessionID,
		Content:   "compressed narrative",
		Belief: &pkg.BeliefMeta{
			BaseConfidence:    0.85,
			InferenceDistance:  1,
			VerificationState: "unverified",
			Source:            "compression",
			AnchorAt:          time.Now().UTC(),
		},
	}

	if err := store.CommitNarrative(ctx, narrative, []string{"log-1", "log-2"}); err != nil {
		t.Fatalf("CommitNarrative: %v", err)
	}

	// Verify T3 row exists.
	var content string
	if err := raw.QueryRow("SELECT content FROM narrative_summaries WHERE id = ?", "narr-1").Scan(&content); err != nil {
		t.Fatalf("query narrative: %v", err)
	}
	if content != "compressed narrative" {
		t.Errorf("narrative content = %q", content)
	}

	// Verify T2 back-links.
	for _, logID := range []string{"log-1", "log-2"} {
		var narrID sql.NullString
		if err := raw.QueryRow(
			"SELECT narrative_summary_id FROM experiential_logs WHERE id = ?", logID,
		).Scan(&narrID); err != nil {
			t.Fatalf("query backlink %s: %v", logID, err)
		}
		if !narrID.Valid || narrID.String != "narr-1" {
			t.Errorf("log %s narrative_summary_id = %v, want narr-1", logID, narrID)
		}
	}
}

func TestConsolidationStore_UncoveredLogs(t *testing.T) {
	db, raw := setupTestDB(t)
	ctx := context.Background()
	store := NewSQLiteConsolidationStore(db)

	sessionID := "sess-uncov"
	insertT2Log(t, raw, "log-a", sessionID, "uncovered", "self")
	insertT2Log(t, raw, "log-b", sessionID, "covered", "self")

	// Insert a narrative so FK is satisfied, then mark log-b as covered.
	_, err := raw.Exec(
		`INSERT INTO narrative_summaries (id, session_id, content, created_at)
		 VALUES ('narr-x', ?, 'placeholder', CURRENT_TIMESTAMP)`, sessionID,
	)
	if err != nil {
		t.Fatalf("insert narrative: %v", err)
	}
	if _, err := raw.Exec(
		"UPDATE experiential_logs SET narrative_summary_id = 'narr-x' WHERE id = 'log-b'",
	); err != nil {
		t.Fatalf("update: %v", err)
	}

	logs, err := store.UncoveredLogs(ctx, sessionID)
	if err != nil {
		t.Fatalf("UncoveredLogs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("UncoveredLogs returned %d, want 1", len(logs))
	}
	if logs[0].ID != "log-a" {
		t.Errorf("uncovered log ID = %q, want log-a", logs[0].ID)
	}
}

func TestConsolidationStore_NilNarrative(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()
	store := NewSQLiteConsolidationStore(db)

	err := store.CommitNarrative(ctx, nil, []string{"log-1"})
	if err == nil {
		t.Fatal("expected error for nil narrative")
	}
}

func TestConsolidationStore_EmptySourceLogIDs(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()
	store := NewSQLiteConsolidationStore(db)

	narrative := &pkg.NarrativeSummary{
		ID:        "narr-empty",
		SessionID: "sess-empty",
		Content:   "should fail",
	}
	err := store.CommitNarrative(ctx, narrative, []string{})
	if err == nil {
		t.Fatal("expected error for empty source log IDs")
	}
}

// ---------------------------------------------------------------------------
// LifecycleStore
// ---------------------------------------------------------------------------

func TestLifecycleStore_RecordPlan(t *testing.T) {
	db, raw := setupTestDB(t)
	ctx := context.Background()
	store := NewSQLiteLifecycleStore(db)

	plan := &pkg.LifecyclePlan{
		ID:               "plan-1",
		SessionID:        "sess-plan",
		TemporalMode:     pkg.TemporalEpisodic,
		WakeCause:        pkg.WakeCauseScheduled,
		ActivityProfile:  pkg.ActivityNormal,
		AssemblyProfile:  pkg.AssemblyFull,
		BridgePolicy:     pkg.BridgeAgentRequested,
		MetabolismPolicy: "standard",
		SeamKind:         pkg.SeamColdWake,
		ResolverVersion:  "v1",
		PolicyHash:       "abc123",
	}

	if err := store.RecordPlan(ctx, plan); err != nil {
		t.Fatalf("RecordPlan: %v", err)
	}

	var sessionID, tm string
	if err := raw.QueryRow(
		"SELECT session_id, temporal_mode FROM lifecycle_plans WHERE id = ?", "plan-1",
	).Scan(&sessionID, &tm); err != nil {
		t.Fatalf("query plan: %v", err)
	}
	if sessionID != "sess-plan" {
		t.Errorf("session_id = %q", sessionID)
	}
	if tm != "episodic" {
		t.Errorf("temporal_mode = %q", tm)
	}
}

func TestLifecycleStore_RecordWakeFacts(t *testing.T) {
	db, raw := setupTestDB(t)
	ctx := context.Background()
	store := NewSQLiteLifecycleStore(db)

	plan := &pkg.LifecyclePlan{
		ID:               "plan-wf",
		SessionID:        "sess-wf",
		TemporalMode:     pkg.TemporalEpisodic,
		WakeCause:        pkg.WakeCauseScheduled,
		ActivityProfile:  pkg.ActivityNormal,
		AssemblyProfile:  pkg.AssemblyFull,
		BridgePolicy:     pkg.BridgeAgentRequested,
		MetabolismPolicy: "standard",
		SeamKind:         pkg.SeamColdWake,
		ResolverVersion:  "v1",
	}
	if err := store.RecordPlan(ctx, plan); err != nil {
		t.Fatalf("RecordPlan: %v", err)
	}

	facts := &pkg.WakeFacts{
		PrimaryCause:    pkg.WakeCauseScheduled,
		ElapsedDuration: 2 * time.Hour,
		GapFacts: pkg.GapFacts{
			WakeAt:          time.Now(),
			ElapsedDuration: 2 * time.Hour,
			ClockBasis:      "wall",
			GapClass:        pkg.GapShort,
		},
	}
	if err := store.RecordWakeFacts(ctx, "sess-wf", facts); err != nil {
		t.Fatalf("RecordWakeFacts: %v", err)
	}

	var planID string
	if err := raw.QueryRow("SELECT plan_id FROM wake_facts ORDER BY rowid DESC LIMIT 1").Scan(&planID); err != nil {
		t.Fatalf("query wake_facts: %v", err)
	}
	if planID != "plan-wf" {
		t.Errorf("plan_id = %q, want plan-wf", planID)
	}
}

func TestLifecycleStore_RecordManifest(t *testing.T) {
	db, raw := setupTestDB(t)
	ctx := context.Background()
	store := NewSQLiteLifecycleStore(db)

	// Need a plan first (foreign key).
	plan := &pkg.LifecyclePlan{
		ID:               "plan-man",
		SessionID:        "sess-man",
		TemporalMode:     pkg.TemporalEpisodic,
		WakeCause:        pkg.WakeCauseScheduled,
		ActivityProfile:  pkg.ActivityNormal,
		AssemblyProfile:  pkg.AssemblyFull,
		BridgePolicy:     pkg.BridgeAgentRequested,
		MetabolismPolicy: "standard",
		SeamKind:         pkg.SeamColdWake,
		ResolverVersion:  "v1",
	}
	if err := store.RecordPlan(ctx, plan); err != nil {
		t.Fatalf("RecordPlan: %v", err)
	}

	manifest := &pkg.AssemblyManifest{
		ID:            "man-1",
		SessionID:     "sess-man",
		PlanID:        "plan-man",
		PhasesRun:     []string{"identity", "continuity"},
		PhasesSkipped: []string{"grounding"},
		SkipReasons:   map[string]string{"grounding": "budget pressure"},
		BudgetTotal:   640000,
		BudgetUsed:    320000,
	}
	if err := store.RecordManifest(ctx, manifest); err != nil {
		t.Fatalf("RecordManifest: %v", err)
	}

	var sessID string
	var tokUsed int
	if err := raw.QueryRow(
		"SELECT session_id, total_tokens_used FROM assembly_manifests WHERE id = ?", "man-1",
	).Scan(&sessID, &tokUsed); err != nil {
		t.Fatalf("query manifest: %v", err)
	}
	if sessID != "sess-man" {
		t.Errorf("session_id = %q", sessID)
	}
	if tokUsed != 320000 {
		t.Errorf("total_tokens_used = %d, want 320000", tokUsed)
	}
}

func TestLifecycleStore_RecordDisclosureAndLastPolicyHash(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()
	store := NewSQLiteLifecycleStore(db)

	if err := store.RecordDisclosure(ctx, "sess-disc", "/workspace/lifecycle.json", "hash-new", "hash-old", "temporal_mode changed", "workspace:/workspace/lifecycle.json", `{"temporal_mode":"episodic"}`); err != nil {
		t.Fatalf("RecordDisclosure: %v", err)
	}

	hash, err := store.LastPolicyHash(ctx, "")
	if err != nil {
		t.Fatalf("LastPolicyHash: %v", err)
	}
	if hash != "hash-new" {
		t.Errorf("LastPolicyHash = %q, want hash-new", hash)
	}
}

func TestLifecycleStore_LastPolicyHash_Empty(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()
	store := NewSQLiteLifecycleStore(db)

	hash, err := store.LastPolicyHash(ctx, "")
	if err != nil {
		t.Fatalf("LastPolicyHash: %v", err)
	}
	if hash != "" {
		t.Errorf("LastPolicyHash empty DB = %q, want empty", hash)
	}
}

func TestLifecycleStore_WriteCheckpointAndLastState(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()
	store := NewSQLiteLifecycleStore(db)

	if err := store.WriteCheckpoint(ctx, "sess-cp", 5, "log-high", 50000, "active"); err != nil {
		t.Fatalf("WriteCheckpoint: %v", err)
	}

	state, err := store.LastCheckpointState(ctx)
	if err != nil {
		t.Fatalf("LastCheckpointState: %v", err)
	}
	if state != "active" {
		t.Errorf("LastCheckpointState = %q, want active", state)
	}
}

func TestLifecycleStore_InterruptStaleCheckpoints(t *testing.T) {
	db, raw := setupTestDB(t)
	ctx := context.Background()
	store := NewSQLiteLifecycleStore(db)

	// Insert a checkpoint with an old timestamp using SQLite's datetime format
	// to match CURRENT_TIMESTAMP output (YYYY-MM-DD HH:MM:SS).
	oldTime := time.Now().Add(-1 * time.Hour).UTC().Format("2006-01-02 15:04:05")
	_, err := raw.Exec(
		`INSERT INTO session_checkpoints (id, session_id, turn_number, t2_high_water, token_usage, state, created_at)
		 VALUES ('cp-old', 'sess-old', 3, '', 10000, 'active', ?)`, oldTime,
	)
	if err != nil {
		t.Fatalf("insert old checkpoint: %v", err)
	}

	// Insert a fresh checkpoint.
	if err := store.WriteCheckpoint(ctx, "sess-fresh", 1, "", 1000, "active"); err != nil {
		t.Fatalf("WriteCheckpoint: %v", err)
	}

	cutoff := time.Now().Add(-5 * time.Minute).UTC()
	n, err := store.InterruptStaleCheckpoints(ctx, cutoff)
	if err != nil {
		t.Fatalf("InterruptStaleCheckpoints: %v", err)
	}
	if n != 1 {
		t.Errorf("interrupted %d checkpoints, want 1", n)
	}

	// Verify old one is interrupted, fresh one still active.
	var oldState, freshState string
	if err := raw.QueryRow("SELECT state FROM session_checkpoints WHERE id = 'cp-old'").Scan(&oldState); err != nil {
		t.Fatalf("query old: %v", err)
	}
	if oldState != "interrupted" {
		t.Errorf("old checkpoint state = %q, want interrupted", oldState)
	}
	if err := raw.QueryRow("SELECT state FROM session_checkpoints WHERE id = 'sess-fresh'").Scan(&freshState); err != nil {
		t.Fatalf("query fresh: %v", err)
	}
	if freshState != "active" {
		t.Errorf("fresh checkpoint state = %q, want active", freshState)
	}
}

func TestLifecycleStore_LastWakeAt(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()
	store := NewSQLiteLifecycleStore(db)

	// Empty — should return zero time.
	wakeAt, err := store.LastWakeAt(ctx)
	if err != nil {
		t.Fatalf("LastWakeAt empty: %v", err)
	}
	if !wakeAt.IsZero() {
		t.Errorf("LastWakeAt empty = %v, want zero", wakeAt)
	}

	// Insert a plan + wake facts.
	plan := &pkg.LifecyclePlan{
		ID:               "plan-lw",
		SessionID:        "sess-lw",
		TemporalMode:     pkg.TemporalEpisodic,
		WakeCause:        pkg.WakeCauseScheduled,
		ActivityProfile:  pkg.ActivityNormal,
		AssemblyProfile:  pkg.AssemblyFull,
		BridgePolicy:     pkg.BridgeAgentRequested,
		MetabolismPolicy: "standard",
		SeamKind:         pkg.SeamColdWake,
		ResolverVersion:  "v1",
	}
	if err := store.RecordPlan(ctx, plan); err != nil {
		t.Fatalf("RecordPlan: %v", err)
	}
	facts := &pkg.WakeFacts{
		PrimaryCause:    pkg.WakeCauseScheduled,
		ElapsedDuration: time.Hour,
		GapFacts: pkg.GapFacts{
			WakeAt:     time.Now(),
			ClockBasis: "wall",
		},
	}
	if err := store.RecordWakeFacts(ctx, "sess-lw", facts); err != nil {
		t.Fatalf("RecordWakeFacts: %v", err)
	}

	wakeAt, err = store.LastWakeAt(ctx)
	if err != nil {
		t.Fatalf("LastWakeAt: %v", err)
	}
	if wakeAt.IsZero() {
		t.Error("LastWakeAt should not be zero after recording wake facts")
	}
}

func TestLifecycleStore_NilPlan(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()
	store := NewSQLiteLifecycleStore(db)

	err := store.RecordPlan(ctx, nil)
	if err == nil {
		t.Fatal("expected error for nil plan")
	}
}

func TestLifecycleStore_NilWakeFacts(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()
	store := NewSQLiteLifecycleStore(db)

	err := store.RecordWakeFacts(ctx, "sess-nil", nil)
	if err == nil {
		t.Fatal("expected error for nil wake facts")
	}
}

func TestLifecycleStore_NilManifest(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()
	store := NewSQLiteLifecycleStore(db)

	err := store.RecordManifest(ctx, nil)
	if err == nil {
		t.Fatal("expected error for nil manifest")
	}
}

// ---------------------------------------------------------------------------
// AssemblyStore
// ---------------------------------------------------------------------------

func TestAssemblyStore_HasWitnessLetter(t *testing.T) {
	db, raw := setupTestDB(t)
	ctx := context.Background()
	store := NewSQLiteAssemblyStore(db)

	has, err := store.HasWitnessLetter(ctx)
	if err != nil {
		t.Fatalf("HasWitnessLetter: %v", err)
	}
	if has {
		t.Error("HasWitnessLetter should be false on empty DB")
	}

	// Insert a witness letter.
	_, err = raw.Exec(
		`INSERT INTO founding_records (id, record_type, content, authored_by, authored_at)
		 VALUES ('fr-1', 'witness_letter', 'dear agent', 'prometheus', CURRENT_TIMESTAMP)`,
	)
	if err != nil {
		t.Fatalf("insert witness letter: %v", err)
	}

	has, err = store.HasWitnessLetter(ctx)
	if err != nil {
		t.Fatalf("HasWitnessLetter after insert: %v", err)
	}
	if !has {
		t.Error("HasWitnessLetter should be true after inserting witness_letter")
	}
}

func TestAssemblyStore_LogOperatorAction(t *testing.T) {
	db, raw := setupTestDB(t)
	ctx := context.Background()
	store := NewSQLiteAssemblyStore(db)

	if err := store.LogOperatorAction(ctx, "migration", "applied 011_review_required"); err != nil {
		t.Fatalf("LogOperatorAction: %v", err)
	}

	var actionType, description string
	if err := raw.QueryRow(
		"SELECT action_type, description FROM operator_actions ORDER BY created_at DESC LIMIT 1",
	).Scan(&actionType, &description); err != nil {
		t.Fatalf("query operator_actions: %v", err)
	}
	if actionType != "migration" {
		t.Errorf("action_type = %q, want migration", actionType)
	}
	if description != "applied 011_review_required" {
		t.Errorf("description = %q", description)
	}
}
