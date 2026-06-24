package aegis

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// mockTrustStore is an in-memory TrustStore for testing.
type mockTrustStore struct {
	mu      sync.Mutex
	scores  map[string]float64
	counts  map[string]int
}

func newMockTrustStore() *mockTrustStore {
	return &mockTrustStore{
		scores: make(map[string]float64),
		counts: make(map[string]int),
	}
}

func (m *mockTrustStore) GetTrust(_ context.Context, source string) (float64, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	score, ok := m.scores[source]
	if !ok {
		return 0, 0, ErrTrustNotFound
	}
	return score, m.counts[source], nil
}

func (m *mockTrustStore) UpdateTrust(_ context.Context, source string, score float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scores[source] = score
	return nil
}

func (m *mockTrustStore) IncrementInteractions(_ context.Context, source string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counts[source]++
	return nil
}

func TestTrustScorer_SkepticalPriorForNewSource(t *testing.T) {
	store := newMockTrustStore()
	scorer := NewTrustScorer(DefaultTrustConfig(), store)

	score, err := scorer.Score(context.Background(), "new-source.example", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// New source at interaction 0: should be exactly skeptical prior.
	cfg := DefaultTrustConfig()
	if score != cfg.SkepticalPrior {
		t.Errorf("expected skeptical prior %.2f, got %.2f", cfg.SkepticalPrior, score)
	}
}

func TestTrustScorer_RampOverInteractions(t *testing.T) {
	store := newMockTrustStore()
	cfg := DefaultTrustConfig()
	scorer := NewTrustScorer(cfg, store)
	ctx := context.Background()
	source := "ramping.example"

	// Seed a higher stored trust.
	_ = store.UpdateTrust(ctx, source, 0.90)

	// At 0 interactions, blended score should be close to skeptical prior.
	score0, _ := scorer.Score(ctx, source, nil)

	// Manually set interactions to RampInteractions - 1 and score a few more times.
	store.mu.Lock()
	store.counts[source] = cfg.RampInteractions - 1
	store.scores[source] = 0.90
	store.mu.Unlock()

	scoreN, _ := scorer.Score(ctx, source, nil)

	// scoreN should be higher than score0 (further along the ramp).
	if scoreN <= score0 {
		t.Errorf("expected score to increase with more interactions (ramp), got score0=%.3f scoreN=%.3f", score0, scoreN)
	}
}

func TestTrustScorer_InjectionFlagPenalty(t *testing.T) {
	store := newMockTrustStore()
	scorer := NewTrustScorer(DefaultTrustConfig(), store)
	ctx := context.Background()

	// Seed a high-trust source past ramp.
	source := "trusted.example"
	_ = store.UpdateTrust(ctx, source, 0.90)
	store.mu.Lock()
	store.counts[source] = 10
	store.mu.Unlock()

	flags := []PatternMatch{
		{Category: "classic", Pattern: "ignore previous"},
	}
	score, err := scorer.Score(ctx, source, flags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score >= 0.90 {
		t.Errorf("expected score to drop after injection flag, got %.3f", score)
	}
}

func TestTrustScorer_MultipleFlags_FloorAt010(t *testing.T) {
	store := newMockTrustStore()
	scorer := NewTrustScorer(DefaultTrustConfig(), store)
	ctx := context.Background()

	source := "attacker.example"
	_ = store.UpdateTrust(ctx, source, 0.90)
	store.mu.Lock()
	store.counts[source] = 10
	store.mu.Unlock()

	// Many flags should drive score to floor.
	flags := make([]PatternMatch, 20)
	for i := range flags {
		flags[i] = PatternMatch{Category: "classic", Pattern: "ignore previous"}
	}
	score, _ := scorer.Score(ctx, source, flags)
	if score < 0.10 {
		t.Errorf("score should not go below floor 0.10, got %.4f", score)
	}
	if score > 0.10+1e-9 {
		t.Errorf("expected score at floor 0.10 with many flags, got %.4f", score)
	}
}

func TestTrustScorer_MaxTrustCap(t *testing.T) {
	store := newMockTrustStore()
	cfg := DefaultTrustConfig()
	scorer := NewTrustScorer(cfg, store)
	ctx := context.Background()

	source := "capped.example"
	// Set stored score above MaxTrust.
	_ = store.UpdateTrust(ctx, source, 0.99)
	store.mu.Lock()
	store.counts[source] = 10
	store.mu.Unlock()

	score, _ := scorer.Score(ctx, source, nil)
	if score > cfg.MaxTrust+1e-9 {
		t.Errorf("expected score capped at MaxTrust %.2f, got %.4f", cfg.MaxTrust, score)
	}
}

func TestTrustScorer_ErrorFromStore(t *testing.T) {
	errStore := &errorTrustStore{}
	scorer := NewTrustScorer(DefaultTrustConfig(), errStore)
	// Should return skeptical prior and propagate error.
	score, err := scorer.Score(context.Background(), "any", nil)
	if !errors.Is(err, errFakeStore) {
		t.Errorf("expected errFakeStore, got %v", err)
	}
	if score != DefaultTrustConfig().SkepticalPrior {
		t.Errorf("expected skeptical prior on error, got %.2f", score)
	}
}

var errFakeStore = errors.New("fake store error")

type errorTrustStore struct{}

func (e *errorTrustStore) GetTrust(_ context.Context, _ string) (float64, int, error) {
	return 0, 0, errFakeStore
}
func (e *errorTrustStore) UpdateTrust(_ context.Context, _ string, _ float64) error {
	return nil
}
func (e *errorTrustStore) IncrementInteractions(_ context.Context, _ string) error {
	return nil
}
