package engine

import (
	"context"
	"log/slog"
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

// hookEntry wraps a TurnHook with its criticality flag.
// Critical hooks halt the pipeline on error; advisory hooks log and continue.
type hookEntry struct {
	hook     TurnHook
	critical bool
}

// HookPipeline runs registered hooks in order after each turn.
type HookPipeline struct {
	hooks []hookEntry
}

// NewHookPipeline returns an empty pipeline.
func NewHookPipeline() *HookPipeline {
	return &HookPipeline{}
}

// Register appends an advisory (non-critical) hook to the pipeline.
// Errors from advisory hooks are logged as warnings; execution continues.
// Hooks run in registration order.
func (p *HookPipeline) Register(h TurnHook) {
	p.hooks = append(p.hooks, hookEntry{hook: h, critical: false})
}

// RegisterCritical appends a critical hook to the pipeline.
// If a critical hook returns an error, RunAll propagates it immediately and
// skips any remaining hooks. Use for hooks whose failure should abort the
// post-turn pipeline (e.g. Tier-2 persistence).
func (p *HookPipeline) RegisterCritical(h TurnHook) {
	p.hooks = append(p.hooks, hookEntry{hook: h, critical: true})
}

// RunAll executes all registered hooks in order.
// Critical hook errors are returned immediately (remaining hooks are skipped).
// Advisory hook errors are logged as warnings and execution continues.
func (p *HookPipeline) RunAll(ctx context.Context, turn TurnResult) error {
	for _, entry := range p.hooks {
		if err := entry.hook.Run(ctx, turn); err != nil {
			if entry.critical {
				return err
			}
			slog.Warn("hook error (advisory — continuing)",
				"hook", entry.hook.Name(),
				"turn", turn.TurnNumber,
				"err", err,
			)
		}
	}
	return nil
}
