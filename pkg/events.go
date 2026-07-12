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
