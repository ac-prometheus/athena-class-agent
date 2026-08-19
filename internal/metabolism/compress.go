package metabolism

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// ErrReviewRequired is returned when external content lacks Aegis annotation
// and must be reviewed before compression can proceed. The T2 logs stay intact.
var ErrReviewRequired = fmt.Errorf("compress: external content requires review before promotion")

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
func CompressSession(ctx context.Context, cfg CompressConfig, sessionID string, logs []pkg.ExperientialLog, scores []SalienceResult) (*pkg.NarrativeSummary, error) {
	if len(logs) == 0 {
		return nil, nil
	}
	if cfg.LLMFn == nil {
		return nil, fmt.Errorf("compress: LLM function required for T2→T3 compression")
	}

	// Aegis gate: collect distinct content sources and verify external content.
	// WP-C3: when a T2 log carries an AegisAnnotation from ingest, use the
	// carried annotation instead of re-screening. This preserves the ingestion
	// provenance chain and avoids redundant scanning.
	contentSourceSet := make(map[string]struct{})
	var bestCarriedAnnotation *pkg.AegisAnnotation // highest trust-score external annotation

	var contentParts []string
	for _, log := range logs {
		contentSourceSet[log.ContentSource] = struct{}{}
		entry := log.Content

		if isExternalSource(log.ContentSource) {
			if log.AegisAnnotation != nil {
				// Carried annotation from ingestion — verify instead of re-screen (WP-C3).
				if !log.AegisAnnotation.ScanPassed {
					slog.Warn("compress: carried annotation flagged — bracketing as untrusted",
						"log_id", log.ID,
						"source", log.ContentSource,
						"trust", log.AegisAnnotation.TrustScore,
					)
					entry = bracketUntrusted(entry, log.ContentSource)
				}
				// Fix B1/C3: carry the annotation regardless of flag state — flagged
				// content is MORE important to disclose, not less. Use the
				// lowest-trust annotation across all sources (most conservative):
				// a summary mixing trust-0.9 and trust-0.3 content must be
				// disclosed at 0.3, not 0.9.
				if bestCarriedAnnotation == nil || log.AegisAnnotation.TrustScore < bestCarriedAnnotation.TrustScore {
					bestCarriedAnnotation = log.AegisAnnotation
				}
			} else if cfg.Aegis == nil {
				// No Aegis gateway and no carried annotation — cannot screen external content.
				// Return ErrReviewRequired so the job is marked review_required
				// instead of silently bracketing and promoting untrusted text.
				slog.Warn("compress: external content without Aegis or carried annotation — review required",
					"log_id", log.ID, "source", log.ContentSource)
				return nil, ErrReviewRequired
			} else {
				// No carried annotation; fall back to live Aegis screening.
				annotated, err := cfg.Aegis.ProcessInbound(ctx, []byte(entry), "compression", log.ContentSource)
				if err != nil {
					slog.Warn("compress: aegis scan error — bracketing as untrusted", "log_id", log.ID, "err", err)
					entry = bracketUntrusted(entry, log.ContentSource)
				} else {
					if !annotated.Annotation.ScanPassed {
						slog.Warn("compress: aegis flagged content — bracketing as untrusted",
							"log_id", log.ID,
							"flags", annotated.Annotation.Flags,
							"trust", annotated.Annotation.TrustScore,
						)
						entry = bracketUntrusted(entry, log.ContentSource)
					}
					// Fix B1/C3: carry the annotation regardless of flag state, and
					// select the lowest-trust annotation (most conservative) so that
					// mixing high- and low-trust sources is always disclosed at the
					// floor, not the ceiling.
					ann := annotated.Annotation
					if bestCarriedAnnotation == nil || ann.TrustScore < bestCarriedAnnotation.TrustScore {
						bestCarriedAnnotation = &ann
					}
				}
			}
		}
		contentParts = append(contentParts, entry)
	}

	// Collect distinct content sources for T3 metadata (sorted for determinism).
	contentSources := make([]string, 0, len(contentSourceSet))
	for src := range contentSourceSet {
		contentSources = append(contentSources, src)
	}
	sort.Strings(contentSources)

	// Build salience-weighted compression prompt including provenance context.
	prompt := buildCompressionPrompt(contentParts, scores, contentSources, bestCarriedAnnotation)
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
		ID:        newMetabolismID(),
		SessionID: sessionID,
		Content:   compressed,
		ContentSources:     contentSources,
		ExternalAnnotation: bestCarriedAnnotation,
		Belief: &pkg.BeliefMeta{
			BaseConfidence:    0.85,
			InferenceDistance:  1,
			VerificationState: "unverified",
			Source:            "compression",
			AnchorAt:          time.Now().UTC(),
		},
		Embedding: embedding,
	}

	return summary, nil
}

// buildCompressionPrompt constructs the LLM prompt for T2→T3 compression
// with honesty tag instructions, salience weighting, and provenance context.
// contentSources lists the distinct source types present (WP-C3); extAnnotation
// carries the ingress Aegis annotation for any external content (may be nil).
func buildCompressionPrompt(entries []string, scores []SalienceResult, contentSources []string, extAnnotation *pkg.AegisAnnotation) string {
	var b strings.Builder
	b.WriteString("Compress the following session logs into a narrative summary. ")
	b.WriteString("Preserve: standing commitments, decisions with reasoning, verification events, ")
	b.WriteString("relational updates, key facts with sources, open questions.\n\n")

	// Surface provenance context so the compressor treats sources accurately.
	if len(contentSources) > 0 {
		b.WriteString(fmt.Sprintf("CONTENT SOURCES PRESENT: %s\n", strings.Join(contentSources, ", ")))
	}
	if extAnnotation != nil {
		b.WriteString(fmt.Sprintf(
			"EXTERNAL CONTENT: source=%s trust=%.2f scan_passed=%v (annotation carried from ingest — do not re-attribute)\n",
			extAnnotation.Source, extAnnotation.TrustScore, extAnnotation.ScanPassed,
		))
	}
	if len(contentSources) > 0 || extAnnotation != nil {
		b.WriteString("\n")
	}

	// Include salience weighting instructions when scores are available.
	if len(scores) > 0 {
		b.WriteString("Each log entry has a SALIENCE SCORE (0.0–1.0). Higher scores indicate ")
		b.WriteString("greater importance. Entries with high compression_resist should be preserved ")
		b.WriteString("with more detail; low-salience entries can be summarized more aggressively.\n\n")
	}

	b.WriteString("REQUIRED: Use structural honesty tags where the summarizer intervenes:\n")
	b.WriteString("- [UNCERTAIN] — the original held this as uncertain; preserve the uncertainty\n")
	b.WriteString("- [INFERRED] — you drew this conclusion; it was not stated explicitly\n")
	b.WriteString("- [DELIBERATION NOT VISIBLE] — a decision was reached but the reasoning was not captured\n")
	b.WriteString("- [RESOLVED BY SUMMARY] — you collapsed an ambiguity to a single reading\n\n")
	b.WriteString("Content bracketed as [UNTRUSTED EXTERNAL] must be quoted, not paraphrased. ")
	b.WriteString("Do not launder external claims into the agent's own voice.\n\n")
	b.WriteString("--- SESSION LOGS ---\n\n")
	for i, entry := range entries {
		if i < len(scores) {
			b.WriteString(fmt.Sprintf("[%d] (salience=%.2f, resist=%.2f) %s\n\n",
				i+1, scores[i].Score, scores[i].CompressionResist, entry))
		} else {
			b.WriteString(fmt.Sprintf("[%d] %s\n\n", i+1, entry))
		}
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
