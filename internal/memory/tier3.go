package memory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// honestyTags are injected into compression prompts to guide the LLM in
// marking epistemic status per-claim. Tags are part of the stored content.
var honestyTags = []string{
	"[UNCERTAIN]",
	"[INFERRED]",
	"[DELIBERATION NOT VISIBLE]",
	"[RESOLVED BY SUMMARY]",
}

// compressionPrompt builds the LLM prompt for T2→T3 compression.
// Tag instructions come first so they influence the entire summary.
func compressionPrompt(sessionID string, entries []pkg.ExperientialLog) string {
	var sb strings.Builder
	sb.WriteString("Compress the following session log into a concise narrative summary.\n")
	sb.WriteString("Apply per-claim honesty tags where appropriate:\n")
	sb.WriteString("  [UNCERTAIN]              — claim is held with low confidence\n")
	sb.WriteString("  [INFERRED]               — claim was derived, not directly observed\n")
	sb.WriteString("  [DELIBERATION NOT VISIBLE] — reasoning steps were not logged\n")
	sb.WriteString("  [RESOLVED BY SUMMARY]    — conflicting entries collapsed to a single claim\n\n")
	sb.WriteString(fmt.Sprintf("Session ID: %s\n\n", sessionID))
	sb.WriteString("Log entries (oldest first):\n")
	for i, e := range entries {
		sb.WriteString(fmt.Sprintf("[%d] source=%s\n%s\n\n", i+1, e.ContentSource, e.Content))
	}
	sb.WriteString("\nProduce a coherent narrative. Apply tags inline, after each claim they qualify.")
	return sb.String()
}

// countHonestyTags counts all honesty tag occurrences in content.
func countHonestyTags(content string) map[string]int {
	counts := make(map[string]int, len(honestyTags))
	for _, tag := range honestyTags {
		counts[tag] = strings.Count(content, tag)
	}
	return counts
}

// CompressSession runs the T2→T3 compression pipeline for a session.
// Entries are grouped conceptually by the LLM prompt; the result is embedded
// and stored as a NarrativeSummary. Returns the inserted summary for edge creation.
func CompressSession(
	ctx context.Context,
	store pkg.MemoryStore,
	provider pkg.EmbeddingProvider,
	sessionID string,
	entries []pkg.ExperientialLog,
	llmFn func(string) (string, error),
) (*pkg.NarrativeSummary, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("tier3: no entries to compress for session %s", sessionID)
	}

	prompt := compressionPrompt(sessionID, entries)
	summaryText, err := llmFn(prompt)
	if err != nil {
		return nil, fmt.Errorf("tier3: LLM compression failed for session %s: %w", sessionID, err)
	}

	tagCounts := countHonestyTags(summaryText)
	totalTags := 0
	for _, n := range tagCounts {
		totalTags += n
	}
	slog.Info("tier3: compressed session",
		"session_id", sessionID,
		"entries", len(entries),
		"honesty_tags", totalTags,
	)

	belief := &pkg.BeliefMeta{
		BaseConfidence:    0.85,
		AnchorAt:          time.Now().UTC(),
		InferenceDistance: 1,
		VerificationState: "unverified",
		Source:            "compression",
	}

	var embedding []float32
	if provider != nil {
		embedding, err = provider.Embed(ctx, summaryText)
		if err != nil {
			// Embedding failure is non-fatal; the background worker will retry.
			slog.Warn("tier3: embedding failed — will retry via background worker",
				"session_id", sessionID, "err", err)
		}
	}

	summary := pkg.NarrativeSummary{
		ID:        newID(),
		Content:   summaryText,
		Belief:    belief,
		Embedding: embedding,
	}

	if err := store.InsertNarrative(ctx, summary); err != nil {
		return nil, fmt.Errorf("tier3: inserting narrative summary for session %s: %w", sessionID, err)
	}

	return &summary, nil
}

// SurfaceNarrative appends a provenance footer when surfacing a T3 summary.
// The footer is NOT stored in the database — it is computed at read time to
// avoid polluting the canonical content used for embedding and re-compression.
func SurfaceNarrative(summary pkg.NarrativeSummary, model string) string {
	tagCounts := countHonestyTags(summary.Content)

	var tagParts []string
	for _, tag := range honestyTags {
		if n := tagCounts[tag]; n > 0 {
			tagParts = append(tagParts, fmt.Sprintf("%d %s", n, tag))
		}
	}

	footer := fmt.Sprintf("\n\n---\n[compiled by %s; tags: %s]",
		model,
		strings.Join(tagParts, ", "),
	)
	return summary.Content + footer
}
