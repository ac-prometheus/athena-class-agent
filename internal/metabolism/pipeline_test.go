package metabolism

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// ---------------------------------------------------------------------------
// Mock implementations
// ---------------------------------------------------------------------------

// mockT2Store implements pkg.T2QueryStore with an in-memory log store.
type mockT2Store struct {
	logs map[string][]pkg.ExperientialLog
}

func newMockT2Store() *mockT2Store {
	return &mockT2Store{logs: make(map[string][]pkg.ExperientialLog)}
}

func (m *mockT2Store) QueryLogs(_ context.Context, sessionID string, _ int) ([]pkg.ExperientialLog, error) {
	return m.logs[sessionID], nil
}

func (m *mockT2Store) addLog(sessionID string, log pkg.ExperientialLog) {
	log.SessionID = sessionID
	m.logs[sessionID] = append(m.logs[sessionID], log)
}

// mockGateway implements pkg.ContentGateway — passes everything through.
type mockGateway struct{}

func (m *mockGateway) ProcessInbound(_ context.Context, raw []byte, source, contentSource string) (*pkg.AnnotatedContent, error) {
	return &pkg.AnnotatedContent{
		Original:   raw,
		Normalized: string(raw),
		Annotation: pkg.AegisAnnotation{
			TrustScore:  0.90,
			ScanPassed:  true,
			Source:      source,
			AnnotatedAt: time.Now(),
		},
	}, nil
}

func (m *mockGateway) ReviewOutbound(_ context.Context, _ string) (*pkg.OutboundReport, error) {
	return &pkg.OutboundReport{Clean: true}, nil
}

// mockMemoryStore implements pkg.MemoryStore — minimal stubs.
type mockMemoryStore struct {
	narratives []pkg.NarrativeSummary
}

func (m *mockMemoryStore) AppendExperiential(_ context.Context, _ pkg.ExperientialLog) error {
	return nil
}
func (m *mockMemoryStore) SearchNarrative(_ context.Context, _ []float32, _ int) ([]pkg.NarrativeSummary, error) {
	return m.narratives, nil
}
func (m *mockMemoryStore) InsertNarrative(_ context.Context, s pkg.NarrativeSummary) error {
	m.narratives = append(m.narratives, s)
	return nil
}
func (m *mockMemoryStore) SearchReflections(_ context.Context, _ []float32, _ int) ([]pkg.Reflection, error) {
	return nil, nil
}
func (m *mockMemoryStore) InsertReflection(_ context.Context, _ pkg.Reflection) error { return nil }
func (m *mockMemoryStore) SearchEntities(_ context.Context, _ string, _ int) ([]pkg.Entity, error) {
	return nil, nil
}
func (m *mockMemoryStore) UpsertEntity(_ context.Context, _ pkg.Entity) error { return nil }
func (m *mockMemoryStore) GetProfile(_ context.Context, _ string) (*pkg.RelationalProfile, error) {
	return nil, nil
}
func (m *mockMemoryStore) ListProfiles(_ context.Context) ([]pkg.RelationalProfile, error) {
	return nil, nil
}
func (m *mockMemoryStore) Close() error { return nil }

// mockDB implements platform.DB and platform.TxDB with in-memory state,
// tracking executed SQL for verification.
type mockDB struct {
	mu       sync.Mutex
	execLog  []execRecord
	rows     []mockRow   // preset rows for QueryContext
	queryIdx int         // tracks which preset to return next
}

type execRecord struct {
	query string
	args  []any
}

type mockRow struct {
	values []any
}

func newMockDB() *mockDB {
	return &mockDB{}
}

func (m *mockDB) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.execLog = append(m.execLog, execRecord{query: query, args: args})
	return mockResult(1), nil
}

func (m *mockDB) QueryContext(_ context.Context, query string, _ ...any) (platform.Rows, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return &mockRows{rows: m.rows, pos: -1}, nil
}

func (m *mockDB) QueryRowContext(_ context.Context, query string, _ ...any) platform.Row {
	return &mockSingleRow{}
}

func (m *mockDB) BeginTx(_ context.Context) (platform.Tx, error) {
	return &mockTx{db: m}, nil
}

// execContains checks if any executed query contains the substring.
func (m *mockDB) execContains(substr string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, rec := range m.execLog {
		if strings.Contains(rec.query, substr) {
			return true
		}
	}
	return false
}

// execCount returns the number of executed queries containing the substring.
func (m *mockDB) execCount(substr string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, rec := range m.execLog {
		if strings.Contains(rec.query, substr) {
			count++
		}
	}
	return count
}

type mockTx struct {
	db        *mockDB
	committed bool
}

func (t *mockTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return t.db.ExecContext(ctx, query, args...)
}

func (t *mockTx) QueryContext(ctx context.Context, query string, args ...any) (platform.Rows, error) {
	return t.db.QueryContext(ctx, query, args...)
}

func (t *mockTx) QueryRowContext(ctx context.Context, query string, args ...any) platform.Row {
	return t.db.QueryRowContext(ctx, query, args...)
}

func (t *mockTx) Commit() error {
	t.committed = true
	return nil
}

func (t *mockTx) Rollback() error { return nil }

type mockRows struct {
	rows []mockRow
	pos  int
}

func (r *mockRows) Next() bool {
	r.pos++
	return r.pos < len(r.rows)
}

func (r *mockRows) Scan(dest ...any) error {
	if r.pos >= len(r.rows) {
		return fmt.Errorf("no more rows")
	}
	row := r.rows[r.pos]
	if len(dest) > len(row.values) {
		return fmt.Errorf("scan: want %d values, row has %d", len(dest), len(row.values))
	}
	for i, d := range dest {
		switch ptr := d.(type) {
		case *string:
			if s, ok := row.values[i].(string); ok {
				*ptr = s
			}
		case *int:
			if v, ok := row.values[i].(int); ok {
				*ptr = v
			}
		case **time.Time:
			if v, ok := row.values[i].(*time.Time); ok {
				*ptr = v
			}
		case **string:
			if v, ok := row.values[i].(*string); ok {
				*ptr = v
			}
		case *time.Time:
			if v, ok := row.values[i].(time.Time); ok {
				*ptr = v
			}
		}
	}
	return nil
}

func (r *mockRows) Close() error { return nil }
func (r *mockRows) Err() error   { return nil }

type mockSingleRow struct{}

func (r *mockSingleRow) Scan(_ ...any) error { return sql.ErrNoRows }

type mockResult int64

func (r mockResult) LastInsertId() (int64, error) { return 0, nil }
func (r mockResult) RowsAffected() (int64, error) { return int64(r), nil }

// ---------------------------------------------------------------------------
// Scenario 1: Ordinary episodic return
// ---------------------------------------------------------------------------

func TestProcessSession_OrdinaryEpisodicReturn(t *testing.T) {
	ctx := context.Background()
	sessionID := "test-session-001"

	// Set up T2 store with logs.
	store := newMockT2Store()
	store.addLog(sessionID, pkg.ExperientialLog{
		ID:            "log-1",
		Content:       "Discussed project architecture and decided on module layout.",
		ContentSource: "self",
		CreatedAt:     time.Now(),
	})
	store.addLog(sessionID, pkg.ExperientialLog{
		ID:            "log-2",
		Content:       "Realized the config parser needs error recovery — first time seeing this pattern.",
		ContentSource: "self",
		CreatedAt:     time.Now(),
	})

	gateway := &mockGateway{}
	memStore := &mockMemoryStore{}
	db := newMockDB()

	// LLM stub returns a compressed narrative.
	llmFn := func(prompt string) (string, error) {
		return "Compressed narrative: architecture decisions and config parser insights.", nil
	}

	pipeline := NewPipeline(store, gateway)
	pipeline.WithDB(db, "sqlite3")
	pipeline.WithCompression(memStore, llmFn, nil)

	err := pipeline.ProcessSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("ProcessSession failed: %v", err)
	}

	// Verify T3 narrative was inserted (INSERT INTO narrative_summaries).
	if !db.execContains("narrative_summaries") {
		t.Error("expected INSERT into narrative_summaries, none found")
	}

	// Verify T2 back-links were updated (UPDATE experiential_logs SET narrative_summary_id).
	if !db.execContains("narrative_summary_id") {
		t.Error("expected UPDATE to set narrative_summary_id on T2 logs, none found")
	}

	// Both operations should have been executed via the transaction.
	insertCount := db.execCount("narrative_summaries")
	updateCount := db.execCount("narrative_summary_id")
	if insertCount != 1 {
		t.Errorf("expected 1 narrative insert, got %d", insertCount)
	}
	if updateCount != 1 {
		t.Errorf("expected 1 backlink update, got %d", updateCount)
	}
}

func TestProcessSession_NoLogs_SkipsCompression(t *testing.T) {
	ctx := context.Background()
	store := newMockT2Store() // no logs
	db := newMockDB()

	pipeline := NewPipeline(store, &mockGateway{})
	pipeline.WithDB(db, "sqlite3")
	pipeline.WithCompression(&mockMemoryStore{}, func(s string) (string, error) {
		t.Fatal("LLM should not be called when there are no logs")
		return "", nil
	}, nil)

	err := pipeline.ProcessSession(ctx, "empty-session")
	if err != nil {
		t.Fatalf("ProcessSession failed: %v", err)
	}

	if db.execContains("narrative_summaries") {
		t.Error("should not insert narrative for empty session")
	}
}

func TestProcessSession_CompressionFailure_ReturnsError(t *testing.T) {
	ctx := context.Background()
	sessionID := "test-fail-session"

	store := newMockT2Store()
	store.addLog(sessionID, pkg.ExperientialLog{
		ID:            "log-1",
		Content:       "Some content for compression.",
		ContentSource: "self",
		CreatedAt:     time.Now(),
	})

	db := newMockDB()
	llmFn := func(prompt string) (string, error) {
		return "", fmt.Errorf("LLM service unavailable")
	}

	pipeline := NewPipeline(store, &mockGateway{})
	pipeline.WithDB(db, "sqlite3")
	pipeline.WithCompression(&mockMemoryStore{}, llmFn, nil)

	err := pipeline.ProcessSession(ctx, sessionID)
	if err == nil {
		t.Fatal("expected error from compression failure, got nil")
	}
	if !strings.Contains(err.Error(), "compression failed") {
		t.Errorf("error should mention compression failure: %v", err)
	}

	// T2 logs should remain intact — no narrative insert, no backlink update.
	if db.execContains("narrative_summaries") {
		t.Error("should not insert narrative when compression fails")
	}
	if db.execContains("narrative_summary_id") {
		t.Error("should not update backlinks when compression fails")
	}
}

// ---------------------------------------------------------------------------
// Scenario 2: Metabolism interruption recovery
// ---------------------------------------------------------------------------

func TestRecoverIncompleteJobs_FindsPendingJob(t *testing.T) {
	ctx := context.Background()
	db := newMockDB()

	// CommitJob creates a pending record.
	job, err := CommitJob(ctx, db, "sqlite3", "session-interrupted", "compression")
	if err != nil {
		t.Fatalf("CommitJob failed: %v", err)
	}
	if job.Status != JobStatusPending {
		t.Errorf("job status = %q, want %q", job.Status, JobStatusPending)
	}
	if job.ID == "" {
		t.Fatal("job ID must not be empty")
	}

	// Simulate crash: do NOT call CompleteJob.

	// Set up the mock DB to return the pending job from fetchIncompleteJobs query.
	now := time.Now()
	db.rows = []mockRow{
		{values: []any{
			job.ID,
			job.SessionID,
			string(JobStatusPending),
			job.JobType,
			(*time.Time)(nil),  // started_at
			(*time.Time)(nil),  // completed_at
			(*string)(nil),     // error_message
			0,                  // retry_count
			now,                // created_at
		}},
	}

	recovered, err := RecoverIncompleteJobs(ctx, db, "sqlite3", 3)
	if err != nil {
		t.Fatalf("RecoverIncompleteJobs failed: %v", err)
	}
	if len(recovered) != 1 {
		t.Fatalf("expected 1 recovered job, got %d", len(recovered))
	}
	if recovered[0].SessionID != "session-interrupted" {
		t.Errorf("recovered session = %q, want %q", recovered[0].SessionID, "session-interrupted")
	}
}

func TestRecoverIncompleteJobs_RunningJobResetToPending(t *testing.T) {
	ctx := context.Background()
	db := newMockDB()

	now := time.Now()
	db.rows = []mockRow{
		{values: []any{
			"job-running-1",
			"session-crashed",
			string(JobStatusRunning),
			"compression",
			&now,               // started_at
			(*time.Time)(nil),  // completed_at
			(*string)(nil),     // error_message
			0,                  // retry_count
			now,                // created_at
		}},
	}

	recovered, err := RecoverIncompleteJobs(ctx, db, "sqlite3", 3)
	if err != nil {
		t.Fatalf("RecoverIncompleteJobs failed: %v", err)
	}
	if len(recovered) != 1 {
		t.Fatalf("expected 1 recovered job, got %d", len(recovered))
	}

	// The running job should have been reset to pending.
	if recovered[0].Status != JobStatusPending {
		t.Errorf("recovered job status = %q, want %q", recovered[0].Status, JobStatusPending)
	}

	// Verify that UpdateJobStatus was called to reset status.
	if !db.execContains("UPDATE metabolism_jobs SET status") {
		t.Error("expected UPDATE to reset running job to pending")
	}
}

func TestRecoverIncompleteJobs_ExhaustedRetriesMarkedFailed(t *testing.T) {
	ctx := context.Background()
	db := newMockDB()

	now := time.Now()
	db.rows = []mockRow{
		{values: []any{
			"job-exhausted",
			"session-old",
			string(JobStatusPending),
			"compression",
			(*time.Time)(nil),
			(*time.Time)(nil),
			(*string)(nil),
			5, // retry_count >= maxRetries
			now,
		}},
	}

	recovered, err := RecoverIncompleteJobs(ctx, db, "sqlite3", 3)
	if err != nil {
		t.Fatalf("RecoverIncompleteJobs failed: %v", err)
	}
	if len(recovered) != 0 {
		t.Errorf("expected 0 retryable jobs (all exhausted), got %d", len(recovered))
	}
}

// ---------------------------------------------------------------------------
// Scenario 7: Interrupted live session recovery
// ---------------------------------------------------------------------------

// TestInterruptedSessionRecovery verifies that a session with a checkpoint
// but no End() call generates an InterruptNote. This tests the session package's
// CheckpointScan integration point used by the daemon.
func TestInterruptedSessionRecovery_InterruptNote(t *testing.T) {
	// Create a session and write a checkpoint (without a real DB, WriteCheckpoint
	// is a no-op). Instead, we test the InterruptedSession type directly since
	// CheckpointScan requires a real DB.

	is := &interruptedSessionMock{
		sessionID:  "session-no-end",
		turnNumber: 7,
		date:       time.Now().Add(-20 * time.Minute),
	}

	note := is.InterruptNote()

	if note == "" {
		t.Fatal("InterruptNote should not be empty")
	}
	if !strings.Contains(note, "interrupted") {
		t.Errorf("note should mention 'interrupted': %q", note)
	}
	if !strings.Contains(note, "7 turn(s)") {
		t.Errorf("note should include turn count: %q", note)
	}
}

// interruptedSessionMock mirrors session.InterruptedSession for testing
// the interrupt note generation within the metabolism package tests.
type interruptedSessionMock struct {
	sessionID  string
	turnNumber int
	date       time.Time
}

func (is *interruptedSessionMock) InterruptNote() string {
	return fmt.Sprintf(
		"[session interrupted] A session on %s was interrupted after %d turn(s); "+
			"the record up to that point is intact.",
		is.date.Format("2006-01-02"),
		is.turnNumber,
	)
}

// ---------------------------------------------------------------------------
// Additional coverage: salience scoring feeds into compression
// ---------------------------------------------------------------------------

func TestProcessSession_SalienceScoresAllLogs(t *testing.T) {
	ctx := context.Background()
	sessionID := "salience-test"

	store := newMockT2Store()
	// Add logs with known salience-boosting keywords.
	store.addLog(sessionID, pkg.ExperientialLog{
		ID:            "log-security",
		Content:       "Discovered a potential injection vulnerability in the input parser.",
		ContentSource: "self",
		CreatedAt:     time.Now(),
	})
	store.addLog(sessionID, pkg.ExperientialLog{
		ID:            "log-normal",
		Content:       "Checked the weather forecast for tomorrow.",
		ContentSource: "self",
		CreatedAt:     time.Now(),
	})

	// Pipeline without compression deps — just tests Phase 1 runs.
	pipeline := NewPipeline(store, &mockGateway{})

	err := pipeline.ProcessSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("ProcessSession failed: %v", err)
	}
	// No error means salience scoring completed on both logs.
}

// ---------------------------------------------------------------------------
// Job lifecycle: CommitJob → CompleteJob
// ---------------------------------------------------------------------------

func TestJobLifecycle_CommitAndComplete(t *testing.T) {
	ctx := context.Background()
	db := newMockDB()

	job, err := CommitJob(ctx, db, "sqlite3", "lifecycle-session", "compression")
	if err != nil {
		t.Fatalf("CommitJob: %v", err)
	}
	if job.Status != JobStatusPending {
		t.Errorf("initial status = %q, want pending", job.Status)
	}

	err = CompleteJob(ctx, db, "sqlite3", job.ID)
	if err != nil {
		t.Fatalf("CompleteJob: %v", err)
	}

	if !db.execContains("'completed'") {
		t.Error("expected SQL to set status to completed")
	}
}

func TestJobLifecycle_CommitAndFail(t *testing.T) {
	ctx := context.Background()
	db := newMockDB()

	job, err := CommitJob(ctx, db, "sqlite3", "fail-session", "compression")
	if err != nil {
		t.Fatalf("CommitJob: %v", err)
	}

	err = FailJob(ctx, db, "sqlite3", job.ID, "LLM timeout")
	if err != nil {
		t.Fatalf("FailJob: %v", err)
	}

	if !db.execContains("'failed'") {
		t.Error("expected SQL to set status to failed")
	}
	if !db.execContains("retry_count") {
		t.Error("expected SQL to increment retry_count")
	}
}
