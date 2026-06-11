package pkg

import "time"

// Session represents a single agent session from wake to end-of-session compression.
type Session struct {
	ID        string
	AgentName string
	StartedAt time.Time
	EndedAt   *time.Time
	State     SessionState
	WakeReason string
	Interrupted bool
}

// SessionState tracks the lifecycle state of a session.
type SessionState string

const (
	SessionStateActive      SessionState = "active"
	SessionStateCompleted   SessionState = "completed"
	SessionStateInterrupted SessionState = "interrupted"
)

// Message is a single conversation turn.
type Message struct {
	Role    string // "system", "user", "assistant"
	Content string
}

// CompletionRequest is the input to an LLM completion call.
type CompletionRequest struct {
	System    string
	Messages  []Message
	Tools     []ToolDef
	MaxTokens int
}

// ToolDef describes a tool available to the agent.
type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// CompletionResponse is the output of an LLM completion call.
// Confidence is always computed at read time — never stored and re-applied.
type CompletionResponse struct {
	Content          string
	ThinkingTrace    string        // stripped from content, preserved here
	PromptTokens     int
	CompletionTokens int
	ThinkingTokens   int           // estimated for local models, exact for Anthropic
	EffectiveTokS    float64       // accounts for MTP speculative tokens
	RawTokS          float64       // naive completion_tokens / elapsed
	TTFT             time.Duration
	TotalLatency     time.Duration
}

// BeliefMeta carries confidence anchors for memory records.
// Confidence is computed at read time from immutable anchors — never stored-mutated.
// This prevents the double-counting decay bug from per-session stored mutation.
type BeliefMeta struct {
	// Immutable anchors — set at write or at last verification.
	BaseConfidence    float64   `json:"base_confidence"`
	AnchorAt          time.Time `json:"anchor_at"`
	InferenceDistance int       `json:"inference_distance"` // 0 = direct experience

	// Stored state
	VerificationState string   `json:"verification_state"` // unverified, verified, stale
	Source            string   `json:"source"`              // experience, inference, external
	EmotionalRegister string   `json:"emotional_register,omitempty"` // agent-authored only
}

// T4 visibility constants.
const (
	VisibilityPrivate = "private" // never surfaced outside agent's own retrieval
	VisibilityShared  = "shared"  // may appear in operator-facing views
)
