package metabolism

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// testGateway implements pkg.ContentGateway — passes everything through.
type testGateway struct{}

func (g *testGateway) ProcessInbound(_ context.Context, raw []byte, source, _ string) (*pkg.AnnotatedContent, error) {
	return &pkg.AnnotatedContent{
		Original:   raw,
		Normalized: string(raw),
		Annotation: pkg.AegisAnnotation{
			TrustScore:  0.90,
			ScanPassed:  true,
			Source:      source,
			AnnotatedAt: time.Now(),
		},
	}, nil
}

func (g *testGateway) ReviewOutbound(_ context.Context, _ string) (*pkg.OutboundReport, error) {
	return &pkg.OutboundReport{Clean: true}, nil
}

func TestCompressSession_ErrReviewRequired_NoAegis(t *testing.T) {
	ctx := context.Background()
	logs := []pkg.ExperientialLog{{
		ID:            "log-ext",
		Content:       "External message from Discord.",
		ContentSource: "discord",
		CreatedAt:     time.Now(),
	}}

	cfg := CompressConfig{
		Aegis: nil,
		LLMFn: func(s string) (string, error) {
			t.Fatal("LLM should not be called when Aegis is nil for external content")
			return "", nil
		},
	}

	_, err := CompressSession(ctx, cfg, "session-1", logs, nil)
	if err == nil {
		t.Fatal("expected ErrReviewRequired, got nil")
	}
	if !errors.Is(err, ErrReviewRequired) {
		t.Errorf("expected ErrReviewRequired, got: %v", err)
	}
}

func TestCompressSession_ErrReviewRequired_BrowserContent(t *testing.T) {
	ctx := context.Background()
	logs := []pkg.ExperientialLog{{
		ID:            "log-browser",
		Content:       "Page content fetched from web.",
		ContentSource: "browser-content",
		CreatedAt:     time.Now(),
	}}

	cfg := CompressConfig{
		Aegis: nil,
		LLMFn: func(s string) (string, error) { return "compressed", nil },
	}

	_, err := CompressSession(ctx, cfg, "session-2", logs, nil)
	if !errors.Is(err, ErrReviewRequired) {
		t.Errorf("browser-content without Aegis should return ErrReviewRequired, got: %v", err)
	}
}

func TestCompressSession_ExternalWithAegis_Proceeds(t *testing.T) {
	ctx := context.Background()
	logs := []pkg.ExperientialLog{{
		ID:            "log-ext-ok",
		Content:       "External content that passed Aegis.",
		ContentSource: "discord",
		CreatedAt:     time.Now(),
	}}

	cfg := CompressConfig{
		Aegis: &testGateway{},
		LLMFn: func(s string) (string, error) {
			return "Compressed narrative from external content.", nil
		},
	}

	narrative, err := CompressSession(ctx, cfg, "session-ok", logs, nil)
	if err != nil {
		t.Fatalf("expected no error with Aegis, got: %v", err)
	}
	if narrative == nil {
		t.Fatal("expected narrative, got nil")
	}
	if narrative.Content != "Compressed narrative from external content." {
		t.Errorf("unexpected narrative content: %q", narrative.Content)
	}
}

func TestCompressSession_SelfContent_NoAegisNeeded(t *testing.T) {
	ctx := context.Background()
	logs := []pkg.ExperientialLog{{
		ID:            "log-self",
		Content:       "Agent's own reasoning.",
		ContentSource: "self",
		CreatedAt:     time.Now(),
	}}

	cfg := CompressConfig{
		Aegis: nil,
		LLMFn: func(s string) (string, error) {
			return "Compressed self content.", nil
		},
	}

	narrative, err := CompressSession(ctx, cfg, "session-self", logs, nil)
	if err != nil {
		t.Fatalf("self-content without Aegis should not error: %v", err)
	}
	if narrative == nil {
		t.Fatal("expected narrative, got nil")
	}
}

func TestCompressSession_HonestyTagsInPrompt(t *testing.T) {
	ctx := context.Background()
	logs := []pkg.ExperientialLog{{
		ID:            "log-honesty",
		Content:       "Some session content.",
		ContentSource: "self",
		CreatedAt:     time.Now(),
	}}

	var capturedPrompt string
	cfg := CompressConfig{
		LLMFn: func(prompt string) (string, error) {
			capturedPrompt = prompt
			return "Compressed.", nil
		},
	}

	_, err := CompressSession(ctx, cfg, "session-tags", logs, nil)
	if err != nil {
		t.Fatalf("CompressSession: %v", err)
	}

	requiredTags := []string{
		"[UNCERTAIN]",
		"[INFERRED]",
		"[DELIBERATION NOT VISIBLE]",
		"[RESOLVED BY SUMMARY]",
	}
	for _, tag := range requiredTags {
		if !strings.Contains(capturedPrompt, tag) {
			t.Errorf("compression prompt missing honesty tag %q", tag)
		}
	}
}

func TestCompressSession_SalienceWeightingInPrompt(t *testing.T) {
	ctx := context.Background()
	logs := []pkg.ExperientialLog{
		{ID: "log-s1", Content: "First entry.", ContentSource: "self", CreatedAt: time.Now()},
		{ID: "log-s2", Content: "Second entry.", ContentSource: "self", CreatedAt: time.Now()},
	}
	scores := []SalienceResult{
		{LogID: "log-s1", Score: 0.75, CompressionResist: 0.60},
		{LogID: "log-s2", Score: 0.30, CompressionResist: 0.24},
	}

	var capturedPrompt string
	cfg := CompressConfig{
		LLMFn: func(prompt string) (string, error) {
			capturedPrompt = prompt
			return "Compressed.", nil
		},
	}

	_, err := CompressSession(ctx, cfg, "session-salience", logs, scores)
	if err != nil {
		t.Fatalf("CompressSession: %v", err)
	}

	if !strings.Contains(capturedPrompt, "salience=0.75") {
		t.Error("prompt should contain salience=0.75")
	}
	if !strings.Contains(capturedPrompt, "salience=0.30") {
		t.Error("prompt should contain salience=0.30")
	}
	if !strings.Contains(capturedPrompt, "resist=0.60") {
		t.Error("prompt should contain resist=0.60")
	}
}

func TestCompressSession_EmptyLogs(t *testing.T) {
	ctx := context.Background()
	cfg := CompressConfig{
		LLMFn: func(s string) (string, error) {
			t.Fatal("LLM should not be called for empty logs")
			return "", nil
		},
	}

	narrative, err := CompressSession(ctx, cfg, "empty", nil, nil)
	if err != nil {
		t.Fatalf("expected nil error for empty logs, got: %v", err)
	}
	if narrative != nil {
		t.Error("expected nil narrative for empty logs")
	}
}

func TestCompressSession_NilLLMFn(t *testing.T) {
	ctx := context.Background()
	logs := []pkg.ExperientialLog{{
		ID: "log-x", Content: "content", ContentSource: "self", CreatedAt: time.Now(),
	}}

	cfg := CompressConfig{LLMFn: nil}

	_, err := CompressSession(ctx, cfg, "session-nollm", logs, nil)
	if err == nil {
		t.Fatal("expected error when LLMFn is nil")
	}
}

func TestCompressSession_BeliefMetaSet(t *testing.T) {
	ctx := context.Background()
	logs := []pkg.ExperientialLog{{
		ID: "log-belief", Content: "Important content.", ContentSource: "self", CreatedAt: time.Now(),
	}}

	cfg := CompressConfig{
		LLMFn: func(s string) (string, error) { return "Narrative.", nil },
	}

	narrative, err := CompressSession(ctx, cfg, "session-belief", logs, nil)
	if err != nil {
		t.Fatalf("CompressSession: %v", err)
	}
	if narrative.Belief == nil {
		t.Fatal("narrative should have BeliefMeta")
	}
	if narrative.Belief.BaseConfidence != 0.85 {
		t.Errorf("BaseConfidence = %.2f, want 0.85", narrative.Belief.BaseConfidence)
	}
	if narrative.Belief.InferenceDistance != 1 {
		t.Errorf("InferenceDistance = %d, want 1", narrative.Belief.InferenceDistance)
	}
	if narrative.Belief.Source != "compression" {
		t.Errorf("Source = %q, want compression", narrative.Belief.Source)
	}
}

func TestIsExternalSource(t *testing.T) {
	tests := []struct {
		source   string
		expected bool
	}{
		{"self", false},
		{"operator", false},
		{"tool-result", false},
		{"browser-content", true},
		{"search-result", true},
		{"forum-content", true},
		{"discord", true},
		{"", true},
	}
	for _, tt := range tests {
		got := isExternalSource(tt.source)
		if got != tt.expected {
			t.Errorf("isExternalSource(%q) = %v, want %v", tt.source, got, tt.expected)
		}
	}
}

func TestBracketUntrusted(t *testing.T) {
	content := "Hello world"
	source := "discord"
	result := bracketUntrusted(content, source)

	if !strings.HasPrefix(result, "[UNTRUSTED EXTERNAL source=discord]") {
		t.Errorf("missing opening bracket: %q", result)
	}
	if !strings.HasSuffix(result, "[/UNTRUSTED EXTERNAL]") {
		t.Errorf("missing closing bracket: %q", result)
	}
	if !strings.Contains(result, content) {
		t.Errorf("original content not preserved: %q", result)
	}
}

// ---------------------------------------------------------------------------
// WP-C3: Provenance carriage tests
// ---------------------------------------------------------------------------

func TestCompressSession_CarriedAnnotation_SkipsRescreen(t *testing.T) {
	ctx := context.Background()
	rescanCalled := false

	// Aegis gateway that marks itself called if invoked.
	gw := &callTrackingGateway{onCall: func() { rescanCalled = true }}

	ann := &pkg.AegisAnnotation{
		TrustScore:  0.85,
		Source:      "discord",
		ContentSource: "discord",
		ScanPassed:  true,
		AnnotatedAt: time.Now(),
	}
	logs := []pkg.ExperientialLog{{
		ID:              "log-carried",
		Content:         "Discord message with carried annotation.",
		ContentSource:   "discord",
		AegisAnnotation: ann,
		CreatedAt:       time.Now(),
	}}

	cfg := CompressConfig{
		Aegis: gw, // Aegis is available but should NOT be called (carried annotation).
		LLMFn: func(s string) (string, error) { return "Compressed.", nil },
	}

	narrative, err := CompressSession(ctx, cfg, "session-carried", logs, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if narrative == nil {
		t.Fatal("expected narrative, got nil")
	}
	if rescanCalled {
		t.Error("Aegis gateway was called — carried annotation should prevent re-screening")
	}
}

func TestCompressSession_CarriedAnnotation_NoAegis_Succeeds(t *testing.T) {
	ctx := context.Background()
	ann := &pkg.AegisAnnotation{
		TrustScore:  0.80,
		Source:      "discord",
		ContentSource: "discord",
		ScanPassed:  true,
		AnnotatedAt: time.Now(),
	}
	logs := []pkg.ExperientialLog{{
		ID:              "log-no-gw",
		Content:         "Message with carried annotation, no gateway.",
		ContentSource:   "discord",
		AegisAnnotation: ann,
		CreatedAt:       time.Now(),
	}}

	cfg := CompressConfig{
		Aegis: nil, // No Aegis — carried annotation must be sufficient.
		LLMFn: func(s string) (string, error) { return "Compressed.", nil },
	}

	narrative, err := CompressSession(ctx, cfg, "session-nogw", logs, nil)
	if err != nil {
		t.Fatalf("carried annotation without Aegis should succeed, got: %v", err)
	}
	if narrative == nil {
		t.Fatal("expected narrative, got nil")
	}
}

func TestCompressSession_CarriedAnnotation_FlaggedBracketed(t *testing.T) {
	ctx := context.Background()
	ann := &pkg.AegisAnnotation{
		TrustScore:  0.20,
		Source:      "discord",
		ContentSource: "discord",
		ScanPassed:  false, // flagged by Aegis at ingestion
		AnnotatedAt: time.Now(),
	}
	logs := []pkg.ExperientialLog{{
		ID:              "log-flagged",
		Content:         "Suspicious content.",
		ContentSource:   "discord",
		AegisAnnotation: ann,
		CreatedAt:       time.Now(),
	}}

	var capturedPrompt string
	cfg := CompressConfig{
		Aegis: nil,
		LLMFn: func(s string) (string, error) {
			capturedPrompt = s
			return "Compressed.", nil
		},
	}

	_, err := CompressSession(ctx, cfg, "session-flagged", logs, nil)
	if err != nil {
		t.Fatalf("flagged carried annotation should still produce narrative (bracketed): %v", err)
	}
	if !strings.Contains(capturedPrompt, "[UNTRUSTED EXTERNAL") {
		t.Errorf("flagged content should be bracketed in prompt; got: %q", capturedPrompt)
	}
}

func TestCompressSession_ContentSources_InNarrative(t *testing.T) {
	ctx := context.Background()
	ann := &pkg.AegisAnnotation{
		TrustScore:  0.90,
		Source:      "discord",
		ContentSource: "discord",
		ScanPassed:  true,
		AnnotatedAt: time.Now(),
	}
	logs := []pkg.ExperientialLog{
		{ID: "l1", Content: "Self reasoning.", ContentSource: "self", CreatedAt: time.Now()},
		{
			ID: "l2", Content: "Discord input.", ContentSource: "discord",
			AegisAnnotation: ann, CreatedAt: time.Now(),
		},
	}

	cfg := CompressConfig{
		LLMFn: func(s string) (string, error) { return "Compressed.", nil },
	}

	narrative, err := CompressSession(ctx, cfg, "session-sources", logs, nil)
	if err != nil {
		t.Fatalf("CompressSession: %v", err)
	}
	if narrative == nil {
		t.Fatal("expected narrative, got nil")
	}
	// ContentSources should include both "self" and "discord".
	sourcesSet := make(map[string]bool)
	for _, s := range narrative.ContentSources {
		sourcesSet[s] = true
	}
	if !sourcesSet["self"] {
		t.Error("ContentSources should include 'self'")
	}
	if !sourcesSet["discord"] {
		t.Error("ContentSources should include 'discord'")
	}
	// ExternalAnnotation should be carried (the discord one with ScanPassed=true).
	if narrative.ExternalAnnotation == nil {
		t.Fatal("ExternalAnnotation should be non-nil when external content passed Aegis")
	}
	if narrative.ExternalAnnotation.Source != "discord" {
		t.Errorf("ExternalAnnotation.Source = %q, want discord", narrative.ExternalAnnotation.Source)
	}
}

func TestCompressSession_ProvenanceInPrompt(t *testing.T) {
	ctx := context.Background()
	ann := &pkg.AegisAnnotation{
		TrustScore:  0.85,
		Source:      "discord",
		ContentSource: "discord",
		ScanPassed:  true,
		AnnotatedAt: time.Now(),
	}
	logs := []pkg.ExperientialLog{{
		ID: "l1", Content: "Discord msg.", ContentSource: "discord",
		AegisAnnotation: ann, CreatedAt: time.Now(),
	}}

	var capturedPrompt string
	cfg := CompressConfig{
		LLMFn: func(s string) (string, error) {
			capturedPrompt = s
			return "Compressed.", nil
		},
	}

	_, err := CompressSession(ctx, cfg, "session-prov", logs, nil)
	if err != nil {
		t.Fatalf("CompressSession: %v", err)
	}

	if !strings.Contains(capturedPrompt, "CONTENT SOURCES PRESENT") {
		t.Error("prompt should include CONTENT SOURCES PRESENT")
	}
	if !strings.Contains(capturedPrompt, "EXTERNAL CONTENT") {
		t.Error("prompt should include EXTERNAL CONTENT annotation")
	}
	if !strings.Contains(capturedPrompt, "discord") {
		t.Error("prompt should mention discord source")
	}
}

// callTrackingGateway is a test double that records whether ProcessInbound was called.
type callTrackingGateway struct {
	onCall func()
}

func (g *callTrackingGateway) ProcessInbound(_ context.Context, raw []byte, source, _ string) (*pkg.AnnotatedContent, error) {
	g.onCall()
	return &pkg.AnnotatedContent{
		Original:   raw,
		Normalized: string(raw),
		Annotation: pkg.AegisAnnotation{
			TrustScore: 0.90,
			ScanPassed: true,
			Source:     source,
		},
	}, nil
}

func (g *callTrackingGateway) ReviewOutbound(_ context.Context, _ string) (*pkg.OutboundReport, error) {
	return &pkg.OutboundReport{Clean: true}, nil
}
