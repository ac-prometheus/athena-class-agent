package engine

import (
	"context"
	"time"
)

// TurnResult carries the output of a completed turn for post-turn hooks.
// Hooks read from this; they must not mutate it.
type TurnResult struct {
	SessionID        string
	TurnNumber       int
	Content          string
	ToolCalls        []string
	PromptTokens     int
	CompletionTokens int
	ThinkingTokens   int
	TTFT             time.Duration
	TotalDuration    time.Duration
}

// TurnHook runs after each turn completes. Hooks are registered at startup
// and run in order. Phase 2+ adds: t2-logger, budget-monitor,
// peripheral-awareness, relational-surfacing, telemetry-emitter.
type TurnHook interface {
	Name() string
	Run(ctx context.Context, turn TurnResult) error
}

// HookPipeline runs registered hooks in order after each turn.
type HookPipeline struct {
	hooks []TurnHook
}

// NewHookPipeline returns an empty pipeline.
func NewHookPipeline() *HookPipeline {
	return &HookPipeline{}
}

// Register appends a hook to the pipeline. Hooks run in registration order.
func (p *HookPipeline) Register(h TurnHook) {
	p.hooks = append(p.hooks, h)
}

// RunAll executes all registered hooks in order.
// Errors from individual hooks are returned immediately; remaining hooks are skipped.
func (p *HookPipeline) RunAll(ctx context.Context, turn TurnResult) error {
	for _, h := range p.hooks {
		if err := h.Run(ctx, turn); err != nil {
			return err
		}
	}
	return nil
}
