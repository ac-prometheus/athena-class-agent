package pkg

import (
	"math"
	"strings"
	"time"
)

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
	Role       string         // "system", "user", "assistant", "tool"
	Content    string         // text for user/system; output for tool
	Blocks     []ContentBlock // structured output for assistant turns
	ToolCallID string         // for role="tool": which call this responds to
	IsError    bool           // for role="tool": whether result is an error
}

// CompletionRequest is the input to an LLM completion call.
type CompletionRequest struct {
	System      string
	Messages    []Message
	Tools       []ToolDef
	MaxTokens   int
	Temperature *float64 // nil = endpoint default
}

// ToolDef describes a tool available to the agent.
type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// ToolCall is a single tool invocation requested by the model.
// Arguments is the raw JSON string — callers unmarshal to map[string]any.
type ToolCall struct {
	ID        string // model-assigned call ID (e.g. "call_abc123")
	Name      string
	Arguments string // raw JSON
}

// ContentBlockType identifies the kind of content in a block.
type ContentBlockType string

const (
	BlockText     ContentBlockType = "text"
	BlockThinking ContentBlockType = "thinking"
	BlockToolCall ContentBlockType = "tool_call"
)

// ContentBlock is a single segment of model output.
type ContentBlock struct {
	Type      ContentBlockType
	Text      string    // BlockText
	Thinking  string    // BlockThinking
	ToolCall  *ToolCall // BlockToolCall
	FieldName string    // round-trip: "reasoning", "reasoning_content", or "" (tag-stripped)
}

// ToolResult is the output of executing a tool call.
type ToolResult struct {
	CallID    string
	Content   string
	IsError   bool
	Terminate bool
}

// ExecutionMode controls how a tool participates in parallel batches.
type ExecutionMode string

const (
	ExecParallel   ExecutionMode = "parallel"
	ExecSequential ExecutionMode = "sequential"
)

// ToolMeta carries per-tool metadata for execution planning.
type ToolMeta struct {
	ExecMode ExecutionMode
	Tier     int
	Keywords []string
}

// CompletionResponse is the output of an LLM completion call.
// Confidence is always computed at read time — never stored and re-applied.
type CompletionResponse struct {
	// Legacy fields — populated alongside Blocks during migration (Phase 4 removes).
	Content          string
	ThinkingTrace    string        // stripped from content, preserved here
	ToolCalls        []ToolCall    // non-nil when the model requested tool invocations

	// Structured output — populated by the MOP SSE parser.
	Blocks           []ContentBlock
	FinishReason     string        // "stop", "tool_calls", "length"

	PromptTokens     int
	CompletionTokens int
	ThinkingTokens   int           // estimated for local models, exact for Anthropic
	EffectiveTokS    float64       // accounts for MTP speculative tokens
	RawTokS          float64       // naive completion_tokens / elapsed
	TTFT             time.Duration
	TotalLatency     time.Duration
}

// TextContent returns concatenated text from all text blocks.
func (r *CompletionResponse) TextContent() string {
	var b strings.Builder
	for _, blk := range r.Blocks {
		if blk.Type == BlockText {
			b.WriteString(blk.Text)
		}
	}
	return b.String()
}

// ThinkingContent returns concatenated thinking from all thinking blocks.
func (r *CompletionResponse) ThinkingContent() string {
	var b strings.Builder
	for _, blk := range r.Blocks {
		if blk.Type == BlockThinking {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(blk.Thinking)
		}
	}
	return b.String()
}

// ToolCallBlocks returns tool calls extracted from content blocks.
func (r *CompletionResponse) ToolCallBlocks() []ToolCall {
	var calls []ToolCall
	for _, blk := range r.Blocks {
		if blk.Type == BlockToolCall && blk.ToolCall != nil {
			calls = append(calls, *blk.ToolCall)
		}
	}
	return calls
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

// Confidence computes the current confidence of this belief at time now.
// Uses the spec formula:
//
//	effectiveRate = decayRate / pow(inferenceDecayBase, InferenceDistance)
//	Confidence    = BaseConfidence * exp(-effectiveRate * daysElapsed)
//
// Confidence is never stored — it is always derived from the immutable anchors.
// This prevents the double-counting decay bug that occurs when each session
// re-applies decay to an already-mutated stored value.
func (m *BeliefMeta) Confidence(now time.Time, decayRate, inferenceDecayBase float64) float64 {
	effectiveRate := decayRate / math.Pow(inferenceDecayBase, float64(m.InferenceDistance))
	days := now.Sub(m.AnchorAt).Hours() / 24
	if days < 0 {
		days = 0
	}
	return m.BaseConfidence * math.Exp(-effectiveRate*days)
}

// T4 visibility constants.
const (
	VisibilityPrivate = "private" // never surfaced outside agent's own retrieval
	VisibilityShared  = "shared"  // may appear in operator-facing views
)

// Content source identifiers for Aegis annotation.
const (
	ContentSourceSelf           = "self"
	ContentSourceOperator       = "operator"
	ContentSourceToolResult     = "tool-result"
	ContentSourceBrowserContent = "browser-content"
	ContentSourceSearchResult   = "search-result"
	ContentSourceForumContent   = "forum-content"
	ContentSourceDiscord        = "discord"
)
