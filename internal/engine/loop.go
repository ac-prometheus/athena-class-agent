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

// ToolCallResult holds a tool name and the (stub) result.
type ToolCallResult struct {
	Name   string
	Result string
}

// LoopConfig configures the agentic loop.
type LoopConfig struct {
	MaxTurns    int
	TokenBudget int
}

// Loop runs the single-turn agentic loop for a session.
// Phase 1 stub: runs exactly one turn, no tool dispatch, no hook pipeline.
// Tool dispatch, hook pipeline, and multi-turn logic are Phase 1+ concerns.
type Loop struct {
	client pkg.LLMClient
	cfg    LoopConfig
}

// NewLoop creates an agentic loop backed by the given LLM client.
func NewLoop(client pkg.LLMClient, cfg LoopConfig) *Loop {
	return &Loop{client: client, cfg: cfg}
}

// RunOneTurn sends a single completion request and returns the turn result.
func (l *Loop) RunOneTurn(ctx context.Context, req pkg.CompletionRequest) (*Turn, error) {
	slog.Info("engine: running turn", "max_tokens", req.MaxTokens)

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

// DispatchTool is a stub — returns "tool not implemented" for every call.
// Real dispatch lands in Phase 4 (Tools & CLI).
func DispatchTool(_ context.Context, name string, _ map[string]any) (string, error) {
	return fmt.Sprintf("tool not implemented: %s", name), nil
}
