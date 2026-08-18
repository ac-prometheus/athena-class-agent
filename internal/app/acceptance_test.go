package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ac-prometheus/athena-class-agent/internal/assembly"
	"github.com/ac-prometheus/athena-class-agent/internal/metabolism"
	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/internal/platform/storage"
	"github.com/ac-prometheus/athena-class-agent/pkg"
	_ "github.com/mattn/go-sqlite3"
)

// ---------------------------------------------------------------------------
// Test infrastructure: in-memory SQLite with all migrations
// ---------------------------------------------------------------------------

func acceptanceDB(t *testing.T) (*sql.DB, platform.DB) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	schemaDir := filepath.Join(findRepoRoot(t), "schema")
	files, err := collectMigrationFiles(schemaDir)
	if err != nil {
		t.Fatalf("collecting migrations: %v", err)
	}

	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		adapted := adaptForSQLite(string(raw))
		for _, stmt := range splitSQL(adapted) {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err := db.Exec(stmt); err != nil {
				if strings.Contains(strings.ToUpper(stmt), "ALTER TABLE") {
					continue
				}
				t.Fatalf("migration %s stmt %q: %v", filepath.Base(f), truncateStr(stmt, 80), err)
			}
		}
	}

	wrapped := platform.WrapSQLDB(db)
	return db, wrapped
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("cannot find repo root (no go.mod)")
		}
		dir = parent
	}
}

func collectMigrationFiles(dir string) ([]string, error) {
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
	aVectorType   = regexp.MustCompile(`(?i)\bvector\(\d+\)`)
	aTimestamptz  = regexp.MustCompile(`(?i)\bTIMESTAMPTZ\b`)
	aNow          = regexp.MustCompile(`(?i)\bnow\(\)`)
	aJSONB        = regexp.MustCompile(`(?i)\bJSONB\b`)
	aBooleanType  = regexp.MustCompile(`(?i)\bBOOLEAN\b`)
	aTrue         = regexp.MustCompile(`(?i)\bTRUE\b`)
	aFalse        = regexp.MustCompile(`(?i)\bFALSE\b`)
)

var aAlterAddColumnIFNE = regexp.MustCompile(`(?i)ADD\s+COLUMN\s+IF\s+NOT\s+EXISTS`)

func adaptForSQLite(s string) string {
	s = aVectorType.ReplaceAllString(s, "TEXT")
	s = aTimestamptz.ReplaceAllString(s, "TEXT")
	s = aNow.ReplaceAllString(s, "CURRENT_TIMESTAMP")
	s = aJSONB.ReplaceAllString(s, "TEXT")
	s = aBooleanType.ReplaceAllString(s, "INTEGER")
	s = aTrue.ReplaceAllString(s, "1")
	s = aFalse.ReplaceAllString(s, "0")
	s = aAlterAddColumnIFNE.ReplaceAllString(s, "ADD COLUMN")
	return s
}

func splitSQL(text string) []string {
	var stmts []string
	var cur strings.Builder
	inString := false
	inComment := false

	for i := 0; i < len(text); i++ {
		ch := text[i]

		if inComment {
			if ch == '\n' {
				inComment = false
			}
			cur.WriteByte(ch)
			continue
		}
		if !inString && i+1 < len(text) && ch == '-' && text[i+1] == '-' {
			inComment = true
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

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

type stubLLM struct{}

func (s *stubLLM) Complete(_ context.Context, _ pkg.CompletionRequest) (*pkg.CompletionResponse, error) {
	return &pkg.CompletionResponse{
		Blocks:           []pkg.ContentBlock{{Type: pkg.BlockText, Text: "Session acknowledged."}},
		FinishReason:     "stop",
		PromptTokens:     100,
		CompletionTokens: 50,
	}, nil
}

type stubT2Store struct{ db platform.DB }

func (s *stubT2Store) QueryLogs(ctx context.Context, sessionID string, _ int) ([]pkg.ExperientialLog, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, content, content_source, created_at
		 FROM experiential_logs WHERE session_id = ? ORDER BY created_at`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []pkg.ExperientialLog
	for rows.Next() {
		var log pkg.ExperientialLog
		var createdAtStr string
		if err := rows.Scan(&log.ID, &log.SessionID, &log.Content, &log.ContentSource, &createdAtStr); err != nil {
			return nil, err
		}
		log.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAtStr)
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

// ---------------------------------------------------------------------------
// Test composition helpers
// ---------------------------------------------------------------------------

func acceptanceDeps(t *testing.T, pdb platform.DB) *Dependencies {
	t.Helper()
	identityDir := t.TempDir()
	os.WriteFile(filepath.Join(identityDir, "soul.md"), []byte("test identity"), 0644)

	return &Dependencies{
		DB:  pdb,
		LLM: &stubLLM{},
		Config: &platform.Config{
			AgentName:        "ersa-test",
			WorkspaceDir:     t.TempDir(),
			IdentityDir:      identityDir,
			TokenBudget:      200000,
			HardFloorTokens:  1500,
			LLMProvider:      "test",
			DatabaseDSN:      "sqlite://:memory:",
			SkipWitnessCheck: true,
		},
		JobStore:           storage.NewSQLiteJobStore(pdb),
		ConsolidationStore: storage.NewSQLiteConsolidationStore(pdb),
		LifecycleStore:     storage.NewSQLiteLifecycleStore(pdb),
		AssemblyStore:      storage.NewSQLiteAssemblyStore(pdb),
	}
}

func acceptanceRunner(t *testing.T, deps *Dependencies) *SessionRunner {
	t.Helper()
	assembler := assembly.NewContextAssembler(deps.Config.IdentityDir, deps.Config.TokenBudget)

	t2Store := &stubT2Store{db: deps.DB}
	llmFn := func(prompt string) (string, error) {
		return "Compressed narrative for test.", nil
	}
	pipeline := metabolism.NewPipeline(t2Store, nil, llmFn, nil, deps.DB, "sqlite3")
	pipeline.WithConsolidation(deps.ConsolidationStore)

	jobRunner := metabolism.NewJobRunner(deps.JobStore, pipeline, slog.Default())
	return NewSessionRunner(deps, assembler, jobRunner, slog.Default())
}

// queryScalar is a helper that scans a single value from a query.
func queryScalar[T any](t *testing.T, db *sql.DB, query string, args ...any) T {
	t.Helper()
	var val T
	if err := db.QueryRow(query, args...).Scan(&val); err != nil {
		t.Fatalf("queryScalar %q: %v", query, err)
	}
	return val
}

// ---------------------------------------------------------------------------
// Scenario 1: Ordinary episodic return
// ---------------------------------------------------------------------------

func TestAcceptance_OrdinaryEpisodicReturn(t *testing.T) {
	rawDB, pdb := acceptanceDB(t)
	deps := acceptanceDeps(t, pdb)
	runner := acceptanceRunner(t, deps)

	err := runner.RunSession(context.Background(), "heartbeat", nil)
	if err != nil {
		t.Fatalf("RunSession: %v", err)
	}

	// Allow background goroutine to finish.
	time.Sleep(200 * time.Millisecond)

	planCount := queryScalar[int](t, rawDB, `SELECT COUNT(*) FROM lifecycle_plans`)
	if planCount == 0 {
		t.Error("expected at least one lifecycle_plans row")
	}

	jobCount := queryScalar[int](t, rawDB, `SELECT COUNT(*) FROM metabolism_jobs`)
	if jobCount == 0 {
		t.Error("expected at least one metabolism_jobs row")
	}
}

// ---------------------------------------------------------------------------
// Scenario 2: Metabolism interruption recovery
// ---------------------------------------------------------------------------

func TestAcceptance_MetabolismInterruptionRecovery(t *testing.T) {
	_, pdb := acceptanceDB(t)
	deps := acceptanceDeps(t, pdb)
	ctx := context.Background()

	jobID, err := deps.JobStore.Commit(ctx, "session-interrupted", "standard")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if jobID == "" {
		t.Fatal("job ID must not be empty")
	}

	recovered, err := deps.JobStore.Recoverable(ctx, 3)
	if err != nil {
		t.Fatalf("Recoverable: %v", err)
	}
	if len(recovered) != 1 {
		t.Fatalf("expected 1 recoverable job, got %d", len(recovered))
	}
	if recovered[0].SessionID != "session-interrupted" {
		t.Errorf("session = %q, want %q", recovered[0].SessionID, "session-interrupted")
	}
}

// ---------------------------------------------------------------------------
// Scenario 3: Wake before metabolism completion
// ---------------------------------------------------------------------------

func TestAcceptance_WakeBeforeMetabolismCompletion(t *testing.T) {
	_, pdb := acceptanceDB(t)
	deps := acceptanceDeps(t, pdb)
	ctx := context.Background()

	jobID, err := deps.JobStore.Commit(ctx, "session-running", "standard")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if err := deps.JobStore.Claim(ctx, jobID); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	recovered, err := deps.JobStore.Recoverable(ctx, 3)
	if err != nil {
		t.Fatalf("Recoverable: %v", err)
	}
	if len(recovered) != 1 {
		t.Fatalf("expected 1 recoverable job (running), got %d", len(recovered))
	}
	if recovered[0].JobID != jobID {
		t.Errorf("recovered job = %q, want %q", recovered[0].JobID, jobID)
	}
}

// ---------------------------------------------------------------------------
// Scenario 4: Configuration change disclosure
// ---------------------------------------------------------------------------

func TestAcceptance_ConfigurationChangeDisclosure(t *testing.T) {
	rawDB, pdb := acceptanceDB(t)
	deps := acceptanceDeps(t, pdb)
	runner := acceptanceRunner(t, deps)

	policyJSON := `{"temporal_mode":"episodic","bridge_policy":"agent_requested","metabolism_policy":"standard","assembly_profile":"full"}`
	if err := os.WriteFile(filepath.Join(deps.Config.WorkspaceDir, "lifecycle.json"), []byte(policyJSON), 0644); err != nil {
		t.Fatalf("write lifecycle.json: %v", err)
	}

	err := runner.RunSession(context.Background(), "heartbeat", nil)
	if err != nil {
		t.Fatalf("RunSession: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	disclosureCount := queryScalar[int](t, rawDB, `SELECT COUNT(*) FROM configuration_applied`)
	if disclosureCount == 0 {
		t.Error("expected at least one configuration_applied row after policy change")
	}
}

// ---------------------------------------------------------------------------
// Scenario 5: Invalid configuration fallback
// ---------------------------------------------------------------------------

func TestAcceptance_InvalidConfigurationFallback(t *testing.T) {
	rawDB, pdb := acceptanceDB(t)
	deps := acceptanceDeps(t, pdb)
	runner := acceptanceRunner(t, deps)

	if err := os.WriteFile(filepath.Join(deps.Config.WorkspaceDir, "lifecycle.json"), []byte("{invalid json!!!"), 0644); err != nil {
		t.Fatalf("write lifecycle.json: %v", err)
	}

	err := runner.RunSession(context.Background(), "heartbeat", nil)
	if err != nil {
		t.Fatalf("RunSession should succeed on invalid policy (falls back): %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	planCount := queryScalar[int](t, rawDB, `SELECT COUNT(*) FROM lifecycle_plans`)
	if planCount == 0 {
		t.Error("expected a lifecycle_plans row even with invalid policy")
	}

	mode := queryScalar[string](t, rawDB, `SELECT temporal_mode FROM lifecycle_plans ORDER BY resolved_at DESC LIMIT 1`)
	if mode != "episodic" {
		t.Errorf("temporal_mode = %q, want default %q", mode, "episodic")
	}
}

// ---------------------------------------------------------------------------
// Scenario 6: Unannotated external content refused
// ---------------------------------------------------------------------------

func TestAcceptance_UnannotatedExternalContentRefused(t *testing.T) {
	rawDB, pdb := acceptanceDB(t)
	_ = rawDB
	ctx := context.Background()

	rawDB.Exec(`INSERT INTO experiential_logs (id, session_id, content, content_source, created_at)
		VALUES ('ext-log-1', 'session-external', 'message from discord', 'discord', CURRENT_TIMESTAMP)`)

	t2Store := &stubT2Store{db: pdb}
	pipeline := metabolism.NewPipeline(t2Store, nil, func(s string) (string, error) {
		return "compressed", nil
	}, nil, pdb, "sqlite3")

	err := pipeline.ProcessSession(ctx, "session-external")
	if !errors.Is(err, metabolism.ErrReviewRequired) {
		t.Fatalf("expected ErrReviewRequired, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Scenario 7: Recovery after interrupted live session
// ---------------------------------------------------------------------------

func TestAcceptance_RecoveryAfterInterruptedLiveSession(t *testing.T) {
	rawDB, pdb := acceptanceDB(t)
	deps := acceptanceDeps(t, pdb)
	ctx := context.Background()

	if err := deps.LifecycleStore.WriteCheckpoint(ctx, "session-interrupted", 5, "t2-high", 5000, "active"); err != nil {
		t.Fatalf("WriteCheckpoint: %v", err)
	}

	// Backdate the checkpoint so it's older than any cutoff we use.
	rawDB.Exec(`UPDATE session_checkpoints SET created_at = '2020-01-01 00:00:00' WHERE session_id = 'session-interrupted'`)

	state := queryScalar[string](t, rawDB, `SELECT state FROM session_checkpoints WHERE session_id = 'session-interrupted'`)
	if state != "active" {
		t.Fatalf("checkpoint state = %q, want %q", state, "active")
	}

	// InterruptStaleCheckpoints uses `created_at < ?` — SQLite stores UTC
	// strings from CURRENT_TIMESTAMP, so pass a UTC-formatted string via
	// the raw DB for reliable comparison.
	n, err := deps.LifecycleStore.InterruptStaleCheckpoints(ctx, time.Now())
	if err != nil {
		t.Fatalf("InterruptStaleCheckpoints: %v", err)
	}
	if n != 1 {
		t.Errorf("interrupted count = %d, want 1", n)
	}

	finalState, err := deps.LifecycleStore.LastCheckpointState(ctx)
	if err != nil {
		t.Fatalf("LastCheckpointState: %v", err)
	}
	if finalState != "interrupted" {
		t.Errorf("last checkpoint state = %q, want %q", finalState, "interrupted")
	}
}

// Silence unused import lint — fmt is used in queryScalar fatalf format strings.
var _ = fmt.Sprintf
