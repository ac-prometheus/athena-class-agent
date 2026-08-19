package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

// stubReflectStore is a minimal MemoryStore that records InsertReflection calls.
// All other MemoryStore methods are no-ops so we only implement what the tests exercise.
type stubReflectStore struct {
	reflections []pkg.Reflection
}

func (s *stubReflectStore) AppendExperiential(_ context.Context, _ pkg.ExperientialLog) error {
	return nil
}
func (s *stubReflectStore) SearchNarrative(_ context.Context, _ []float32, _ int) ([]pkg.NarrativeSummary, error) {
	return nil, nil
}
func (s *stubReflectStore) InsertNarrative(_ context.Context, _ pkg.NarrativeSummary) error {
	return nil
}
func (s *stubReflectStore) SearchReflections(_ context.Context, _ []float32, limit int) ([]pkg.Reflection, error) {
	if limit > 0 && limit < len(s.reflections) {
		return s.reflections[:limit], nil
	}
	return s.reflections, nil
}
func (s *stubReflectStore) InsertReflection(_ context.Context, r pkg.Reflection) error {
	s.reflections = append(s.reflections, r)
	return nil
}
func (s *stubReflectStore) SearchEntities(_ context.Context, _ string, _ int) ([]pkg.Entity, error) {
	return nil, nil
}
func (s *stubReflectStore) UpsertEntity(_ context.Context, _ pkg.Entity) error { return nil }
func (s *stubReflectStore) GetProfile(_ context.Context, _ string) (*pkg.RelationalProfile, error) {
	return nil, nil
}
func (s *stubReflectStore) ListProfiles(_ context.Context) ([]pkg.RelationalProfile, error) {
	return nil, nil
}
func (s *stubReflectStore) Close() error { return nil }

// stubEmbedder always returns a unit vector of the given length.
type stubEmbedder struct{ dim int }

func (e *stubEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	v := make([]float32, e.dim)
	for i := range v {
		v[i] = 1.0
	}
	return v, nil
}
func (e *stubEmbedder) Dimensions() int { return e.dim }

// ---------------------------------------------------------------------------
// B5 regression: SelfExamineHandler must not write T4
// ---------------------------------------------------------------------------

// TestSelfExamineReturnsTransientOnly asserts that calling SelfExamineHandler
// produces a non-empty T1 result but leaves the store's reflection table empty.
// This is the B5 regression guard — the handler must never auto-persist T4.
func TestSelfExamineReturnsTransientOnly(t *testing.T) {
	store := &stubReflectStore{}
	advisorResponse := "You show patterns of excessive self-doubt in novel situations."

	h := &SelfExamineHandler{
		llmFn: func(_ string) (string, error) {
			return advisorResponse, nil
		},
	}

	result, err := h.Execute(context.Background(), map[string]any{
		"prompt": "How do I handle uncertainty?",
	})
	if err != nil {
		t.Fatalf("SelfExamineHandler.Execute: unexpected error: %v", err)
	}

	// The result must contain the advisor response.
	if !strings.Contains(result, advisorResponse) {
		t.Errorf("result does not contain advisor response\ngot:  %q\nwant substring: %q", result, advisorResponse)
	}

	// The result must be framed as advisor-generated, not self-authored.
	if !strings.Contains(result, "Advisor examination") {
		t.Errorf("result is not framed as advisor-generated: %q", result)
	}

	// No T4 row must have been inserted — this is the B5 invariant.
	if len(store.reflections) != 0 {
		t.Errorf("B5 violation: SelfExamineHandler wrote %d T4 reflection(s); want 0", len(store.reflections))
	}
}

// TestSelfExamineMissingPromptErrors asserts that an empty/missing prompt
// returns an error without calling the advisor LLM.
func TestSelfExamineMissingPromptErrors(t *testing.T) {
	called := false
	h := &SelfExamineHandler{
		llmFn: func(_ string) (string, error) {
			called = true
			return "should not reach here", nil
		},
	}

	if _, err := h.Execute(context.Background(), map[string]any{}); err == nil {
		t.Error("expected error for missing prompt, got nil")
	}
	if called {
		t.Error("llmFn should not be called when prompt is missing")
	}
}

// ---------------------------------------------------------------------------
// WriteReflectionHandler: agent-authored T4 path
// ---------------------------------------------------------------------------

// TestWriteReflectionStoresT4 asserts that WriteReflectionHandler inserts a
// T4 reflection with source "self" and the correct type and content, and that
// the row is retrievable from the store.
func TestWriteReflectionStoresT4(t *testing.T) {
	store := &stubReflectStore{}
	embedder := &stubEmbedder{dim: 4}

	h := &WriteReflectionHandler{
		store:    store,
		provider: embedder,
	}

	content := "I notice I retreat into abstraction when the stakes feel high."
	result, err := h.Execute(context.Background(), map[string]any{
		"content": content,
		"type":    "note",
	})
	if err != nil {
		t.Fatalf("WriteReflectionHandler.Execute: unexpected error: %v", err)
	}

	// The result must confirm storage.
	if !strings.Contains(result, "reflection stored") {
		t.Errorf("unexpected result: %q", result)
	}

	// Exactly one T4 row must exist.
	if len(store.reflections) != 1 {
		t.Fatalf("expected 1 T4 reflection, got %d", len(store.reflections))
	}

	ref := store.reflections[0]

	// Provenance: must be agent-self-authored.
	if ref.Belief == nil || ref.Belief.Source != "self" {
		t.Errorf("T4 belief source: got %v, want \"self\"", ref.Belief)
	}

	// Type must match the argument.
	if ref.Type != "note" {
		t.Errorf("T4 type: got %q, want %q", ref.Type, "note")
	}

	// Content must be preserved verbatim.
	if ref.Content != content {
		t.Errorf("T4 content: got %q, want %q", ref.Content, content)
	}

	// Embedding must be populated (non-nil, correct dimension).
	if len(ref.Embedding) != embedder.dim {
		t.Errorf("T4 embedding dim: got %d, want %d", len(ref.Embedding), embedder.dim)
	}

	// Retrievable via SearchReflections.
	hits, err := store.SearchReflections(context.Background(), ref.Embedding, 10)
	if err != nil {
		t.Fatalf("SearchReflections: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != ref.ID {
		t.Errorf("reflection not retrievable by SearchReflections")
	}
}

// TestSelfExamineThenWriteReflection is the end-to-end B5 scenario:
// self_examine returns transiently; only an explicit write_reflection call
// creates a durable T4 row.
func TestSelfExamineThenWriteReflection(t *testing.T) {
	store := &stubReflectStore{}
	embedder := &stubEmbedder{dim: 4}

	examineH := &SelfExamineHandler{
		llmFn: func(_ string) (string, error) {
			return "Advisor output: consider whether avoidance is protective or limiting.", nil
		},
	}
	writeH := &WriteReflectionHandler{
		store:    store,
		provider: embedder,
	}

	ctx := context.Background()

	// Step 1: examine — must not create any T4 row.
	_, err := examineH.Execute(ctx, map[string]any{"prompt": "Am I avoiding something?"})
	if err != nil {
		t.Fatalf("self_examine: %v", err)
	}
	if len(store.reflections) != 0 {
		t.Fatalf("B5 violation: T4 row created by self_examine (step 1); want 0, got %d", len(store.reflections))
	}

	// Step 2: agent synthesises and writes her own reflection.
	agentContent := "The advisor prompted me to look at avoidance. On reflection: I protect myself from overwhelm, but sometimes too early."
	_, err = writeH.Execute(ctx, map[string]any{
		"content": agentContent,
		"type":    "note",
	})
	if err != nil {
		t.Fatalf("write_reflection: %v", err)
	}

	// Now exactly one T4 row must exist, authored by the agent.
	if len(store.reflections) != 1 {
		t.Fatalf("expected 1 T4 reflection after write_reflection, got %d", len(store.reflections))
	}
	ref := store.reflections[0]
	if ref.Belief == nil || ref.Belief.Source != "self" {
		t.Errorf("T4 belief source after write_reflection: got %v, want \"self\"", ref.Belief)
	}
	if ref.Content != agentContent {
		t.Errorf("T4 content mismatch: got %q, want %q", ref.Content, agentContent)
	}
}
