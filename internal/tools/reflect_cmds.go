package tools

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// SelfExamineHandler runs a self-examination prompt through the LLM and stores
// the result as a T4 reflection with type "examination".
type SelfExamineHandler struct {
	store    pkg.MemoryStore
	provider pkg.EmbeddingProvider
	// llmFn calls an LLM with a prompt and returns the response text.
	// Injected to keep this handler testable without a full LLMClient dependency.
	llmFn func(string) (string, error)
}

// Name implements pkg.ToolHandler.
func (h *SelfExamineHandler) Name() string { return "self_examine" }

// Execute runs a self-examination prompt and stores the result as T4.
// args["prompt"] — the examination question/prompt (required)
// args["visibility"] — "private" (default) or "shared"
func (h *SelfExamineHandler) Execute(ctx context.Context, args map[string]any) (string, error) {
	prompt, ok := stringArg(args, "prompt")
	if !ok || prompt == "" {
		return "", fmt.Errorf("self_examine: missing required arg 'prompt'")
	}
	visibility, _ := stringArg(args, "visibility")
	if visibility == "" {
		visibility = pkg.VisibilityPrivate
	}

	if h.llmFn == nil {
		return "", fmt.Errorf("self_examine: llmFn not configured")
	}

	content, err := h.llmFn(prompt)
	if err != nil {
		return "", fmt.Errorf("self_examine: LLM call: %w", err)
	}

	vec, err := h.provider.Embed(ctx, content)
	if err != nil {
		return "", fmt.Errorf("self_examine: embed result: %w", err)
	}

	ref := pkg.Reflection{
		ID:         newToolID(),
		Type:       "examination",
		Content:    content,
		Visibility: visibility,
		// T4 consent boundary: BaseConfidence is never set by the system.
		// The agent authors reflections; the system does not assign confidence to them.
		Belief: &pkg.BeliefMeta{
			Source:   "self",
			AnchorAt: time.Now().UTC(),
		},
		Embedding: vec,
	}

	if err := h.store.InsertReflection(ctx, ref); err != nil {
		return "", fmt.Errorf("self_examine: store reflection: %w", err)
	}

	return fmt.Sprintf("examination stored (id=%s)\n\n%s", ref.ID, content), nil
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
