package aegis

import (
	"context"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// NoOpGateway is a ContentGateway that passes everything through without
// scanning or scoring. Use it as the default when no Aegis pipeline is configured.
type NoOpGateway struct{}

// ProcessInbound returns the content unmodified with a neutral annotation.
func (n *NoOpGateway) ProcessInbound(_ context.Context, raw []byte, source, contentSource string) (*pkg.AnnotatedContent, error) {
	return &pkg.AnnotatedContent{
		Original:   raw,
		Normalized: string(raw),
		Annotation: pkg.AegisAnnotation{
			TrustScore:    1.0,
			Source:        source,
			ContentSource: contentSource,
			ScanPassed:    true,
			AnnotatedAt:   time.Now().UTC(),
		},
	}, nil
}

// ReviewOutbound returns a clean report with no findings.
func (n *NoOpGateway) ReviewOutbound(_ context.Context, _ string) (*pkg.OutboundReport, error) {
	return &pkg.OutboundReport{Clean: true}, nil
}

// Compile-time interface check.
var _ pkg.ContentGateway = (*NoOpGateway)(nil)
