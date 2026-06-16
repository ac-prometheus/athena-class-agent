package engine

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ac-prometheus/athena-class-agent/internal/awareness"
	"github.com/ac-prometheus/athena-class-agent/internal/harness"
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
	budget *harness.TokenBudget
}

// NewBudgetMonitorHook creates a BudgetMonitorHook tracking the given budget.
func NewBudgetMonitorHook(budget *harness.TokenBudget) *BudgetMonitorHook {
	return &BudgetMonitorHook{budget: budget}
}

func (h *BudgetMonitorHook) Name() string { return "budget-monitor" }

func (h *BudgetMonitorHook) Run(ctx context.Context, turn TurnResult) error {
	level := h.budget.Add(turn.PromptTokens, turn.CompletionTokens)
	switch level {
	case harness.BudgetSoft:
		slog.Warn("budget-monitor: soft threshold reached — agent should begin wrapping up",
			"session", turn.SessionID,
			"turn", turn.TurnNumber,
			"remaining", h.budget.Remaining(),
		)
	case harness.BudgetHard:
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
