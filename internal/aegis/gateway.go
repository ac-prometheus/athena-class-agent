package aegis

import (
	"bytes"
	"context"
	"log/slog"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// Gateway is the Aegis pipeline entry point.
type Gateway struct {
	trustScorer *TrustScorer
}

// NewGateway constructs a Gateway with the given trust scorer.
func NewGateway(trustScorer *TrustScorer) *Gateway {
	return &Gateway{trustScorer: trustScorer}
}

// ProcessInbound runs raw content through the full Aegis inbound pipeline:
// sanitize → scan → score → annotate.
func (g *Gateway) ProcessInbound(ctx context.Context, raw []byte, source, contentSource string) (*pkg.AnnotatedContent, error) {
	normalized, modified := Sanitize(raw)

	flags := Scan(normalized)

	flagNames := make([]string, len(flags))
	for i, f := range flags {
		flagNames[i] = f.Category + ":" + f.Pattern
	}

	trustScore, err := g.trustScorer.Score(ctx, source, flags)
	if err != nil {
		slog.Warn("aegis: trust scoring error", "source", source, "err", err)
		trustScore = g.trustScorer.cfg.SkepticalPrior
	}

	annotation := pkg.AegisAnnotation{
		TrustScore:    trustScore,
		Source:        source,
		ContentSource: contentSource,
		Flags:         flagNames,
		Sanitized:     modified,
		ScanPassed:    len(flags) == 0,
		AnnotatedAt:   time.Now().UTC(),
	}

	slog.Info("aegis: inbound processed",
		"source", source,
		"trust_score", trustScore,
		"flag_count", len(flags),
		"sanitized", modified,
	)

	return &pkg.AnnotatedContent{
		Original:   bytes.Clone(raw),
		Normalized: normalized,
		Annotation: annotation,
	}, nil
}

// ReviewOutbound scans outgoing content for sensitive data leaks.
// Never blocks — returns report only per autonomy invariants.
func (g *Gateway) ReviewOutbound(ctx context.Context, content string) (*pkg.OutboundReport, error) {
	report := ScanOutbound(content)
	if !report.Clean {
		slog.Warn("aegis: outbound review findings", "count", len(report.Findings))
	}
	return report, nil
}

// Compile-time interface check.
var _ pkg.ContentGateway = (*Gateway)(nil)
