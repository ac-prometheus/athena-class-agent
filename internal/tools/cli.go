package tools

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// CLIDispatcher is the harness-side handler for SocketRequests from the CLI binary.
// It routes method names to sandbox execution, advisor queries, tool registry lookups,
// or the built-in "discover" verb.
type CLIDispatcher struct {
	registry pkg.ToolRegistry
	sandbox  *Sandbox
	advisor  *Advisor
}

// NewCLIDispatcher creates a CLIDispatcher wired to the given registry, sandbox, and advisor.
func NewCLIDispatcher(registry pkg.ToolRegistry, sandbox *Sandbox, advisor *Advisor) *CLIDispatcher {
	return &CLIDispatcher{
		registry: registry,
		sandbox:  sandbox,
		advisor:  advisor,
	}
}

// Handle routes a SocketRequest to the appropriate handler and returns a SocketResponse.
// The method field drives dispatch:
//
//	"bash" | "shell"  → sandbox.Execute
//	"advisor"         → advisor.Ask
//	"discover"        → list available tool groups by tier
//	anything else     → registry.Get(method) → handler.Execute
func (d *CLIDispatcher) Handle(ctx context.Context, req SocketRequest) SocketResponse {
	slog.Debug("cli dispatcher: handle", "method", req.Method)

	switch strings.ToLower(req.Method) {
	case "bash", "shell":
		return d.handleBash(ctx, req.Args)
	case "advisor":
		return d.handleAdvisor(ctx, req.Args)
	case "discover":
		return d.handleDiscover(req.Args)
	default:
		return d.handleRegistry(ctx, req.Method, req.Args)
	}
}

func (d *CLIDispatcher) handleBash(ctx context.Context, args map[string]any) SocketResponse {
	command, ok := stringArg(args, "command")
	if !ok {
		return SocketResponse{Error: "bash: missing required arg 'command'"}
	}
	if d.sandbox == nil {
		return SocketResponse{Error: "bash: sandbox not configured"}
	}
	result, err := d.sandbox.Execute(ctx, command)
	if err != nil {
		return SocketResponse{Error: fmt.Sprintf("bash: %v", err)}
	}
	return SocketResponse{Result: result}
}

func (d *CLIDispatcher) handleAdvisor(ctx context.Context, args map[string]any) SocketResponse {
	question, ok := stringArg(args, "question")
	if !ok {
		return SocketResponse{Error: "advisor: missing required arg 'question'"}
	}
	provider, _ := stringArg(args, "provider")
	if d.advisor == nil {
		return SocketResponse{Error: "advisor: advisor not configured"}
	}
	result, err := d.advisor.Ask(ctx, question, provider)
	if err != nil {
		return SocketResponse{Error: fmt.Sprintf("advisor: %v", err)}
	}
	return SocketResponse{Result: result}
}

func (d *CLIDispatcher) handleDiscover(args map[string]any) SocketResponse {
	groups := d.registry.List()
	if len(groups) == 0 {
		return SocketResponse{Result: "no tool groups registered"}
	}

	// Filter by tier if requested.
	tierStr, hasTier := stringArg(args, "tier")

	var sb strings.Builder
	for _, g := range groups {
		if hasTier && fmt.Sprintf("%d", g.Tier) != tierStr && strings.ToLower(g.Name) != tierStr {
			continue
		}
		sb.WriteString(fmt.Sprintf("tier %d (%s) — %d tool(s)", g.Tier, g.Name, len(g.Tools)))
		if len(g.Keywords) > 0 {
			sb.WriteString(fmt.Sprintf(", keywords: %s", strings.Join(g.Keywords, ", ")))
		}
		sb.WriteString("\n")
		for _, t := range g.Tools {
			sb.WriteString(fmt.Sprintf("  • %s", t.Name))
			if t.Description != "" {
				sb.WriteString(fmt.Sprintf(": %s", t.Description))
			}
			sb.WriteString("\n")
		}
	}
	return SocketResponse{Result: sb.String()}
}

func (d *CLIDispatcher) handleRegistry(ctx context.Context, method string, args map[string]any) SocketResponse {
	handler, ok := d.registry.Get(method)
	if !ok {
		return SocketResponse{Error: fmt.Sprintf("unknown command: %s", method)}
	}
	if args == nil {
		args = make(map[string]any)
	}
	result, err := handler.Execute(ctx, args)
	if err != nil {
		return SocketResponse{Error: fmt.Sprintf("%s: %v", method, err)}
	}
	return SocketResponse{Result: result}
}

// stringArg extracts a string value from args by key.
func stringArg(args map[string]any, key string) (string, bool) {
	v, ok := args[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}
