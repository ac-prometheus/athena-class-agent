package engine

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ac-prometheus/athena-class-agent/internal/awareness"
	"github.com/ac-prometheus/athena-class-agent/internal/assembly"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// T2LoggerHook writes each completed turn's content to the experiential log (Tier 2).
// Tier 2 is append-only and never deleted — this hook is the write path.
type T2LoggerHook struct {
	store pkg.MemoryStore
}

// NewT2LoggerHook creates a T2LoggerHook backed by the given store.
func NewT2LoggerHook(store pkg.MemoryStore) *T2LoggerHook {
	return &T2LoggerHook{store: store}
}

func (h *T2LoggerHook) Name() string { return "tier2-logger" }

func (h *T2LoggerHook) Run(ctx context.Context, turn TurnResult) error {
	entry := pkg.ExperientialLog{
		ID:            fmt.Sprintf("t2-%s-%d-%d", turn.SessionID, turn.TurnNumber, time.Now().UnixNano()),
		SessionID:     turn.SessionID,
		Content:       turn.Content,
		ContentSource: "self",
	}
	if err := h.store.AppendExperiential(ctx, entry); err != nil {
		return fmt.Errorf("tier2-logger: append failed (turn %d): %w", turn.TurnNumber, err)
	}
	slog.Debug("tier2-logger: turn logged", "session", turn.SessionID, "turn", turn.TurnNumber)
	return nil
}

// BudgetMonitorHook checks the session budget level after each turn and logs
// warnings at soft and hard thresholds. Hard budget is a signal to the loop
// to begin wrapping up — the hook itself does not halt the session.
type BudgetMonitorHook struct {
	budget *assembly.TokenBudget
}

// NewBudgetMonitorHook creates a BudgetMonitorHook tracking the given budget.
func NewBudgetMonitorHook(budget *assembly.TokenBudget) *BudgetMonitorHook {
	return &BudgetMonitorHook{budget: budget}
}

func (h *BudgetMonitorHook) Name() string { return "budget-monitor" }

func (h *BudgetMonitorHook) Run(ctx context.Context, turn TurnResult) error {
	level := h.budget.Add(turn.PromptTokens, turn.CompletionTokens)
	switch level {
	case assembly.BudgetSoft:
		slog.Warn("budget-monitor: soft threshold reached — agent should begin wrapping up",
			"session", turn.SessionID,
			"turn", turn.TurnNumber,
			"remaining", h.budget.Remaining(),
		)
	case assembly.BudgetHard:
		slog.Warn("budget-monitor: hard threshold reached — session should end",
			"session", turn.SessionID,
			"turn", turn.TurnNumber,
			"remaining", h.budget.Remaining(),
		)
	}
	return nil
}

// PeripheralAwarenessHook computes semantic velocity after each turn.
// When velocity exceeds the threshold (with jitter), it emits a nudge suggestion
// as a structured log entry. The agent sees the suggestion via its logging stream,
// not as an injected message — it is a hint, not a command.
type PeripheralAwarenessHook struct {
	pa       *awareness.PeripheralAwareness
	provider pkg.EmbeddingProvider
}

// NewPeripheralAwarenessHook creates the hook with the given peripheral awareness
// tracker and embedding provider.
func NewPeripheralAwarenessHook(pa *awareness.PeripheralAwareness, provider pkg.EmbeddingProvider) *PeripheralAwarenessHook {
	return &PeripheralAwarenessHook{pa: pa, provider: provider}
}

func (h *PeripheralAwarenessHook) Name() string { return "peripheral-awareness" }

func (h *PeripheralAwarenessHook) Run(ctx context.Context, turn TurnResult) error {
	if h.provider == nil {
		return nil
	}

	vec, err := h.provider.Embed(ctx, turn.Content)
	if err != nil {
		// Non-fatal: peripheral awareness is advisory, not load-bearing.
		slog.Warn("peripheral-awareness: embedding failed", "err", err, "turn", turn.TurnNumber)
		return nil
	}

	// Check for nudge before updating centroid — velocity must be against the prior state.
	nudge := h.pa.CheckNudge(turn.TurnNumber, vec, turn.ToolCalls)
	h.pa.UpdateCentroid(vec)

	if nudge != nil {
		slog.Info("peripheral-awareness: semantic drift nudge",
			"turn", nudge.Turn,
			"velocity", fmt.Sprintf("%.4f", nudge.Velocity),
			"topic", nudge.Topic,
			"suggestion", nudge.Command,
		)
	}
	return nil
}

// RetrievalUsageHook tracks which memory records were loaded at session start
// and marks which ones the agent actually referenced during the session.
// At session end (LogUsageReport), it emits a coverage report so unused retrievals
// can inform future retrieval tuning.
type RetrievalUsageHook struct {
	retrievedIDs  map[string]bool // IDs loaded at session start
	referencedIDs map[string]bool // IDs the agent actually used
}

// NewRetrievalUsageHook creates the hook with the set of record IDs that were
// loaded into the session context.
func NewRetrievalUsageHook(retrievedIDs []string) *RetrievalUsageHook {
	ids := make(map[string]bool, len(retrievedIDs))
	for _, id := range retrievedIDs {
		ids[id] = true
	}
	return &RetrievalUsageHook{
		retrievedIDs:  ids,
		referencedIDs: make(map[string]bool),
	}
}

func (h *RetrievalUsageHook) Name() string { return "retrieval-usage" }

// Run scans turn content for any of the retrieved IDs and marks matches as referenced.
func (h *RetrievalUsageHook) Run(ctx context.Context, turn TurnResult) error {
	for id := range h.retrievedIDs {
		if contains(turn.Content, id) {
			h.referencedIDs[id] = true
		}
	}
	return nil
}

// LogUsageReport emits the session-end coverage summary.
// Call this from session teardown after the last turn hook has run.
func (h *RetrievalUsageHook) LogUsageReport(sessionID string) {
	total := len(h.retrievedIDs)
	used := len(h.referencedIDs)
	unused := total - used
	slog.Info("retrieval-usage: session coverage report",
		"session", sessionID,
		"retrieved", total,
		"referenced", used,
		"unused", unused,
	)
	if unused > 0 {
		for id := range h.retrievedIDs {
			if !h.referencedIDs[id] {
				slog.Debug("retrieval-usage: unreferenced record", "id", id)
			}
		}
	}
}

// contains reports whether substr appears in s (avoiding strings import cycle).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		findInString(s, substr))
}

func findInString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
