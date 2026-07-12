package engine

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// --- Mock helpers ---

type mockClient struct {
	responses []*pkg.CompletionResponse
	callCount int
}

func (m *mockClient) Complete(_ context.Context, _ pkg.CompletionRequest) (*pkg.CompletionResponse, error) {
	if m.callCount >= len(m.responses) {
		return &pkg.CompletionResponse{
			Blocks:       []pkg.ContentBlock{{Type: pkg.BlockText, Text: "done"}},
			FinishReason: "stop",
		}, nil
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return resp, nil
}

type mockHandler struct {
	name   string
	result string
	err    error
	called atomic.Int64
}

func (h *mockHandler) Name() string { return h.name }
func (h *mockHandler) Execute(_ context.Context, _ map[string]any) (string, error) {
	h.called.Add(1)
	return h.result, h.err
}

type mockRegistry struct {
	handlers map[string]pkg.ToolHandler
	meta     map[string]pkg.ToolMeta
}

func newMockRegistry() *mockRegistry {
	return &mockRegistry{
		handlers: make(map[string]pkg.ToolHandler),
		meta:     make(map[string]pkg.ToolMeta),
	}
}

func (r *mockRegistry) Register(h pkg.ToolHandler) {
	r.handlers[h.Name()] = h
	r.meta[h.Name()] = pkg.ToolMeta{ExecMode: pkg.ExecParallel}
}

func (r *mockRegistry) registerSeq(h pkg.ToolHandler) {
	r.handlers[h.Name()] = h
	r.meta[h.Name()] = pkg.ToolMeta{ExecMode: pkg.ExecSequential}
}

func (r *mockRegistry) Get(name string) (pkg.ToolHandler, bool) {
	h, ok := r.handlers[name]
	return h, ok
}

func (r *mockRegistry) GetMeta(name string) (pkg.ToolMeta, bool) {
	m, ok := r.meta[name]
	return m, ok
}

func (r *mockRegistry) List() []pkg.ToolGroup { return nil }

// toolCallResp builds a CompletionResponse with one tool call block.
func toolCallResp(id, name, args string) *pkg.CompletionResponse {
	return &pkg.CompletionResponse{
		Blocks: []pkg.ContentBlock{
			{Type: pkg.BlockToolCall, ToolCall: &pkg.ToolCall{ID: id, Name: name, Arguments: args}},
		},
		FinishReason: "tool_calls",
	}
}

// textResp builds a CompletionResponse with a text block.
func textResp(text string) *pkg.CompletionResponse {
	return &pkg.CompletionResponse{
		Blocks:       []pkg.ContentBlock{{Type: pkg.BlockText, Text: text}},
		FinishReason: "stop",
	}
}

func newEngine(client pkg.LLMClient, reg pkg.ToolRegistry) *Engine {
	return NewEngine(client, reg, nil, nil)
}

// --- Tests ---

func TestRunLoop_SingleTurn(t *testing.T) {
	client := &mockClient{responses: []*pkg.CompletionResponse{textResp("hello")}}
	e := newEngine(client, newMockRegistry())

	result, err := e.RunLoop(context.Background(), pkg.CompletionRequest{}, EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Iterations != 1 {
		t.Errorf("got %d iterations, want 1", result.Iterations)
	}
	if result.Terminated {
		t.Error("should not be terminated")
	}
}

func TestRunLoop_ToolCall(t *testing.T) {
	handler := &mockHandler{name: "echo", result: "pong"}
	reg := newMockRegistry()
	reg.Register(handler)

	client := &mockClient{responses: []*pkg.CompletionResponse{
		toolCallResp("c1", "echo", `{"msg":"ping"}`),
		textResp("done"),
	}}
	e := newEngine(client, reg)

	result, err := e.RunLoop(context.Background(), pkg.CompletionRequest{}, EngineConfig{ParallelTools: true})
	if err != nil {
		t.Fatal(err)
	}
	if handler.called.Load() != 1 {
		t.Errorf("handler called %d times, want 1", handler.called.Load())
	}

	// History: initial(0) + assistant + tool-result + assistant-final = 3 appended.
	var toolMessages int
	for _, m := range result.History {
		if m.Role == "tool" {
			toolMessages++
		}
	}
	if toolMessages != 1 {
		t.Errorf("got %d role:tool messages, want 1", toolMessages)
	}
}

func TestRunLoop_ParallelExecution(t *testing.T) {
	handler1 := &mockHandler{name: "slow1", result: "r1"}
	handler2 := &mockHandler{name: "slow2", result: "r2"}

	wrapExec := func(h *mockHandler) pkg.ToolHandler {
		return &funcHandler{
			name: h.name,
			fn: func(_ context.Context, _ map[string]any) (string, error) {
				time.Sleep(50 * time.Millisecond)
				return h.result, nil
			},
		}
	}

	reg := newMockRegistry()
	reg.Register(wrapExec(handler1))
	reg.Register(wrapExec(handler2))

	// Single response with two tool calls, then done.
	client := &mockClient{responses: []*pkg.CompletionResponse{
		{
			Blocks: []pkg.ContentBlock{
				{Type: pkg.BlockToolCall, ToolCall: &pkg.ToolCall{ID: "c1", Name: "slow1", Arguments: "{}"}},
				{Type: pkg.BlockToolCall, ToolCall: &pkg.ToolCall{ID: "c2", Name: "slow2", Arguments: "{}"}},
			},
			FinishReason: "tool_calls",
		},
		textResp("done"),
	}}
	e := newEngine(client, reg)

	t0 := time.Now()
	_, err := e.RunLoop(context.Background(), pkg.CompletionRequest{}, EngineConfig{ParallelTools: true})
	elapsed := time.Since(t0)
	if err != nil {
		t.Fatal(err)
	}

	// Both started before either could finish (parallel): total < 90ms (not 100ms sequential).
	if elapsed >= 90*time.Millisecond {
		t.Errorf("parallel execution took %v, expected < 90ms", elapsed)
	}
}

// funcHandler allows ad-hoc handler creation in tests.
type funcHandler struct {
	name string
	fn   func(context.Context, map[string]any) (string, error)
}

func (f *funcHandler) Name() string { return f.name }
func (f *funcHandler) Execute(ctx context.Context, args map[string]any) (string, error) {
	return f.fn(ctx, args)
}

func TestRunLoop_SequentialFallback(t *testing.T) {
	var mu sync.Mutex
	order := make([]string, 0, 2)

	makeHandler := func(name string) pkg.ToolHandler {
		return &funcHandler{
			name: name,
			fn: func(_ context.Context, _ map[string]any) (string, error) {
				mu.Lock()
				order = append(order, name)
				mu.Unlock()
				return name + "-result", nil
			},
		}
	}

	reg := newMockRegistry()
	reg.registerSeq(makeHandler("seq-tool"))
	reg.Register(makeHandler("par-tool"))

	client := &mockClient{responses: []*pkg.CompletionResponse{
		{
			Blocks: []pkg.ContentBlock{
				{Type: pkg.BlockToolCall, ToolCall: &pkg.ToolCall{ID: "c1", Name: "seq-tool", Arguments: "{}"}},
				{Type: pkg.BlockToolCall, ToolCall: &pkg.ToolCall{ID: "c2", Name: "par-tool", Arguments: "{}"}},
			},
			FinishReason: "tool_calls",
		},
		textResp("done"),
	}}
	e := newEngine(client, reg)

	_, err := e.RunLoop(context.Background(), pkg.CompletionRequest{}, EngineConfig{ParallelTools: true})
	if err != nil {
		t.Fatal(err)
	}

	// Sequential fallback: seq-tool runs first (index 0) and seq-tool is index 0 in the batch.
	if len(order) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(order))
	}
	// Both ran; order is deterministic (no parallel).
	if order[0] != "seq-tool" {
		t.Errorf("expected seq-tool first, got %s", order[0])
	}
}

func TestRunLoop_ErrorAsResult(t *testing.T) {
	handler := &mockHandler{name: "broken", err: errors.New("kaboom")}
	reg := newMockRegistry()
	reg.Register(handler)

	client := &mockClient{responses: []*pkg.CompletionResponse{
		toolCallResp("c1", "broken", "{}"),
		textResp("recovered"),
	}}
	e := newEngine(client, reg)

	result, err := e.RunLoop(context.Background(), pkg.CompletionRequest{}, EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}

	var errMsg pkg.Message
	for _, m := range result.History {
		if m.Role == "tool" {
			errMsg = m
		}
	}
	if !errMsg.IsError {
		t.Error("expected IsError=true on tool result")
	}
}

func TestRunLoop_Terminate(t *testing.T) {
	reg := newMockRegistry()
	reg.Register(&funcHandler{
		name: "stopper",
		fn: func(_ context.Context, _ map[string]any) (string, error) {
			return "bye", nil
		},
	})

	// Override registry to return a ToolHandlerV2 that signals Terminate.
	v2 := &terminateHandler{name: "stopper"}
	reg.handlers["stopper"] = v2

	client := &mockClient{responses: []*pkg.CompletionResponse{
		toolCallResp("c1", "stopper", "{}"),
		textResp("should-not-reach"),
	}}
	e := newEngine(client, reg)

	result, err := e.RunLoop(context.Background(), pkg.CompletionRequest{}, EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Terminated {
		t.Error("expected Terminated=true")
	}
	if result.Iterations != 1 {
		t.Errorf("expected 1 iteration, got %d", result.Iterations)
	}
}

// terminateHandler implements ToolHandlerV2 and signals Terminate.
type terminateHandler struct{ name string }

func (h *terminateHandler) Name() string { return h.name }
func (h *terminateHandler) Execute(_ context.Context, _ map[string]any) (string, error) {
	return "bye", nil
}
func (h *terminateHandler) ExecuteV2(_ context.Context, _ map[string]any) (*pkg.ToolResult, error) {
	return &pkg.ToolResult{Content: "bye", Terminate: true}, nil
}

func TestRunLoop_DryRun(t *testing.T) {
	handler := &mockHandler{name: "action", result: "real-result"}
	reg := newMockRegistry()
	reg.Register(handler)

	client := &mockClient{responses: []*pkg.CompletionResponse{
		toolCallResp("c1", "action", "{}"),
		textResp("done"),
	}}
	e := newEngine(client, reg)

	_, err := e.RunLoop(context.Background(), pkg.CompletionRequest{}, EngineConfig{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if handler.called.Load() != 0 {
		t.Errorf("handler called %d times in dry-run, want 0", handler.called.Load())
	}
}

func TestRunLoop_MaxTurns(t *testing.T) {
	handler := &mockHandler{name: "loop", result: "go"}
	reg := newMockRegistry()
	reg.Register(handler)

	// Always return tool calls — loop should stop at MaxIterations.
	client := &infiniteToolClient{toolName: "loop"}
	e := newEngine(client, reg)

	result, err := e.RunLoop(context.Background(), pkg.CompletionRequest{}, EngineConfig{
		MaxIterations: 3,
		ParallelTools: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Iterations != 3 {
		t.Errorf("got %d iterations, want 3", result.Iterations)
	}
}

// infiniteToolClient always returns a single tool call response.
type infiniteToolClient struct{ toolName string }

func (c *infiniteToolClient) Complete(_ context.Context, _ pkg.CompletionRequest) (*pkg.CompletionResponse, error) {
	return toolCallResp("cx", c.toolName, "{}"), nil
}

func TestRunLoop_AegisBeforeHook(t *testing.T) {
	handler := &mockHandler{name: "guarded", result: "secret"}
	reg := newMockRegistry()
	reg.Register(handler)

	client := &mockClient{responses: []*pkg.CompletionResponse{
		toolCallResp("c1", "guarded", "{}"),
		textResp("done"),
	}}
	e := newEngine(client, reg)

	blocked := false
	cfg := EngineConfig{
		BeforeToolCall: func(_ context.Context, _ pkg.ToolCall, _ map[string]any) (*HookResult, error) {
			blocked = true
			return &HookResult{Block: true, Reason: "aegis: injection detected"}, nil
		},
	}

	result, err := e.RunLoop(context.Background(), pkg.CompletionRequest{}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !blocked {
		t.Error("expected BeforeToolCall to fire")
	}
	if handler.called.Load() != 0 {
		t.Error("handler should not be called when blocked")
	}

	// The blocked result should be IsError=true.
	for _, m := range result.History {
		if m.Role == "tool" && !m.IsError {
			t.Error("expected IsError=true on blocked tool result")
		}
	}
}

func TestRunLoop_AegisAfterHook(t *testing.T) {
	handler := &mockHandler{name: "probe", result: "ok"}
	reg := newMockRegistry()
	reg.Register(handler)

	client := &mockClient{responses: []*pkg.CompletionResponse{
		toolCallResp("c1", "probe", "{}"),
		textResp("done"),
	}}
	e := newEngine(client, reg)

	annotated := false
	cfg := EngineConfig{
		AfterToolCall: func(_ context.Context, _ pkg.ToolCall, r *pkg.ToolResult) (*pkg.ToolResult, error) {
			annotated = true
			// Annotate-only: append a note but do not block.
			r.Content += " [annotated]"
			return r, nil
		},
	}

	result, err := e.RunLoop(context.Background(), pkg.CompletionRequest{}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !annotated {
		t.Error("expected AfterToolCall to fire")
	}

	var found bool
	for _, m := range result.History {
		if m.Role == "tool" && m.Content == "ok [annotated]" {
			found = true
		}
	}
	if !found {
		t.Error("expected annotated content in history")
	}
}

func TestRunLoop_Rehearsal(t *testing.T) {
	callCount := 0
	client := &countingClient{fn: func() { callCount++ }}
	e := newEngine(client, newMockRegistry())

	result, err := e.RunLoop(context.Background(), pkg.CompletionRequest{}, EngineConfig{Rehearsal: true})
	if err != nil {
		t.Fatal(err)
	}
	if callCount != 0 {
		t.Errorf("LLM called %d times in rehearsal mode, want 0", callCount)
	}
	if len(result.History) != 0 {
		t.Error("rehearsal should return empty history")
	}
}

type countingClient struct{ fn func() }

func (c *countingClient) Complete(_ context.Context, _ pkg.CompletionRequest) (*pkg.CompletionResponse, error) {
	c.fn()
	return textResp("x"), nil
}

