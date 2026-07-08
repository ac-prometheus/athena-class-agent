package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// DispatchToolCall unmarshals the tool call arguments, looks up the handler, and
// executes it. If dryRun is true the call is logged but not executed.
func DispatchToolCall(ctx context.Context, registry pkg.ToolRegistry, call pkg.ToolCall, dryRun bool) (string, error) {
	const maxArgsSize = 1 << 20 // 1 MB

	var args map[string]any
	if call.Arguments != "" {
		if len(call.Arguments) > maxArgsSize {
			return "", fmt.Errorf("dispatch: arguments for %q exceed %d bytes — rejecting", call.Name, maxArgsSize)
		}
		if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
			return "", fmt.Errorf("dispatch: unmarshal args for %q: %w", call.Name, err)
		}
	}
	if args == nil {
		args = make(map[string]any)
	}

	handler, ok := registry.Get(call.Name)
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", call.Name)
	}

	if dryRun {
		summary := toolCallSummary(args)
		slog.Info("engine: dry-run tool call", "tool", call.Name, "args", summary)
		return fmt.Sprintf("[dry-run] %s(%s)", call.Name, summary), nil
	}

	result, err := handler.Execute(ctx, args)
	if err != nil {
		return "", fmt.Errorf("dispatch: execute %q: %w", call.Name, err)
	}
	return result, nil
}
