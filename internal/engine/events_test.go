package engine

import (
	"context"
	"sync"
	"testing"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// eventRecorder collects events emitted during a test.
type eventRecorder struct {
	mu     sync.Mutex
	events []pkg.EngineEvent
}

func (r *eventRecorder) sink(ev pkg.EngineEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

func (r *eventRecorder) types() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.events))
	for i, e := range r.events {
		out[i] = e.Type
	}
	return out
}

func TestEventEmission_PlainCompletion(t *testing.T) {
	client := &mockClient{
		responses: []*pkg.CompletionResponse{
			{
				Blocks:       []pkg.ContentBlock{{Type: pkg.BlockText, Text: "hello"}},
				FinishReason: "stop",
			},
		},
	}

	reg := newMockRegistry()
	eng := NewEngine(client, reg, nil)
	eng.WithSessionID("test-session")

	rec := &eventRecorder{}

	_, err := eng.RunLoop(context.Background(), pkg.CompletionRequest{
		Messages: []pkg.Message{{Role: "user", Content: "hi"}},
	}, EngineConfig{
		EventSink: rec.sink,
	})
	if err != nil {
		t.Fatalf("RunLoop: %v", err)
	}

	types := rec.types()
	expected := []string{
		pkg.EngineEventSessionStart,
		pkg.EngineEventTurnStart,
		pkg.EngineEventLLMRequest,
		pkg.EngineEventLLMResponse,
		pkg.EngineEventLoopTerminated,
	}

	if len(types) != len(expected) {
		t.Fatalf("event count = %d, want %d: %v", len(types), len(expected), types)
	}
	for i, want := range expected {
		if types[i] != want {
			t.Errorf("event[%d] = %q, want %q", i, types[i], want)
		}
	}

	for _, ev := range rec.events {
		if ev.SessionID != "test-session" {
			t.Errorf("event %q has session_id %q, want %q", ev.Type, ev.SessionID, "test-session")
		}
		if ev.Timestamp == 0 {
			t.Errorf("event %q has zero timestamp", ev.Type)
		}
	}
}

func TestEventEmission_ToolRoundTrip(t *testing.T) {
	client := &mockClient{
		responses: []*pkg.CompletionResponse{
			{
				Blocks: []pkg.ContentBlock{
					{Type: pkg.BlockToolCall, ToolCall: &pkg.ToolCall{ID: "call_1", Name: "echo_tool", Arguments: `{"msg":"hi"}`}},
				},
				FinishReason: "tool_calls",
			},
			{
				Blocks:       []pkg.ContentBlock{{Type: pkg.BlockText, Text: "done"}},
				FinishReason: "stop",
			},
		},
	}

	reg := newMockRegistry()
	reg.Register(&mockHandler{name: "echo_tool", result: "echoed"})
	eng := NewEngine(client, reg, nil)
	eng.WithSessionID("test-tool-session")

	rec := &eventRecorder{}

	_, err := eng.RunLoop(context.Background(), pkg.CompletionRequest{
		Messages: []pkg.Message{{Role: "user", Content: "use echo"}},
	}, EngineConfig{
		EventSink:     rec.sink,
		ParallelTools: false,
	})
	if err != nil {
		t.Fatalf("RunLoop: %v", err)
	}

	types := rec.types()

	hasDispatch := false
	hasResult := false
	for _, typ := range types {
		if typ == pkg.EngineEventToolDispatch {
			hasDispatch = true
		}
		if typ == pkg.EngineEventToolResult {
			hasResult = true
		}
	}
	if !hasDispatch {
		t.Error("missing tool_dispatch event")
	}
	if !hasResult {
		t.Error("missing tool_result event")
	}
}

func TestEventEmission_HookBlock(t *testing.T) {
	client := &mockClient{
		responses: []*pkg.CompletionResponse{
			{
				Blocks: []pkg.ContentBlock{
					{Type: pkg.BlockToolCall, ToolCall: &pkg.ToolCall{ID: "call_1", Name: "blocked_tool", Arguments: `{}`}},
				},
				FinishReason: "tool_calls",
			},
			{
				Blocks:       []pkg.ContentBlock{{Type: pkg.BlockText, Text: "ok"}},
				FinishReason: "stop",
			},
		},
	}

	reg := newMockRegistry()
	reg.Register(&mockHandler{name: "blocked_tool", result: "should not run"})
	eng := NewEngine(client, reg, nil)
	eng.WithSessionID("test-block-session")

	rec := &eventRecorder{}

	_, err := eng.RunLoop(context.Background(), pkg.CompletionRequest{
		Messages: []pkg.Message{{Role: "user", Content: "try blocked tool"}},
	}, EngineConfig{
		EventSink:     rec.sink,
		ParallelTools: false,
		BeforeToolCall: func(_ context.Context, _ pkg.ToolCall, _ map[string]any) (*HookResult, error) {
			return &HookResult{Block: true, Reason: "aegis: test block"}, nil
		},
	})
	if err != nil {
		t.Fatalf("RunLoop: %v", err)
	}

	types := rec.types()
	hasBlock := false
	for _, typ := range types {
		if typ == pkg.EngineEventHookBlock {
			hasBlock = true
		}
	}
	if !hasBlock {
		t.Errorf("missing hook_block event; got: %v", types)
	}
}

func TestEventEmission_NilSink(t *testing.T) {
	client := &mockClient{
		responses: []*pkg.CompletionResponse{
			{
				Blocks:       []pkg.ContentBlock{{Type: pkg.BlockText, Text: "hello"}},
				FinishReason: "stop",
			},
		},
	}

	reg := newMockRegistry()
	eng := NewEngine(client, reg, nil)

	// Nil EventSink — must not panic.
	_, err := eng.RunLoop(context.Background(), pkg.CompletionRequest{
		Messages: []pkg.Message{{Role: "user", Content: "hi"}},
	}, EngineConfig{})
	if err != nil {
		t.Fatalf("RunLoop with nil sink: %v", err)
	}
}
