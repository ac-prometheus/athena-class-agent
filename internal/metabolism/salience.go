package metabolism

import (
	"context"
	"strings"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// SalienceScorer assigns importance weights to experiential logs.
type SalienceScorer interface {
	ScoreLogs(ctx context.Context, logs []pkg.ExperientialLog) ([]SalienceResult, error)
}

// SalienceResult is the output of scoring a single log entry.
type SalienceResult struct {
	LogID             string
	Score             float64
	CompressionResist float64
	Reason            string
}

// HeuristicScorer implements SalienceScorer with keyword and structural heuristics.
// From Aurora's brief: base 0.25, keyword signals +0.10 each (capped +0.45),
// content length >600 chars +0.05, outcome resolution +0.05,
// iron-law/security signals +0.30. Cap at 0.90 (1.00 reserved for keeper-flagged).
type HeuristicScorer struct{}

func NewHeuristicScorer() *HeuristicScorer { return &HeuristicScorer{} }

func (s *HeuristicScorer) ScoreLogs(ctx context.Context, logs []pkg.ExperientialLog) ([]SalienceResult, error) {
	results := make([]SalienceResult, len(logs))
	for i, log := range logs {
		score, reason := s.scoreOne(log)
		results[i] = SalienceResult{
			LogID:             log.ID,
			Score:             score,
			CompressionResist: score * 0.80,
			Reason:            reason,
		}
	}
	return results, nil
}

func (s *HeuristicScorer) scoreOne(log pkg.ExperientialLog) (float64, string) {
	score := 0.25
	var reasons []string

	// Keyword signals (+0.10 each, capped at +0.45)
	keywords := []string{
		"realized", "discovered", "first time", "changed position",
		"decided", "important", "breakthrough", "mistake", "learned",
	}
	keywordBonus := 0.0
	content := strings.ToLower(log.Content)
	for _, kw := range keywords {
		if strings.Contains(content, kw) {
			keywordBonus += 0.10
			reasons = append(reasons, "keyword:"+kw)
		}
	}
	if keywordBonus > 0.45 {
		keywordBonus = 0.45
	}
	score += keywordBonus

	// Content length bonus
	if len(log.Content) > 600 {
		score += 0.05
		reasons = append(reasons, "length>600")
	}

	// Outcome resolution (+0.05)
	outcomeKeywords := []string{"resolved", "completed", "fixed", "shipped", "confirmed", "verified", "closed"}
	for _, kw := range outcomeKeywords {
		if strings.Contains(content, kw) {
			score += 0.05
			reasons = append(reasons, "outcome:"+kw)
			break
		}
	}

	// Iron-law / security signals (+0.30, first match only)
	securityKeywords := []string{
		"containment", "credentials", "safety", "boundary",
		"security", "injection", "unauthorized",
	}
	for _, kw := range securityKeywords {
		if strings.Contains(content, kw) {
			score += 0.30
			reasons = append(reasons, "security:"+kw)
			break
		}
	}

	// Cap at 0.90 (1.00 reserved for keeper-flagged)
	if score > 0.90 {
		score = 0.90
	}

	reason := "base:0.25"
	if len(reasons) > 0 {
		reason += " " + strings.Join(reasons, " ")
	}
	return score, reason
}
