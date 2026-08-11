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
		{"operator", true},
		{"tool-result", true},
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
