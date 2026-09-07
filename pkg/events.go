package pkg

// StreamEventType identifies the kind of stream event.
type StreamEventType string

const (
	EventTextStart     StreamEventType = "text_start"
	EventTextDelta     StreamEventType = "text_delta"
	EventTextEnd       StreamEventType = "text_end"
	EventThinkStart    StreamEventType = "thinking_start"
	EventThinkDelta    StreamEventType = "thinking_delta"
	EventThinkEnd      StreamEventType = "thinking_end"
	EventToolCallStart StreamEventType = "toolcall_start"
	EventToolCallDelta StreamEventType = "toolcall_delta"
	EventToolCallEnd   StreamEventType = "toolcall_end"
)

// StreamEvent is a single event emitted during an LLM completion stream.
type StreamEvent struct {
	Type     StreamEventType
	Text     string    // delta content for text/thinking events
	ToolCall *ToolCall // populated on EventToolCallEnd
	Index    int       // tool call index (for toolcall events)
}

// StreamSubscriber receives stream events from an LLM completion.
type StreamSubscriber interface {
	OnEvent(ev StreamEvent)
}

// ---------------------------------------------------------------------------
// Engine lifecycle events (HARN-45)
// ---------------------------------------------------------------------------

// EngineEvent is emitted by the engine loop at key lifecycle points.
// Phase 5A's TUI will consume these over UDS; this sprint is emit-only.
//
// Contract: EventSink implementations must return fast and never block.
// The engine loop calls the sink synchronously — a blocking sink stalls
// inference. Use a non-blocking channel send or a direct function call
// that buffers internally if the consumer is slow.
type EngineEvent struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
	Timestamp int64  `json:"timestamp"` // UnixNano for precision without time.Time serialization cost
	Data      any    `json:"data,omitempty"`
}

const (
	EngineEventSessionStart     = "session_start"
	EngineEventSessionEnd       = "session_end"
	EngineEventTurnStart        = "turn_start"
	EngineEventLLMRequest       = "llm_request"
	EngineEventLLMResponse      = "llm_response"
	EngineEventToolDispatch     = "tool_dispatch"
	EngineEventToolResult       = "tool_result"
	EngineEventHookBlock        = "hook_block"
	EngineEventSteeringInjected = "steering_injected"
	EngineEventLoopTerminated   = "loop_terminated"
)

// EventSink receives engine lifecycle events. Nil means no-op.
// Implementations must return immediately — the engine loop calls the
// sink on the hot path. See EngineEvent doc for the full contract.
type EventSink func(EngineEvent)
