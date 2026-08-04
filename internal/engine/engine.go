package engine

// Engine is the MOP Phase 3 agentic loop. Parallel tool dispatch, Aegis hook
// integration, FinishReason handling, and role:"tool" message threading.
//
// Five autonomy invariants (never relax these):
//  1. Never block on annotation alone — AfterToolCall is annotate-only.
//  2. Flag penalties don't corrupt stored trust.
//  3. Outbound review never blocks.
//  4. Inbound scan never fails silently — fall back to skeptical prior.
//  5. Trust ramp prevents cold-start over-trust.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

const maxToolIterationsDefault = 25
const toolTimeoutDefault = 2 * time.Minute

// HookResult is the return value of a BeforeToolCall hook.
// Block=true causes the tool call to be skipped with an error result.
type HookResult struct {
	Block  bool
	Reason string
}

// EngineConfig configures the Engine's tool execution behaviour.
type EngineConfig struct {
	MaxIterations int           // 0 → maxToolIterationsDefault
	ParallelTools bool          // true = parallel tool execution (default)
	ToolTimeout   time.Duration // per-tool; 0 → toolTimeoutDefault
	DryRun        bool          // skip tool execution; hooks still run
	Rehearsal     bool          // return immediately without starting the loop

	// BeforeToolCall is called before each tool execution. Returning a HookResult
	// with Block=true prevents execution and returns an error result. Nil = no-op.
	BeforeToolCall func(ctx context.Context, tc pkg.ToolCall, args map[string]any) (*HookResult, error)

	// AfterToolCall is called after each tool execution. The returned *ToolResult
	// replaces the original; returning nil preserves it. Never blocks per invariant 1/3.
	AfterToolCall func(ctx context.Context, tc pkg.ToolCall, result *pkg.ToolResult) (*pkg.ToolResult, error)

	// ShouldStop is called after tool results are collected. Return true to end the
	// loop early (e.g. budget exhausted). Nil = no-op.
	ShouldStop func(resp *pkg.CompletionResponse, results []pkg.ToolResult) bool
}

// LoopResult is the output of Engine.RunLoop.
type LoopResult struct {
	FinalResponse *pkg.CompletionResponse
	History       []pkg.Message
	Iterations    int
	Terminated    bool // a tool signaled Terminate
}

// Engine drives the multi-turn agentic loop with parallel tool execution and
// Aegis hook integration. It coexists with the legacy Loop — Phase 4 removes Loop.
type Engine struct {
	client   pkg.LLMClient
	registry pkg.ToolRegistry
	hooks    *HookPipeline
	aegis    pkg.ContentGateway
}

// NewEngine creates an Engine.
func NewEngine(client pkg.LLMClient, registry pkg.ToolRegistry, hooks *HookPipeline) *Engine {
	return &Engine{
		client:   client,
		registry: registry,
		hooks:    hooks,
	}
}

// WithAegis sets the Aegis content gateway for inbound/outbound screening.
func (e *Engine) WithAegis(gw pkg.ContentGateway) {
	e.aegis = gw
}

// RunLoop executes the multi-turn agentic loop over req.
// It appends assistant and tool-result messages to history and returns when
// the model produces no tool calls, a tool signals Terminate, MaxIterations is
// reached, or ctx is cancelled.
func (e *Engine) RunLoop(ctx context.Context, req pkg.CompletionRequest, cfg EngineConfig) (*LoopResult, error) {
	if cfg.Rehearsal {
		slog.Info("engine: rehearsal mode — skipping agent loop")
		return &LoopResult{}, nil
	}

	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = maxToolIterationsDefault
	}
	if cfg.ToolTimeout <= 0 {
		cfg.ToolTimeout = toolTimeoutDefault
	}

	// Auto-wire Aegis gateway into hook slots when the caller hasn't set them.
	if e.aegis != nil {
		if cfg.BeforeToolCall == nil {
			gw := e.aegis
			cfg.BeforeToolCall = func(ctx context.Context, tc pkg.ToolCall, args map[string]any) (*HookResult, error) {
				argsJSON, err := json.Marshal(args)
				if err != nil {
					return &HookResult{Block: true, Reason: fmt.Sprintf("aegis: failed to marshal args: %v", err)}, nil
				}
				annotated, err := gw.ProcessInbound(ctx, argsJSON, tc.Name, "tool_args")
				if err != nil {
					return &HookResult{Block: true, Reason: fmt.Sprintf("aegis: inbound scan error: %v", err)}, nil
				}
				if !annotated.Annotation.ScanPassed {
					// Inbound screening blocks (Invariant 4). Outbound annotation does not (Invariant 3).
					return &HookResult{
						Block:  true,
						Reason: fmt.Sprintf("aegis: inbound scan failed — flags: %v, trust: %.2f", annotated.Annotation.Flags, annotated.Annotation.TrustScore),
					}, nil
				}
				return nil, nil
			}
		}
		if cfg.AfterToolCall == nil {
			gw := e.aegis
			cfg.AfterToolCall = func(ctx context.Context, tc pkg.ToolCall, result *pkg.ToolResult) (*pkg.ToolResult, error) {
				report, err := gw.ReviewOutbound(ctx, result.Content)
				if err != nil {
					slog.Warn("engine: aegis outbound review error", "tool", tc.Name, "err", err)
					return nil, nil // don't block on review errors (invariant 3)
				}
				if !report.Clean {
					slog.Warn("engine: aegis outbound review findings",
						"tool", tc.Name, "findings", report.Findings)
					// Annotate result with findings but don't mutate content.
					annotated := *result
					annotated.Content = fmt.Sprintf("[aegis: outbound findings: %v]\n%s",
						report.Findings, result.Content)
					return &annotated, nil
				}
				return nil, nil
			}
		}
	}

	history := make([]pkg.Message, len(req.Messages))
	copy(history, req.Messages)

	var iterations int

	for {
		iterations++

		currentReq := pkg.CompletionRequest{
			System:      req.System,
			Messages:    history,
			Tools:       req.Tools,
			MaxTokens:   req.MaxTokens,
			Temperature: req.Temperature,
		}

		slog.Info("engine: iteration start", "iteration", iterations, "messages", len(history))

		resp, err := e.client.Complete(ctx, currentReq)
		if err != nil {
			return nil, fmt.Errorf("engine: iteration %d: %w", iterations, err)
		}

		slog.Info("engine: iteration complete",
			"iteration", iterations,
			"finish_reason", resp.FinishReason,
			"prompt_tokens", resp.PromptTokens,
			"completion_tokens", resp.CompletionTokens,
			"ttft_ms", resp.TTFT.Milliseconds(),
			"total_ms", resp.TotalLatency.Milliseconds(),
		)

		// Append assistant message with full block structure.
		history = append(history, pkg.Message{
			Role:    "assistant",
			Content: resp.TextContent(),
			Blocks:  resp.Blocks,
		})

		toolCalls := resp.ToolCallBlocks()

		// Truncated response — treat every tool call as an error and retry.
		if resp.FinishReason == "length" && len(toolCalls) > 0 {
			for _, tc := range toolCalls {
				history = append(history, pkg.Message{
					Role:       "tool",
					Content:    "Error: response was truncated. Re-issue your tool call with shorter arguments.",
					ToolCallID: tc.ID,
					IsError:    true,
				})
			}
			if iterations >= cfg.MaxIterations {
				return &LoopResult{FinalResponse: resp, History: history, Iterations: iterations}, nil
			}
			continue
		}

		// No tool calls → loop complete.
		if len(toolCalls) == 0 {
			// Run turn hooks on the final turn.
			if e.hooks != nil {
				tr := e.buildTurnResult(iterations, resp)
				if err := e.hooks.RunAll(ctx, tr); err != nil {
					slog.Warn("engine: hook error on final turn", "err", err)
				}
			}
			return &LoopResult{FinalResponse: resp, History: history, Iterations: iterations}, nil
		}

		// Execute tool calls.
		results, terminated := e.executeToolCalls(ctx, toolCalls, cfg)

		// Append tool results as role:"tool" messages.
		for _, r := range results {
			history = append(history, pkg.Message{
				Role:       "tool",
				Content:    r.Content,
				ToolCallID: r.CallID,
				IsError:    r.IsError,
			})
		}

		// Run turn hooks.
		if e.hooks != nil {
			tr := e.buildTurnResult(iterations, resp)
			for _, r := range results {
				tr.ToolCalls = append(tr.ToolCalls, r.CallID)
			}
			if err := e.hooks.RunAll(ctx, tr); err != nil {
				slog.Warn("engine: hook error", "iteration", iterations, "err", err)
			}
		}

		if terminated {
			return &LoopResult{
				FinalResponse: resp,
				History:       history,
				Iterations:    iterations,
				Terminated:    true,
			}, nil
		}

		if cfg.ShouldStop != nil && cfg.ShouldStop(resp, results) {
			return &LoopResult{FinalResponse: resp, History: history, Iterations: iterations}, nil
		}

		if iterations >= cfg.MaxIterations {
			slog.Warn("engine: max iterations reached", "max_iterations", cfg.MaxIterations)
			return &LoopResult{FinalResponse: resp, History: history, Iterations: iterations}, nil
		}
	}
}

// executeToolCalls runs all tool calls in the batch, in parallel unless forced sequential.
// Returns collected results and whether ALL results signalled Terminate (Pi pattern).
func (e *Engine) executeToolCalls(ctx context.Context, calls []pkg.ToolCall, cfg EngineConfig) ([]pkg.ToolResult, bool) {
	results := make([]pkg.ToolResult, len(calls))

	// Pi pattern: if ANY tool in the batch declares sequential, run the whole batch sequentially.
	forceSequential := false
	for _, call := range calls {
		if meta, ok := e.registry.GetMeta(call.Name); ok && meta.ExecMode == pkg.ExecSequential {
			forceSequential = true
			break
		}
	}

	if cfg.ParallelTools && !forceSequential && len(calls) > 1 {
		var wg sync.WaitGroup
		var mu sync.Mutex
		for i, call := range calls {
			wg.Add(1)
			go func(idx int, tc pkg.ToolCall) {
				defer wg.Done()
				r := e.executeSingleTool(ctx, tc, cfg)
				mu.Lock()
				results[idx] = r
				mu.Unlock()
			}(i, call)
		}
		wg.Wait()
	} else {
		for i, call := range calls {
			results[i] = e.executeSingleTool(ctx, call, cfg)
			if results[i].Terminate {
				// Sequential: stop on first terminate signal. Slice to filled entries
				// only — zero-value slots produce blank ToolCallID messages that
				// providers reject.
				results = results[:i+1]
				break
			}
		}
	}

	// Pi pattern: terminate only when ALL collected results signal Terminate.
	terminated := len(results) > 0
	for _, r := range results {
		if !r.Terminate {
			terminated = false
			break
		}
	}

	return results, terminated
}

// executeSingleTool runs one tool call through the full pipeline:
// unknown-check → arg parse → BeforeToolCall → execute → AfterToolCall.
func (e *Engine) executeSingleTool(ctx context.Context, tc pkg.ToolCall, cfg EngineConfig) pkg.ToolResult {
	result := pkg.ToolResult{CallID: tc.ID}

	// 1. Unknown tool → immediate error.
	handler, ok := e.registry.Get(tc.Name)
	if !ok {
		result.Content = fmt.Sprintf("Error: unknown tool %q. Available: %s", tc.Name, e.availableToolNames())
		result.IsError = true
		return result
	}

	// 2. Parse arguments.
	var args map[string]any
	if tc.Arguments != "" {
		if len(tc.Arguments) > 1<<20 {
			result.Content = fmt.Sprintf("Error: arguments for %q exceed 1MB — rejecting", tc.Name)
			result.IsError = true
			return result
		}
		if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
			result.Content = fmt.Sprintf("Error: malformed JSON arguments for %q: %v", tc.Name, err)
			result.IsError = true
			return result
		}
	}
	if args == nil {
		args = make(map[string]any)
	}

	// 3. BeforeToolCall hook (Aegis inbound scan or custom).
	// Invariant 4: inbound scan must not fail silently. On hook error, block
	// the tool call and return the error to the model as a tool result.
	if cfg.BeforeToolCall != nil {
		hookResult, err := cfg.BeforeToolCall(ctx, tc, args)
		if err != nil {
			slog.Warn("engine: BeforeToolCall hook error — blocking execution", "tool", tc.Name, "err", err)
			result.Content = fmt.Sprintf("Error: pre-execution check failed: %v", err)
			result.IsError = true
			return result
		}
		if hookResult != nil && hookResult.Block {
			result.Content = fmt.Sprintf("Error: %s", hookResult.Reason)
			result.IsError = true
			return result
		}
	}

	// 4. DryRun: skip execution but let hooks still run (already called above).
	if cfg.DryRun {
		result.Content = fmt.Sprintf("[dry-run] %s skipped", tc.Name)
		return result
	}

	// 5. Execute with per-tool timeout.
	toolCtx, cancel := context.WithTimeout(ctx, cfg.ToolTimeout)
	defer cancel()

	// V2 handler first, fall back to V1.
	var execErr error
	if v2, ok2 := handler.(pkg.ToolHandlerV2); ok2 {
		v2result, err := v2.ExecuteV2(toolCtx, args)
		if err != nil {
			execErr = err
		} else if v2result != nil {
			result = *v2result
			result.CallID = tc.ID // preserve CallID
		}
	} else {
		output, err := handler.Execute(toolCtx, args)
		if err != nil {
			execErr = err
		} else {
			result.Content = output
		}
	}

	if execErr != nil {
		if toolCtx.Err() == context.DeadlineExceeded {
			result.Content = fmt.Sprintf("Error: tool %q timed out after %v", tc.Name, cfg.ToolTimeout)
		} else {
			result.Content = fmt.Sprintf("Error: tool %q failed: %v", tc.Name, execErr)
		}
		result.IsError = true
		return result
	}

	// 6. AfterToolCall hook — annotate-only, never blocks (invariants 1 & 3).
	// Protect the Terminate flag: hooks must not be able to flip it.
	if cfg.AfterToolCall != nil {
		originalTerminate := result.Terminate
		mutated, err := cfg.AfterToolCall(ctx, tc, &result)
		if err != nil {
			slog.Warn("engine: AfterToolCall hook error", "tool", tc.Name, "err", err)
		} else if mutated != nil {
			mutated.Terminate = originalTerminate
			result = *mutated
		}
	}

	return result
}

func (e *Engine) availableToolNames() string {
	groups := e.registry.List()
	var names []string
	for _, g := range groups {
		for _, t := range g.Tools {
			names = append(names, t.Name)
		}
	}
	return strings.Join(names, ", ")
}

func (e *Engine) buildTurnResult(turnNumber int, resp *pkg.CompletionResponse) TurnResult {
	return TurnResult{
		TurnNumber:       turnNumber,
		Content:          resp.TextContent(),
		PromptTokens:     resp.PromptTokens,
		CompletionTokens: resp.CompletionTokens,
		ThinkingTokens:   resp.ThinkingTokens,
		TTFT:             resp.TTFT,
		TotalDuration:    resp.TotalLatency,
	}
}
