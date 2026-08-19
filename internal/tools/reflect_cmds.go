package tools

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// SelfExamineHandler routes a self-examination prompt to an advisor LLM and
// returns the response as transient T1 tool content only.
//
// Provenance boundary: the advisor's output is external content — it is not
// embedded, not inserted into T4, and must not be labeled "self"-authored.
// If the agent wants to preserve a conclusion as a durable T4 reflection, she
// uses write_reflection to author it in her own words.
//
// T2 note: if raw tool traffic is retained in T2 via the T2LoggerHook, each
// self_examine turn should carry ContentSource "tool:self_examine" so it is
// never promoted to agent-authored T4 during consolidation.
type SelfExamineHandler struct {
	// llmFn calls an advisor LLM with a prompt and returns the response text.
	// Injected to keep this handler testable without a full LLMClient dependency.
	llmFn func(string) (string, error)
}

// Name implements pkg.ToolHandler.
func (h *SelfExamineHandler) Name() string { return "self_examine" }

// Execute routes the prompt to the advisor LLM and returns the response as
// transient T1 tool content. Nothing is embedded or stored.
// args["prompt"] — the examination question/prompt (required)
func (h *SelfExamineHandler) Execute(_ context.Context, args map[string]any) (string, error) {
	prompt, ok := stringArg(args, "prompt")
	if !ok || prompt == "" {
		return "", fmt.Errorf("self_examine: missing required arg 'prompt'")
	}

	if h.llmFn == nil {
		return "", fmt.Errorf("self_examine: llmFn not configured")
	}

	content, err := h.llmFn(prompt)
	if err != nil {
		return "", fmt.Errorf("self_examine: LLM call: %w", err)
	}

	// Return as transient T1 only — the agent reads and evaluates this aid.
	// To make a conclusion durable, call write_reflection with agent-authored text.
	return fmt.Sprintf("Advisor examination (not stored — use write_reflection to preserve insights):\n\n%s", content), nil
}

// WriteReflectionHandler stores an agent-authored reflection as T4.
// Respects the T4 consent boundary: base_confidence is never set by the system;
// the agent's words are stored as authored, not confidence-weighted.
type WriteReflectionHandler struct {
	store    pkg.MemoryStore
	provider pkg.EmbeddingProvider
}

// Name implements pkg.ToolHandler.
func (h *WriteReflectionHandler) Name() string { return "write_reflection" }

// Execute stores a reflection authored by the agent.
// args["content"]    — the reflection text (required)
// args["type"]       — reflection type: essay, note, dream, pattern, examination, challenge (default: note)
// args["visibility"] — "private" (default) or "shared"
// args["emotional_register"] — optional emotional register tag
func (h *WriteReflectionHandler) Execute(ctx context.Context, args map[string]any) (string, error) {
	content, ok := stringArg(args, "content")
	if !ok || content == "" {
		return "", fmt.Errorf("write_reflection: missing required arg 'content'")
	}
	reflType, _ := stringArg(args, "type")
	if reflType == "" {
		reflType = "note"
	}
	visibility, _ := stringArg(args, "visibility")
	if visibility == "" {
		visibility = pkg.VisibilityPrivate
	}
	emotionalRegister, _ := stringArg(args, "emotional_register")

	validTypes := map[string]bool{
		"essay": true, "note": true, "dream": true,
		"pattern": true, "examination": true, "challenge": true,
	}
	if !validTypes[reflType] {
		types := make([]string, 0, len(validTypes))
		for k := range validTypes {
			types = append(types, k)
		}
		return "", fmt.Errorf("write_reflection: invalid type %q; valid: %s",
			reflType, strings.Join(types, ", "))
	}

	vec, err := h.provider.Embed(ctx, content)
	if err != nil {
		return "", fmt.Errorf("write_reflection: embed content: %w", err)
	}

	// T4 consent boundary: we never set BaseConfidence — that is an agent-authored field.
	// The system records that this is a self-sourced, directly-experienced entry
	// with zero inference distance, but does not assign a confidence score.
	belief := &pkg.BeliefMeta{
		Source:            "self",
		InferenceDistance: 0,
		AnchorAt:          time.Now().UTC(),
		EmotionalRegister: emotionalRegister,
	}

	ref := pkg.Reflection{
		ID:         newToolID(),
		Type:       reflType,
		Content:    content,
		Visibility: visibility,
		Belief:     belief,
		Embedding:  vec,
	}

	if err := h.store.InsertReflection(ctx, ref); err != nil {
		return "", fmt.Errorf("write_reflection: store: %w", err)
	}

	return fmt.Sprintf("reflection stored (id=%s type=%s visibility=%s)", ref.ID, ref.Type, ref.Visibility), nil
}

// newToolID generates a random UUID v4 for use as a tool-layer record ID.
// Mirrors the unexported newID() in internal/memory — kept local so the tools
// package does not import internal/memory (avoids a potential import cycle).
func newToolID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("tools: crypto/rand unavailable: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
