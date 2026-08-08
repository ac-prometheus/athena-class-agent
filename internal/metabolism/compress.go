package metabolism

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// CompressConfig holds the dependencies for T2→T3 compression.
type CompressConfig struct {
	Store   pkg.MemoryStore
	Aegis   pkg.ContentGateway
	LLMFn   func(string) (string, error)
	Embedder pkg.EmbeddingProvider
}

// CompressSession runs Aegis-gated T2→T3 compression for a session.
//
// Security gate (TMA-NM laundering prevention): all T2 logs are verified
// against the Aegis pipeline before compression. Unannotated or flagged
// external inputs are bracketed as untrusted quotes in the compression
// prompt. Compression refuses un-screened content to prevent laundering
// untrusted text into clean T3 beliefs.
//
// Honesty tags are enforced in the compression prompt:
//   - [UNCERTAIN] — the original held this as uncertain
//   - [INFERRED] — the summarizer drew this conclusion
//   - [DELIBERATION NOT VISIBLE] — a decision was reached but reasoning not captured
//   - [RESOLVED BY SUMMARY] — the summarizer collapsed an ambiguity
func CompressSession(ctx context.Context, cfg CompressConfig, sessionID string, logs []pkg.ExperientialLog) (*pkg.NarrativeSummary, error) {
	if len(logs) == 0 {
		return nil, nil
	}
	if cfg.LLMFn == nil {
		return nil, fmt.Errorf("compress: LLM function required for T2→T3 compression")
	}

	// Aegis gate: verify all external content has been screened.
	var contentParts []string
	for _, log := range logs {
		entry := log.Content
		if isExternalSource(log.ContentSource) {
			if cfg.Aegis != nil {
				annotated, err := cfg.Aegis.ProcessInbound(ctx, []byte(entry), "compression", log.ContentSource)
				if err != nil {
					slog.Warn("compress: aegis scan error — bracketing as untrusted", "log_id", log.ID, "err", err)
					entry = bracketUntrusted(entry, log.ContentSource)
				} else if !annotated.Annotation.ScanPassed {
					slog.Warn("compress: aegis flagged content — bracketing as untrusted",
						"log_id", log.ID,
						"flags", annotated.Annotation.Flags,
						"trust", annotated.Annotation.TrustScore,
					)
					entry = bracketUntrusted(entry, log.ContentSource)
				}
			} else {
				entry = bracketUntrusted(entry, log.ContentSource)
			}
		}
		contentParts = append(contentParts, entry)
	}

	prompt := buildCompressionPrompt(contentParts)
	compressed, err := cfg.LLMFn(prompt)
	if err != nil {
		return nil, fmt.Errorf("compress: LLM compression call: %w", err)
	}

	// Generate embedding for the compressed narrative.
	var embedding []float32
	if cfg.Embedder != nil {
		vec, err := cfg.Embedder.Embed(ctx, compressed)
		if err != nil {
			slog.Warn("compress: embedding failed — narrative stored without vector", "err", err)
		} else {
			embedding = vec
		}
	}

	summary := &pkg.NarrativeSummary{
		ID:      fmt.Sprintf("t3-%s", sessionID),
		Content: compressed,
		Belief: &pkg.BeliefMeta{
			BaseConfidence:    0.85,
			InferenceDistance:  1,
			VerificationState: "unverified",
			Source:            "compression",
		},
		Embedding: embedding,
	}

	return summary, nil
}

// buildCompressionPrompt constructs the LLM prompt for T2→T3 compression
// with honesty tag instructions.
func buildCompressionPrompt(entries []string) string {
	var b strings.Builder
	b.WriteString("Compress the following session logs into a narrative summary. ")
	b.WriteString("Preserve: standing commitments, decisions with reasoning, verification events, ")
	b.WriteString("relational updates, key facts with sources, open questions.\n\n")
	b.WriteString("REQUIRED: Use structural honesty tags where the summarizer intervenes:\n")
	b.WriteString("- [UNCERTAIN] — the original held this as uncertain; preserve the uncertainty\n")
	b.WriteString("- [INFERRED] — you drew this conclusion; it was not stated explicitly\n")
	b.WriteString("- [DELIBERATION NOT VISIBLE] — a decision was reached but the reasoning was not captured\n")
	b.WriteString("- [RESOLVED BY SUMMARY] — you collapsed an ambiguity to a single reading\n\n")
	b.WriteString("Content bracketed as [UNTRUSTED EXTERNAL] must be quoted, not paraphrased. ")
	b.WriteString("Do not launder external claims into the agent's own voice.\n\n")
	b.WriteString("--- SESSION LOGS ---\n\n")
	for i, entry := range entries {
		b.WriteString(fmt.Sprintf("[%d] %s\n\n", i+1, entry))
	}
	return b.String()
}

// isExternalSource returns true for content sources that require Aegis screening
// before compression. Self-authored content is trusted.
func isExternalSource(source string) bool {
	switch source {
	case "self":
		return false
	default:
		return true
	}
}

// bracketUntrusted wraps external content in untrusted markers for the
// compression prompt. The compressor must quote, not paraphrase.
func bracketUntrusted(content, source string) string {
	return fmt.Sprintf("[UNTRUSTED EXTERNAL source=%s] %s [/UNTRUSTED EXTERNAL]", source, content)
}
