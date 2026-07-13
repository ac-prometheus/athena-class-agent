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

// RunResult carries the full conversation history and the final assistant response.
type RunResult struct {
	Messages      []pkg.Message
	FinalResponse string
	TurnsUsed     int
}

// Run executes the multi-turn agentic loop.
// systemPrompt is placed in CompletionRequest.System. budget controls MaxTokens.
// The loop exits when the model produces a response with no tool calls, when
// MaxTurns is reached, or when ctx is cancelled.
func (l *Loop) Run(ctx context.Context, systemPrompt string, budget int) (*RunResult, error) {
	if l.cfg.Rehearsal {
		slog.Info("engine: rehearsal mode — skipping agent loop")
		return &RunResult{}, nil
	}

	var messages []pkg.Message
	turnIndex := 0

	for turnIndex < l.cfg.MaxTurns {
		req := pkg.CompletionRequest{
			System:    systemPrompt,
			Messages:  messages,
			MaxTokens: budget,
		}

		slog.Info("engine: turn start", "turn", turnIndex, "messages", len(messages))

		resp, err := l.client.Complete(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("engine: LLM completion turn %d: %w", turnIndex, err)
		}

		toolCalls := resp.ToolCallBlocks()
		textContent := resp.TextContent()

		slog.Info("engine: turn complete",
			"turn", turnIndex,
			"prompt_tokens", resp.PromptTokens,
			"completion_tokens", resp.CompletionTokens,
			"tool_calls", len(toolCalls),
			"ttft_ms", resp.TTFT.Milliseconds(),
			"total_ms", resp.TotalLatency.Milliseconds(),
		)

		turnResult := TurnResult{
			TurnNumber:       turnIndex,
			Content:          textContent,
			PromptTokens:     resp.PromptTokens,
			CompletionTokens: resp.CompletionTokens,
			ThinkingTokens:   resp.ThinkingTokens,
			TTFT:             resp.TTFT,
			TotalDuration:    resp.TotalLatency,
		}
		for _, tc := range toolCalls {
			turnResult.ToolCalls = append(turnResult.ToolCalls, tc.Name)
		}

		// No tool calls → the model is done; run hooks and return.
		if len(toolCalls) == 0 {
			messages = append(messages, pkg.Message{Role: "assistant", Content: textContent})

			if l.hooks != nil {
				if err := l.hooks.RunAll(ctx, turnResult); err != nil {
					slog.Warn("engine: hook error after final turn", "err", err)
				}
			}

			return &RunResult{
				Messages:      messages,
				FinalResponse: textContent,
				TurnsUsed:     turnIndex + 1,
			}, nil
		}

		// Append assistant turn to conversation history.
		messages = append(messages, pkg.Message{Role: "assistant", Content: textContent})

		// Execute each tool call and inject results back into the conversation.
		for _, call := range toolCalls {
			result, dispErr := DispatchToolCall(ctx, l.registry, call, l.cfg.DryRun)
			if dispErr != nil {
				slog.Warn("engine: tool dispatch error",
					"tool", call.Name, "err", dispErr)
				result = fmt.Sprintf("error: %v", dispErr)
			}

			// Tool results are fed back as user messages (OpenAI-compat convention).
			toolResultContent := fmt.Sprintf("[tool_result id=%s name=%s]\n%s", call.ID, call.Name, result)
			messages = append(messages, pkg.Message{Role: "user", Content: toolResultContent})
		}

		// Run hooks after the assistant turn (not after tool-result injection).
		if l.hooks != nil {
			if err := l.hooks.RunAll(ctx, turnResult); err != nil {
				slog.Warn("engine: hook error", "turn", turnIndex, "err", err)
			}
		}

		turnIndex++
	}

	slog.Warn("engine: max turns reached", "max_turns", l.cfg.MaxTurns)
	return &RunResult{
		Messages:  messages,
		TurnsUsed: turnIndex,
	}, nil
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

