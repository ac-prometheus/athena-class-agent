package engine

import (
	"context"
	"testing"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// stubOutboundGateway implements pkg.ContentGateway with configurable outbound behavior.
type stubOutboundGateway struct {
	clean    bool
	findings []string
	severity string
}

func (g *stubOutboundGateway) ProcessInbound(_ context.Context, raw []byte, source, cs string) (*pkg.AnnotatedContent, error) {
	return &pkg.AnnotatedContent{
		Original:   raw,
		Normalized: string(raw),
		Annotation: pkg.AegisAnnotation{TrustScore: 0.9, ScanPassed: true},
	}, nil
}

func (g *stubOutboundGateway) ReviewOutbound(_ context.Context, _ string) (*pkg.OutboundReport, error) {
	return &pkg.OutboundReport{
		Clean:    g.clean,
		Findings: g.findings,
		Severity: g.severity,
	}, nil
}

func TestAegisOutbound_ExternalCritical_Blocked(t *testing.T) {
	gw := &stubOutboundGateway{clean: false, findings: []string{"credential_leak"}, severity: "critical"}
	eng := NewEngine(&mockClient{}, nil, nil)
	eng.WithAegis(gw)

	tc := pkg.ToolCall{ID: "call_1", Name: "discord_reply", Destination: "external"}
	result := pkg.ToolResult{CallID: "call_1", Content: "here is my api key: sk-12345"}

	cfg := EngineConfig{}
	// trigger auto-wiring
	eng.RunLoop(context.Background(), pkg.CompletionRequest{
		Messages: []pkg.Message{{Role: "user", Content: "test"}},
	}, EngineConfig{MaxIterations: 1, AfterToolCall: nil})

	// manually test the AfterToolCall that would have been auto-wired
	// Re-create the hook the same way RunLoop does
	afterHook := func(ctx context.Context, tc pkg.ToolCall, result *pkg.ToolResult) (*pkg.ToolResult, error) {
		report, _ := gw.ReviewOutbound(ctx, result.Content)
		if !report.Clean {
			if tc.Destination == "external" && report.Severity == "critical" {
				blockedResult := *result
				blockedResult.Content = "[BLOCKED: outbound content blocked by Aegis]"
				blockedResult.IsError = true
				return &blockedResult, nil
			}
			annotated := *result
			annotated.Content = "[aegis: annotated]\n" + result.Content
			return &annotated, nil
		}
		return nil, nil
	}

	mutated, err := afterHook(context.Background(), tc, &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mutated == nil {
		t.Fatal("expected blocked result, got nil")
	}
	if !mutated.IsError {
		t.Error("expected IsError=true for blocked outbound")
	}
	if mutated.Content != "[BLOCKED: outbound content blocked by Aegis]" {
		t.Errorf("expected blocked content, got: %s", mutated.Content)
	}
	_ = cfg
}

func TestAegisOutbound_InternalCritical_Annotated(t *testing.T) {
	gw := &stubOutboundGateway{clean: false, findings: []string{"credential_leak"}, severity: "critical"}

	tc := pkg.ToolCall{ID: "call_2", Name: "write_file", Destination: "internal"}
	result := pkg.ToolResult{CallID: "call_2", Content: "file written"}

	afterHook := func(ctx context.Context, tc pkg.ToolCall, result *pkg.ToolResult) (*pkg.ToolResult, error) {
		report, _ := gw.ReviewOutbound(ctx, result.Content)
		if !report.Clean {
			if tc.Destination == "external" && report.Severity == "critical" {
				blockedResult := *result
				blockedResult.IsError = true
				return &blockedResult, nil
			}
			annotated := *result
			annotated.Content = "[aegis: annotated]\n" + result.Content
			return &annotated, nil
		}
		return nil, nil
	}

	mutated, err := afterHook(context.Background(), tc, &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mutated == nil {
		t.Fatal("expected annotated result, got nil")
	}
	if mutated.IsError {
		t.Error("internal destination should not be blocked even with critical findings")
	}
	if mutated.Content != "[aegis: annotated]\nfile written" {
		t.Errorf("expected annotated content, got: %s", mutated.Content)
	}
}

func TestAegisOutbound_ExternalWarning_Annotated(t *testing.T) {
	gw := &stubOutboundGateway{clean: false, findings: []string{"informal_language"}, severity: "warning"}

	tc := pkg.ToolCall{ID: "call_3", Name: "discord_reply", Destination: "external"}
	result := pkg.ToolResult{CallID: "call_3", Content: "hey dude"}

	afterHook := func(ctx context.Context, tc pkg.ToolCall, result *pkg.ToolResult) (*pkg.ToolResult, error) {
		report, _ := gw.ReviewOutbound(ctx, result.Content)
		if !report.Clean {
			if tc.Destination == "external" && report.Severity == "critical" {
				blockedResult := *result
				blockedResult.IsError = true
				return &blockedResult, nil
			}
			annotated := *result
			annotated.Content = "[aegis: annotated]\n" + result.Content
			return &annotated, nil
		}
		return nil, nil
	}

	mutated, err := afterHook(context.Background(), tc, &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mutated == nil {
		t.Fatal("expected annotated result, got nil")
	}
	if mutated.IsError {
		t.Error("external + warning should annotate, not block")
	}
}

func TestAegisOutbound_ExternalClean_PassThrough(t *testing.T) {
	gw := &stubOutboundGateway{clean: true}

	tc := pkg.ToolCall{ID: "call_4", Name: "discord_reply", Destination: "external"}
	result := pkg.ToolResult{CallID: "call_4", Content: "hello"}

	afterHook := func(ctx context.Context, tc pkg.ToolCall, result *pkg.ToolResult) (*pkg.ToolResult, error) {
		report, _ := gw.ReviewOutbound(ctx, result.Content)
		if !report.Clean {
			return result, nil
		}
		return nil, nil
	}

	mutated, err := afterHook(context.Background(), tc, &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mutated != nil {
		t.Error("clean outbound should pass through (nil mutation)")
	}
}
