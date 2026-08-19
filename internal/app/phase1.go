package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ac-prometheus/athena-class-agent/internal/assembly"
	"github.com/ac-prometheus/athena-class-agent/internal/engine"
	"github.com/ac-prometheus/athena-class-agent/internal/session"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// Phase1Runner implements the legacy single-turn connectivity runner.
// Kept as a fallback gated by PHASE1_MODE=true.
type Phase1Runner struct {
	deps      *Dependencies
	assembler *assembly.ContextAssembler
}

// NewPhase1Runner creates a Phase 1 compatibility runner.
func NewPhase1Runner(deps *Dependencies, assembler *assembly.ContextAssembler) *Phase1Runner {
	return &Phase1Runner{deps: deps, assembler: assembler}
}

// RunSession executes a single connectivity-test turn.
// ctx is propagated from the daemon so external cancellation terminates cleanly.
func (r *Phase1Runner) RunSession(ctx context.Context, trigger pkg.SessionTrigger) error {
	cfg := assembly.MinimalAssembleConfig()
	cfg.InbandNotes = trigger.InbandNotes
	assembled, err := r.assembler.Assemble(ctx, cfg)
	if err != nil {
		return fmt.Errorf("assembling context: %w", err)
	}

	sess := session.NewSession(r.deps.Config.AgentName, trigger.WakeReason)
	budget := assembly.NewTokenBudget(r.deps.Config.TokenBudget, r.deps.Config.HardFloorTokens)
	hooks := engine.NewHookPipeline()

	loop := engine.NewLoop(r.deps.LLM, nil, hooks, engine.LoopConfig{
		MaxTurns:    1,
		TokenBudget: r.deps.Config.TokenBudget,
	})

	req := pkg.CompletionRequest{
		System: assembled.SystemPrompt,
		Messages: []pkg.Message{
			{Role: "user", Content: "Hello. This is a Phase 1 connectivity test."},
		},
		MaxTokens: 256,
	}

	turn, err := loop.RunOneTurn(ctx, req)
	if err != nil {
		return fmt.Errorf("running turn: %w", err)
	}

	sess.RecordTurn(turn.Response.PromptTokens, turn.Response.CompletionTokens)
	level := budget.Add(turn.Response.PromptTokens, turn.Response.CompletionTokens)

	slog.Info("agent: turn result",
		"content", turn.Response.TextContent(),
		"budget_level", level,
		"tokens_remaining", budget.Remaining(),
	)

	hookErr := hooks.RunAll(ctx, engine.TurnResult{
		SessionID:        sess.GetID(),
		TurnNumber:       sess.TurnCount(),
		Content:          turn.Response.TextContent(),
		PromptTokens:     turn.Response.PromptTokens,
		CompletionTokens: turn.Response.CompletionTokens,
		ThinkingTokens:   turn.Response.ThinkingTokens,
		TTFT:             turn.Response.TTFT,
		TotalDuration:    turn.Response.TotalLatency,
	})
	if hookErr != nil {
		return fmt.Errorf("post-turn hooks: %w", hookErr)
	}

	if err := sess.End(); err != nil {
		return fmt.Errorf("session end: %w", err)
	}
	return nil
}
