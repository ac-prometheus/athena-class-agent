package telemetry

import (
	"time"

	"github.com/ac-prometheus/athena-class-agent/internal/memory"
)

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

// EchoDiversityMetrics captures the compositional diversity of the echo pool
// loaded at session start.
type EchoDiversityMetrics struct {
	TemporalSpreadDays float64 // age range between newest and oldest echo
	TopicDiversity     float64 // average pairwise cosine distance across echo embeddings
	Count              int
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

	// Phase 6 fields
	EchoDiversity         EchoDiversityMetrics
	ConvergenceRatio      float64
	ContradictionSurfaced bool
	DecayReport           *memory.DecayReport
}
