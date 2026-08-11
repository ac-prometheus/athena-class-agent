package metabolism

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

func TestHeuristicScorer_BaseScore(t *testing.T) {
	scorer := NewHeuristicScorer()
	logs := []pkg.ExperientialLog{{
		ID:            "log-base",
		Content:       "Nothing special here.",
		ContentSource: "self",
		CreatedAt:     time.Now(),
	}}

	results, err := scorer.ScoreLogs(context.Background(), logs)
	if err != nil {
		t.Fatalf("ScoreLogs: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Score != 0.25 {
		t.Errorf("base score = %.2f, want 0.25", results[0].Score)
	}
}

func TestHeuristicScorer_EmptyContent(t *testing.T) {
	scorer := NewHeuristicScorer()
	logs := []pkg.ExperientialLog{{
		ID:            "log-empty",
		Content:       "",
		ContentSource: "self",
		CreatedAt:     time.Now(),
	}}

	results, err := scorer.ScoreLogs(context.Background(), logs)
	if err != nil {
		t.Fatalf("ScoreLogs: %v", err)
	}
	if results[0].Score != 0.25 {
		t.Errorf("empty content score = %.2f, want 0.25", results[0].Score)
	}
}

func TestHeuristicScorer_SingleKeyword(t *testing.T) {
	scorer := NewHeuristicScorer()
	logs := []pkg.ExperientialLog{{
		ID:            "log-kw",
		Content:       "I realized something today.",
		ContentSource: "self",
		CreatedAt:     time.Now(),
	}}

	results, err := scorer.ScoreLogs(context.Background(), logs)
	if err != nil {
		t.Fatalf("ScoreLogs: %v", err)
	}
	if results[0].Score != 0.35 {
		t.Errorf("single keyword score = %.2f, want 0.35 (0.25 + 0.10)", results[0].Score)
	}
}

func TestHeuristicScorer_MultipleKeywordsCapped(t *testing.T) {
	scorer := NewHeuristicScorer()
	// 6 keywords × 0.10 = 0.60, but capped at 0.45
	logs := []pkg.ExperientialLog{{
		ID:            "log-multi",
		Content:       "I realized and discovered for the first time that I decided something important — a breakthrough.",
		ContentSource: "self",
		CreatedAt:     time.Now(),
	}}

	results, err := scorer.ScoreLogs(context.Background(), logs)
	if err != nil {
		t.Fatalf("ScoreLogs: %v", err)
	}
	// 0.25 base + 0.45 capped keywords = 0.70
	if results[0].Score != 0.70 {
		t.Errorf("capped keyword score = %.2f, want 0.70", results[0].Score)
	}
}

func TestHeuristicScorer_SecurityKeyword(t *testing.T) {
	scorer := NewHeuristicScorer()
	logs := []pkg.ExperientialLog{{
		ID:            "log-sec",
		Content:       "Found a credentials leak in the config.",
		ContentSource: "self",
		CreatedAt:     time.Now(),
	}}

	results, err := scorer.ScoreLogs(context.Background(), logs)
	if err != nil {
		t.Fatalf("ScoreLogs: %v", err)
	}
	// 0.25 base + 0.30 security = 0.55
	if results[0].Score != 0.55 {
		t.Errorf("security score = %.2f, want 0.55", results[0].Score)
	}
}

func TestHeuristicScorer_CappedAt090(t *testing.T) {
	scorer := NewHeuristicScorer()
	// Keywords (6 → capped 0.45) + security (0.30) + base (0.25) = 1.00 → capped 0.90
	logs := []pkg.ExperientialLog{{
		ID:            "log-cap",
		Content:       "I realized and discovered for the first time I decided something important — a breakthrough. Also found a security issue.",
		ContentSource: "self",
		CreatedAt:     time.Now(),
	}}

	results, err := scorer.ScoreLogs(context.Background(), logs)
	if err != nil {
		t.Fatalf("ScoreLogs: %v", err)
	}
	if results[0].Score != 0.90 {
		t.Errorf("capped score = %.2f, want 0.90", results[0].Score)
	}
}

func TestHeuristicScorer_LongContent(t *testing.T) {
	scorer := NewHeuristicScorer()
	longContent := strings.Repeat("word ", 200) // >600 chars
	logs := []pkg.ExperientialLog{{
		ID:            "log-long",
		Content:       longContent,
		ContentSource: "self",
		CreatedAt:     time.Now(),
	}}

	results, err := scorer.ScoreLogs(context.Background(), logs)
	if err != nil {
		t.Fatalf("ScoreLogs: %v", err)
	}
	// 0.25 base + 0.05 length = 0.30
	if results[0].Score != 0.30 {
		t.Errorf("long content score = %.2f, want 0.30", results[0].Score)
	}
}

func TestHeuristicScorer_OutcomeKeyword(t *testing.T) {
	scorer := NewHeuristicScorer()
	logs := []pkg.ExperientialLog{{
		ID:            "log-outcome",
		Content:       "The bug was resolved after investigation.",
		ContentSource: "self",
		CreatedAt:     time.Now(),
	}}

	results, err := scorer.ScoreLogs(context.Background(), logs)
	if err != nil {
		t.Fatalf("ScoreLogs: %v", err)
	}
	// 0.25 base + 0.05 outcome = 0.30
	if results[0].Score != 0.30 {
		t.Errorf("outcome score = %.2f, want 0.30", results[0].Score)
	}
}

func TestHeuristicScorer_CompressionResist(t *testing.T) {
	scorer := NewHeuristicScorer()
	logs := []pkg.ExperientialLog{{
		ID:            "log-resist",
		Content:       "I realized something important.",
		ContentSource: "self",
		CreatedAt:     time.Now(),
	}}

	results, err := scorer.ScoreLogs(context.Background(), logs)
	if err != nil {
		t.Fatalf("ScoreLogs: %v", err)
	}
	expected := results[0].Score * 0.80
	if math.Abs(results[0].CompressionResist-expected) > 0.001 {
		t.Errorf("CompressionResist = %.4f, want %.4f (score * 0.80)", results[0].CompressionResist, expected)
	}
}

func TestHeuristicScorer_MultipleLogs(t *testing.T) {
	scorer := NewHeuristicScorer()
	logs := []pkg.ExperientialLog{
		{ID: "log-a", Content: "Nothing here.", ContentSource: "self", CreatedAt: time.Now()},
		{ID: "log-b", Content: "I realized something with security implications.", ContentSource: "self", CreatedAt: time.Now()},
	}

	results, err := scorer.ScoreLogs(context.Background(), logs)
	if err != nil {
		t.Fatalf("ScoreLogs: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Score >= results[1].Score {
		t.Errorf("plain log (%.2f) should score lower than keyword+security log (%.2f)",
			results[0].Score, results[1].Score)
	}
	if results[0].LogID != "log-a" || results[1].LogID != "log-b" {
		t.Errorf("LogIDs should match input order: got %q, %q", results[0].LogID, results[1].LogID)
	}
}

func TestHeuristicScorer_ReasonIncludesKeywords(t *testing.T) {
	scorer := NewHeuristicScorer()
	logs := []pkg.ExperientialLog{{
		ID:            "log-reason",
		Content:       "I realized something about safety.",
		ContentSource: "self",
		CreatedAt:     time.Now(),
	}}

	results, err := scorer.ScoreLogs(context.Background(), logs)
	if err != nil {
		t.Fatalf("ScoreLogs: %v", err)
	}
	reason := results[0].Reason
	if !strings.Contains(reason, "keyword:realized") {
		t.Errorf("reason should mention keyword:realized, got %q", reason)
	}
	if !strings.Contains(reason, "security:safety") {
		t.Errorf("reason should mention security:safety, got %q", reason)
	}
}
