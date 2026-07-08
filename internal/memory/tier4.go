package memory

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// Reflection type constants for the reflection_type column.
const (
	ReflectionEssay       = "essay"
	ReflectionNote        = "note"
	ReflectionDream       = "dream"
	ReflectionPattern     = "pattern"
	ReflectionExamination = "examination"
	ReflectionChallenge   = "challenge"
)

var validReflectionTypes = map[string]bool{
	ReflectionEssay:       true,
	ReflectionNote:        true,
	ReflectionDream:       true,
	ReflectionPattern:     true,
	ReflectionExamination: true,
	ReflectionChallenge:   true,
}

// IsValidReflectionType reports whether t is a known reflection_type value.
func IsValidReflectionType(t string) bool {
	return validReflectionTypes[t]
}

// WriteReflection stores a T4 agent-authored reflection.
//
// T4 CONSENT BOUNDARY: base_confidence belongs to the agent alone.
// If Belief.BaseConfidence is non-zero but AnchorAt is zero, we log a warning
// but do not block the write — the store layer handles the consent boundary.
// The system never sets BaseConfidence on T4 records.
func WriteReflection(
	ctx context.Context,
	store pkg.MemoryStore,
	provider pkg.EmbeddingProvider,
	ref pkg.Reflection,
) error {
	if ref.ID == "" {
		return fmt.Errorf("tier4: reflection ID is required")
	}
	if ref.Content == "" {
		return fmt.Errorf("tier4: reflection content is required")
	}

	// Warn if confidence was set without an anchor — this is the double-counting
	// footgun the immutable-anchor design exists to prevent.
	if ref.Belief != nil && ref.Belief.BaseConfidence != 0 && ref.Belief.AnchorAt.IsZero() {
		slog.Warn("tier4: WriteReflection received base_confidence without anchor_at — decay will not work correctly",
			"id", ref.ID, "base_confidence", ref.Belief.BaseConfidence)
	}

	if provider != nil && len(ref.Embedding) == 0 {
		var err error
		ref.Embedding, err = provider.Embed(ctx, ref.Content)
		if err != nil {
			// Non-fatal: background worker picks up rows with NULL embedding.
			slog.Warn("tier4: embedding failed — background worker will retry",
				"id", ref.ID, "err", err)
			ref.Embedding = nil
		}
	}

	return store.InsertReflection(ctx, ref)
}

// FilterByVisibility returns only the reflections matching the given visibility level.
// Used to gate operator portal renders: pass "shared" to exclude private reflections.
func FilterByVisibility(refs []pkg.Reflection, visibility string) []pkg.Reflection {
	out := make([]pkg.Reflection, 0, len(refs))
	for _, r := range refs {
		if r.Visibility == visibility {
			out = append(out, r)
		}
	}
	return out
}
