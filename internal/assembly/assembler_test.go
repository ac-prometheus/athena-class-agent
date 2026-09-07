package assembly

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

func TestDefaultPhases_PriorityOrder(t *testing.T) {
	phases := DefaultPhases(t.TempDir())
	if len(phases) == 0 {
		t.Fatal("DefaultPhases returned empty registry")
	}

	prev := -1
	for _, p := range phases {
		pri := p.Priority()
		if pri <= prev {
			t.Errorf("phase %q priority %d is not strictly greater than previous %d", p.Name(), pri, prev)
		}
		prev = pri
	}
}

func TestDefaultPhases_KnownPhaseNames(t *testing.T) {
	phases := DefaultPhases(t.TempDir())
	names := make(map[string]bool)
	for _, p := range phases {
		names[p.Name()] = true
	}
	for _, want := range []string{"identity", "continuity", "world-model", "echo-pool", "incoming", "grounding"} {
		if !names[want] {
			t.Errorf("expected phase %q in DefaultPhases, not found", want)
		}
	}
}

func setupIdentityDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "soul.md"), []byte("I am a test agent."), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestAssemble_ManifestTracking(t *testing.T) {
	dir := setupIdentityDir(t)
	assembler := NewContextAssembler(dir, 200000)

	cfg := MinimalAssembleConfig()
	cfg.Plan = &pkg.LifecyclePlan{
		ID:        "plan-test-001",
		SessionID: "sess-test-001",
	}
	cfg.SessionID = "sess-test-001"
	cfg.SkipWitnessCheck = true

	result, err := assembler.Assemble(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	m := result.Manifest
	if m == nil {
		t.Fatal("Manifest is nil")
	}
	if m.ID == "" {
		t.Error("Manifest.ID is empty")
	}
	if m.PlanID != "plan-test-001" {
		t.Errorf("Manifest.PlanID = %q, want %q", m.PlanID, "plan-test-001")
	}
	if m.SessionID != "sess-test-001" {
		t.Errorf("Manifest.SessionID = %q, want %q", m.SessionID, "sess-test-001")
	}
	if len(m.PhasesRun) == 0 {
		t.Error("Manifest.PhasesRun is empty — expected at least identity phase")
	}
	if m.BudgetTotal <= 0 {
		t.Errorf("Manifest.BudgetTotal = %d, want > 0", m.BudgetTotal)
	}
}

func TestAssemble_NilPlan_NoPanic(t *testing.T) {
	dir := setupIdentityDir(t)
	assembler := NewContextAssembler(dir, 200000)

	cfg := MinimalAssembleConfig()
	cfg.SkipWitnessCheck = true

	result, err := assembler.Assemble(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Assemble with nil Plan failed: %v", err)
	}
	if result.Manifest.PlanID != "" {
		t.Errorf("PlanID should be empty when Plan is nil, got %q", result.Manifest.PlanID)
	}
}

func TestAssemble_BudgetEnforcement_SkipsPhase(t *testing.T) {
	dir := setupIdentityDir(t)
	// EchoPoolPhase has MinBudget=8000 chars. With a tiny total budget, it should be skipped.
	// Budget = 1 token → 4 chars total. Identity (MinBudget=0, CharsUsed=0) proceeds.
	// ContinuityPhase (MinBudget=0) proceeds. WorldModel (MinBudget=1) tries to run.
	// Echoes (MinBudget=8000) should be skipped when remaining < 8000.
	assembler := NewContextAssembler(dir, 1)

	cfg := MinimalAssembleConfig()
	cfg.SkipWitnessCheck = true

	result, err := assembler.Assemble(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	skipped := make(map[string]bool)
	for _, name := range result.Manifest.PhasesSkipped {
		skipped[name] = true
	}
	if !skipped["echo-pool"] {
		t.Error("expected 'echo-pool' phase to be skipped under tight budget")
	}
}

func TestAssemble_SystemPromptNotEmpty(t *testing.T) {
	dir := setupIdentityDir(t)
	assembler := NewContextAssembler(dir, 200000)

	cfg := MinimalAssembleConfig()
	cfg.SkipWitnessCheck = true

	result, err := assembler.Assemble(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}
	if result.SystemPrompt == "" {
		t.Error("SystemPrompt is empty")
	}
}

func TestTokenBudget_Levels(t *testing.T) {
	b := NewTokenBudget(1000, 50)

	if b.Level() != BudgetOK {
		t.Errorf("initial level = %d, want BudgetOK", b.Level())
	}
	if b.Remaining() != 1000 {
		t.Errorf("initial remaining = %d, want 1000", b.Remaining())
	}

	// Use 500 tokens (50%) — still OK.
	level := b.Add(300, 200)
	if level != BudgetOK {
		t.Errorf("at 50%% usage: level = %d, want BudgetOK", level)
	}

	// Use 310 more (total 810, 81%) — should be BudgetSoft.
	level = b.Add(200, 110)
	if level != BudgetSoft {
		t.Errorf("at 81%% usage: level = %d, want BudgetSoft", level)
	}

	// Use 150 more (total 960, 96%) — should be BudgetHard.
	level = b.Add(100, 50)
	if level != BudgetHard {
		t.Errorf("at 96%% usage: level = %d, want BudgetHard", level)
	}

	if b.Remaining() != 40 {
		t.Errorf("remaining = %d, want 40", b.Remaining())
	}
}

func TestTokenBudget_HardFloor(t *testing.T) {
	b := NewTokenBudget(1000, 100)

	// Use 890 tokens → 110 remaining, but 89% < 95% so normally BudgetSoft.
	// However remaining (110) > hardFloor (100), so still BudgetSoft.
	level := b.Add(890, 0)
	if level != BudgetSoft {
		t.Errorf("at 89%% with 110 remaining: level = %d, want BudgetSoft", level)
	}

	// Use 20 more → 910 total, 91% but only 90 remaining (< hardFloor 100) → BudgetHard.
	level = b.Add(20, 0)
	if level != BudgetHard {
		t.Errorf("at 91%% with 90 remaining (below hardFloor): level = %d, want BudgetHard", level)
	}
}

func TestTokenBudget_Remaining_NeverNegative(t *testing.T) {
	b := NewTokenBudget(100, 10)
	b.Add(200, 0)
	if b.Remaining() != 0 {
		t.Errorf("remaining = %d, want 0 (not negative)", b.Remaining())
	}
}

func TestTokenBudget_ZeroTotal(t *testing.T) {
	b := NewTokenBudget(0, 0)
	level := b.Add(100, 100)
	if level != BudgetOK {
		t.Errorf("zero-total budget: level = %d, want BudgetOK (no-op budget)", level)
	}
}

// ---------------------------------------------------------------------------
// HARN-104: findContradiction regression test — point lookup by ID
// ---------------------------------------------------------------------------

type stubMemoryStoreForEcho struct {
	reflections map[string]*pkg.Reflection
}

func (s *stubMemoryStoreForEcho) AppendExperiential(_ context.Context, _ pkg.ExperientialLog) error {
	return nil
}
func (s *stubMemoryStoreForEcho) SearchNarrative(_ context.Context, _ []float32, _ int) ([]pkg.NarrativeSummary, error) {
	return nil, nil
}
func (s *stubMemoryStoreForEcho) InsertNarrative(_ context.Context, _ pkg.NarrativeSummary) error {
	return nil
}
func (s *stubMemoryStoreForEcho) SearchReflections(_ context.Context, _ []float32, limit int) ([]pkg.Reflection, error) {
	// Only return up to limit most recent — simulates the old bug
	var out []pkg.Reflection
	for _, r := range s.reflections {
		out = append(out, *r)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
func (s *stubMemoryStoreForEcho) GetReflectionByID(_ context.Context, id string) (*pkg.Reflection, error) {
	r, ok := s.reflections[id]
	if !ok {
		return nil, nil
	}
	return r, nil
}
func (s *stubMemoryStoreForEcho) InsertReflection(_ context.Context, ref pkg.Reflection) error {
	s.reflections[ref.ID] = &ref
	return nil
}
func (s *stubMemoryStoreForEcho) SearchEntities(_ context.Context, _ string, _ int) ([]pkg.Entity, error) {
	return nil, nil
}
func (s *stubMemoryStoreForEcho) UpsertEntity(_ context.Context, _ pkg.Entity) error { return nil }
func (s *stubMemoryStoreForEcho) GetProfile(_ context.Context, _ string) (*pkg.RelationalProfile, error) {
	return nil, nil
}
func (s *stubMemoryStoreForEcho) ListProfiles(_ context.Context) ([]pkg.RelationalProfile, error) {
	return nil, nil
}
func (s *stubMemoryStoreForEcho) Close() error { return nil }

type stubEdgeStoreForEcho struct {
	edges map[string][]pkg.MemoryEdge
}

func (s *stubEdgeStoreForEcho) CreateEdge(_ context.Context, fromID, toID string, fromTier, toTier int, edgeType, author string) error {
	s.edges[fromID] = append(s.edges[fromID], pkg.MemoryEdge{
		FromID: fromID, ToID: toID, FromTier: fromTier, ToTier: toTier, EdgeType: edgeType, Author: author,
	})
	return nil
}
func (s *stubEdgeStoreForEcho) GetEdges(_ context.Context, recordID, direction string) ([]pkg.MemoryEdge, error) {
	return s.edges[recordID], nil
}
func (s *stubEdgeStoreForEcho) FetchDownstreamEdges(_ context.Context, _ string) ([]pkg.EdgeNode, error) {
	return nil, nil
}

func TestFindContradiction_OldReflectionReachable(t *testing.T) {
	// Create 30 reflections — target is the oldest (index 0)
	store := &stubMemoryStoreForEcho{reflections: make(map[string]*pkg.Reflection)}
	for i := 0; i < 30; i++ {
		id := fmt.Sprintf("ref-%03d", i)
		store.reflections[id] = &pkg.Reflection{
			ID:      id,
			Content: fmt.Sprintf("reflection content %d", i),
		}
	}

	// The oldest reflection is the contradiction target
	targetID := "ref-000"

	// Create an edge: echo source → contradicts → oldest reflection
	edges := &stubEdgeStoreForEcho{edges: make(map[string][]pkg.MemoryEdge)}
	edges.edges["echo-source"] = []pkg.MemoryEdge{
		{FromID: "echo-source", ToID: targetID, EdgeType: "contradicts"},
	}

	cfg := &AssembleConfig{
		store: store,
		edges: edges,
	}

	id, content, found := findContradiction(context.Background(), cfg, []string{"echo-source"})
	if !found {
		t.Fatal("findContradiction did not find the oldest reflection — HARN-104 regression")
	}
	if id != targetID {
		t.Errorf("found ID = %q, want %q", id, targetID)
	}
	if content != "reflection content 0" {
		t.Errorf("found content = %q, want %q", content, "reflection content 0")
	}
}

func TestFindContradiction_MissingTarget(t *testing.T) {
	store := &stubMemoryStoreForEcho{reflections: make(map[string]*pkg.Reflection)}
	store.reflections["ref-001"] = &pkg.Reflection{ID: "ref-001", Content: "exists"}

	edges := &stubEdgeStoreForEcho{edges: make(map[string][]pkg.MemoryEdge)}
	edges.edges["echo-source"] = []pkg.MemoryEdge{
		{FromID: "echo-source", ToID: "ref-nonexistent", EdgeType: "contradicts"},
	}

	cfg := &AssembleConfig{
		store: store,
		edges: edges,
	}

	_, _, found := findContradiction(context.Background(), cfg, []string{"echo-source"})
	if found {
		t.Error("findContradiction should return false for nonexistent target")
	}
}
