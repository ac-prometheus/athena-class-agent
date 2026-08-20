package engine

import (
	"context"
	"testing"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// mockMemoryStore is a test double for pkg.MemoryStore that records T2 appends.
type mockMemoryStore struct {
	appended []pkg.ExperientialLog
	err      error
}

func (m *mockMemoryStore) AppendExperiential(_ context.Context, entry pkg.ExperientialLog) error {
	if m.err != nil {
		return m.err
	}
	m.appended = append(m.appended, entry)
	return nil
}

func (m *mockMemoryStore) SearchNarrative(_ context.Context, _ []float32, _ int) ([]pkg.NarrativeSummary, error) {
	return nil, nil
}
func (m *mockMemoryStore) InsertNarrative(_ context.Context, _ pkg.NarrativeSummary) error {
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

// ---------------------------------------------------------------------------
// T2LoggerHook: C-2 fix — tool-only turns write tool provenance, not empty entries.
// ---------------------------------------------------------------------------

func TestT2LoggerHook_SelfContent_LoggedAsSelf(t *testing.T) {
	store := &mockMemoryStore{}
	hook := NewT2LoggerHook(store)

	turn := TurnResult{
		SessionID:  "sess-1",
		TurnNumber: 1,
		Content:    "Agent reasoning.",
		// No tool results.
	}

	if err := hook.Run(context.Background(), turn); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.appended) != 1 {
		t.Fatalf("expected 1 T2 entry, got %d", len(store.appended))
	}
	entry := store.appended[0]
	if entry.ContentSource != "self" {
		t.Errorf("ContentSource = %q, want 'self'", entry.ContentSource)
	}
	if entry.Content != "Agent reasoning." {
		t.Errorf("Content = %q, want agent text", entry.Content)
	}
}

func TestT2LoggerHook_ToolOnly_WritesToolResults_NotEmpty(t *testing.T) {
	// C-2 fix: tool-only turns must not write empty T2 entries; instead write tool results.
	store := &mockMemoryStore{}
	hook := NewT2LoggerHook(store)

	turn := TurnResult{
		SessionID:  "sess-2",
		TurnNumber: 2,
		Content:    "", // tool-only turn has no self text
		ToolResults: []pkg.ToolResult{
			{CallID: "call-1", Content: "Tool output A"},
			{CallID: "call-2", Content: "Tool output B"},
		},
	}

	if err := hook.Run(context.Background(), turn); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.appended) != 2 {
		t.Fatalf("expected 2 T2 entries (one per tool result), got %d", len(store.appended))
	}
	for _, entry := range store.appended {
		if entry.ContentSource != pkg.ContentSourceToolResult {
			t.Errorf("tool result entry ContentSource = %q, want %q",
				entry.ContentSource, pkg.ContentSourceToolResult)
		}
		if entry.Content == "" {
			t.Error("tool result entry must not have empty Content")
		}
	}
}

func TestT2LoggerHook_MixedTurn_WritesBothEntries(t *testing.T) {
	// Turn with both self text and tool results should produce one "self" entry
	// plus one entry per tool result.
	store := &mockMemoryStore{}
	hook := NewT2LoggerHook(store)

	turn := TurnResult{
		SessionID:  "sess-3",
		TurnNumber: 3,
		Content:    "Agent reasoning after tool.",
		ToolResults: []pkg.ToolResult{
			{CallID: "call-x", Content: "Tool result X"},
		},
	}

	if err := hook.Run(context.Background(), turn); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.appended) != 2 {
		t.Fatalf("expected 2 T2 entries (1 self + 1 tool), got %d", len(store.appended))
	}
	sources := make(map[string]int)
	for _, e := range store.appended {
		sources[e.ContentSource]++
	}
	if sources["self"] != 1 {
		t.Errorf("expected 1 'self' entry, got %d", sources["self"])
	}
	if sources[pkg.ContentSourceToolResult] != 1 {
		t.Errorf("expected 1 'tool-result' entry, got %d", sources[pkg.ContentSourceToolResult])
	}
}

func TestT2LoggerHook_EmptyTurn_WritesNothing(t *testing.T) {
	// A turn with no content and no tool results should not write any T2 entries.
	store := &mockMemoryStore{}
	hook := NewT2LoggerHook(store)

	turn := TurnResult{
		SessionID:  "sess-4",
		TurnNumber: 4,
		Content:    "",
		ToolResults: nil,
	}

	if err := hook.Run(context.Background(), turn); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.appended) != 0 {
		t.Errorf("expected 0 T2 entries for empty turn, got %d", len(store.appended))
	}
}

func TestT2LoggerHook_EmptyToolResult_Skipped(t *testing.T) {
	// Tool results with empty content should be silently skipped (no T2 entry).
	store := &mockMemoryStore{}
	hook := NewT2LoggerHook(store)

	turn := TurnResult{
		SessionID:  "sess-5",
		TurnNumber: 5,
		Content:    "",
		ToolResults: []pkg.ToolResult{
			{CallID: "call-empty", Content: ""}, // empty — must be skipped
		},
	}

	if err := hook.Run(context.Background(), turn); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.appended) != 0 {
		t.Errorf("expected 0 T2 entries for empty tool result, got %d", len(store.appended))
	}
}

func TestT2LoggerHook_SpecificContentSource_Preserved(t *testing.T) {
	// ToolResults that carry a specific ContentSource (browser-content, search-result,
	// forum-content) must be logged with that label — not the generic "tool-result".
	// This is the core of the WP-C3 fix: vocabulary was dead at T2 ingress; now it's live.
	store := &mockMemoryStore{}
	hook := NewT2LoggerHook(store)

	turn := TurnResult{
		SessionID:  "sess-6",
		TurnNumber: 6,
		Content:    "",
		ToolResults: []pkg.ToolResult{
			{CallID: "call-browser", Content: "page content", ContentSource: pkg.ContentSourceBrowserContent},
			{CallID: "call-search", Content: "search results", ContentSource: pkg.ContentSourceSearchResult},
			{CallID: "call-forum", Content: "forum post", ContentSource: pkg.ContentSourceForumContent},
			{CallID: "call-internal", Content: "pinboard data"}, // no ContentSource → fallback
		},
	}

	if err := hook.Run(context.Background(), turn); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.appended) != 4 {
		t.Fatalf("expected 4 T2 entries, got %d", len(store.appended))
	}

	byCallID := make(map[string]string, 4)
	for _, e := range store.appended {
		// Extract call suffix from ID (format: t2-sess-6-6-<ts>-tool<i>)
		byCallID[e.Content] = e.ContentSource
	}

	cases := []struct {
		content string
		want    string
	}{
		{"page content", pkg.ContentSourceBrowserContent},
		{"search results", pkg.ContentSourceSearchResult},
		{"forum post", pkg.ContentSourceForumContent},
		{"pinboard data", pkg.ContentSourceToolResult},
	}
	for _, tc := range cases {
		got := byCallID[tc.content]
		if got != tc.want {
			t.Errorf("content %q: ContentSource = %q, want %q", tc.content, got, tc.want)
		}
	}
}

// TestT2LoggerHook_TurnResultHasToolResults verifies that the engine populates
// ToolResults on TurnResult so T2LoggerHook can consume them (integration smoke test).
func TestT2LoggerHook_TurnResultHasToolResults(t *testing.T) {
	// Build an engine with a T2LoggerHook and run a single tool-call turn.
	store := &mockMemoryStore{}
	hook := NewT2LoggerHook(store)
	pipeline := NewHookPipeline()
	pipeline.RegisterCritical(hook)

	handler := &mockHandler{name: "my-tool", result: "tool-output"}
	reg := newMockRegistry()
	reg.Register(handler)

	client := &mockClient{responses: []*pkg.CompletionResponse{
		toolCallResp("c1", "my-tool", `{}`),
		textResp("done"),
	}}

	eng := NewEngine(client, reg, pipeline)
	eng.WithSessionID("sess-tool")

	_, err := eng.RunLoop(context.Background(), pkg.CompletionRequest{}, EngineConfig{ParallelTools: true})
	if err != nil {
		t.Fatalf("RunLoop: %v", err)
	}

	// The first hook call (tool-call turn) should have written the tool result.
	// The second hook call (final text turn "done") should have written self content.
	var toolResultEntries, selfEntries int
	for _, e := range store.appended {
		switch e.ContentSource {
		case pkg.ContentSourceToolResult:
			toolResultEntries++
		case "self":
			selfEntries++
		}
	}
	if toolResultEntries == 0 {
		t.Error("expected at least 1 tool-result T2 entry from the tool-call turn")
	}
	if selfEntries == 0 {
		t.Error("expected at least 1 self T2 entry from the final text turn")
	}
}

// ---------------------------------------------------------------------------
// TurnResult.ToolResults field wiring (unit test for buildTurnResult + engine)
// ---------------------------------------------------------------------------

func TestTurnResult_ToolResultsField(t *testing.T) {
	// Verify ToolResults is populated in TurnResult when engine executes tools.
	var capturedTurns []TurnResult

	captureHook := NewFuncHook("capture", func(_ context.Context, tr TurnResult) error {
		capturedTurns = append(capturedTurns, tr)
		return nil
	})
	pipeline := NewHookPipeline()
	pipeline.Register(captureHook)

	handler := &mockHandler{name: "probe", result: "probe-result"}
	reg := newMockRegistry()
	reg.Register(handler)

	client := &mockClient{responses: []*pkg.CompletionResponse{
		toolCallResp("c1", "probe", `{}`),
		textResp("done"),
	}}

	eng := NewEngine(client, reg, pipeline)
	_, err := eng.RunLoop(context.Background(), pkg.CompletionRequest{}, EngineConfig{ParallelTools: true})
	if err != nil {
		t.Fatalf("RunLoop: %v", err)
	}

	if len(capturedTurns) < 2 {
		t.Fatalf("expected at least 2 hook calls, got %d", len(capturedTurns))
	}

	// First turn was the tool-call turn; ToolResults should be non-empty.
	toolTurn := capturedTurns[0]
	if len(toolTurn.ToolResults) == 0 {
		t.Error("tool-call TurnResult should have non-empty ToolResults")
	}
	if toolTurn.ToolResults[0].Content != "probe-result" {
		t.Errorf("ToolResults[0].Content = %q, want 'probe-result'", toolTurn.ToolResults[0].Content)
	}

	// Second turn was the final text turn; ToolResults should be nil/empty.
	finalTurn := capturedTurns[1]
	if len(finalTurn.ToolResults) != 0 {
		t.Errorf("final text TurnResult should have empty ToolResults, got %d", len(finalTurn.ToolResults))
	}
}

// Ensure _ time reference doesn't get optimized away.
var _ = time.Now
