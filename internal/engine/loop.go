package engine

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// Turn represents a single completed agentic turn.
type Turn struct {
	Index    int
	Request  pkg.CompletionRequest
	Response *pkg.CompletionResponse
	ToolCall *ToolCallResult // non-nil if the model requested a tool
}

// ToolCallResult holds a tool name and the result string.
type ToolCallResult struct {
	Name   string
	Result string
}

// LoopConfig configures the agentic loop.
//
// DryRun suppresses tool execution (handlers are not called) but hooks still
// run — this allows testing the hook pipeline (T2 logging, budget monitoring,
// peripheral awareness) without real tool side effects. For full side-effect
// suppression (no hooks, no tool calls), use Rehearsal mode, which returns
// before the loop starts.
type LoopConfig struct {
	MaxTurns    int  // default 50 if zero
	TokenBudget int
	DryRun      bool // log tool calls but don't execute; hooks still run
	Rehearsal   bool // run orientation only, no agent loop
}

// Loop drives the multi-turn agentic loop for a session.
type Loop struct {
	client   pkg.LLMClient
	registry pkg.ToolRegistry
	hooks    *HookPipeline
	cfg      LoopConfig
}

// NewLoop creates an agentic loop backed by the given LLM client, registry, and hook pipeline.
// If cfg.MaxTurns is zero it defaults to 50.
func NewLoop(client pkg.LLMClient, registry pkg.ToolRegistry, hooks *HookPipeline, cfg LoopConfig) *Loop {
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = 50
	}
	return &Loop{
		client:   client,
		registry: registry,
		hooks:    hooks,
		cfg:      cfg,
	}
}

// RunOneTurn sends a single completion request and returns the turn result.
// Preserved for callers that want single-shot behaviour without the full loop.
func (l *Loop) RunOneTurn(ctx context.Context, req pkg.CompletionRequest) (*Turn, error) {
	slog.Info("engine: running single turn", "max_tokens", req.MaxTokens)

	resp, err := l.client.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM completion: %w", err)
	}

	slog.Info("engine: turn complete",
		"prompt_tokens", resp.PromptTokens,
		"completion_tokens", resp.CompletionTokens,
		"ttft_ms", resp.TTFT.Milliseconds(),
		"total_ms", resp.TotalLatency.Milliseconds(),
	)

	return &Turn{
		Index:    0,
		Request:  req,
		Response: resp,
	}, nil
}

