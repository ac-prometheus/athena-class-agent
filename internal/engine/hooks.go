package engine

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// TurnResult carries the output of a completed turn for post-turn hooks.
// Hooks read from this; they must not mutate it.
type TurnResult struct {
	SessionID        string
	TurnNumber       int
	Content          string
	ToolCalls        []string
	// ToolResults carries the outputs of tool calls executed this turn.
	// Non-nil when the turn had tool calls. Used by T2LoggerHook to write
	// tool provenance entries to T2 on tool-only turns (C-2 fix, WP-C3).
	ToolResults      []pkg.ToolResult
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
// mu guards the hooks slice against concurrent Register and RunAll calls
// (HARN-4: tool calls execute in parallel, and both the registration path and
// the dispatch path must be race-free even if one session's hooks are somehow
// shared — e.g. across test goroutines).
type HookPipeline struct {
	mu    sync.Mutex
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
	p.mu.Lock()
	defer p.mu.Unlock()
	p.hooks = append(p.hooks, hookEntry{hook: h, critical: false})
}

// RegisterCritical appends a critical hook to the pipeline.
// If a critical hook returns an error, RunAll propagates it immediately and
// skips any remaining hooks. Use for hooks whose failure should abort the
// post-turn pipeline (e.g. Tier-2 persistence).
func (p *HookPipeline) RegisterCritical(h TurnHook) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.hooks = append(p.hooks, hookEntry{hook: h, critical: true})
}

// RunAll executes all registered hooks in order.
// Critical hook errors are returned immediately (remaining hooks are skipped).
// Advisory hook errors are logged as warnings and execution continues.
func (p *HookPipeline) RunAll(ctx context.Context, turn TurnResult) error {
	p.mu.Lock()
	entries := make([]hookEntry, len(p.hooks))
	copy(entries, p.hooks)
	p.mu.Unlock()

	for _, entry := range entries {
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

// FuncHook adapts a plain function into a TurnHook for inline registration.
// Useful for lightweight per-session hooks (e.g. sess.RecordTurn callbacks)
// that don't warrant a named struct.
type FuncHook struct {
	name string
	fn   func(ctx context.Context, turn TurnResult) error
}

// NewFuncHook creates a TurnHook backed by fn.
func NewFuncHook(name string, fn func(ctx context.Context, turn TurnResult) error) *FuncHook {
	return &FuncHook{name: name, fn: fn}
}

func (h *FuncHook) Name() string                                    { return h.name }
func (h *FuncHook) Run(ctx context.Context, turn TurnResult) error { return h.fn(ctx, turn) }
