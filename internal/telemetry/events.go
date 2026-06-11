package telemetry

import "time"

// TurnEvent carries per-turn metrics emitted after each completed turn.
type TurnEvent struct {
	SessionID        string
	TurnNumber       int
	PromptTokens     int
	CompletionTokens int
	ThinkingTokens   int
	TTFT             time.Duration
	TotalDuration    time.Duration
	ToolCalls        []string
	Model            string
}

// SessionEvent carries aggregate metrics emitted at session end.
type SessionEvent struct {
	SessionID             string
	TotalTurns            int
	TotalPromptTokens     int
	TotalCompletionTokens int
	TotalDuration         time.Duration
	AverageTokPerSec      float64
	AverageTTFT           time.Duration
}
