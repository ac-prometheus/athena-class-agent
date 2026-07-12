package benchmark

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ac-prometheus/athena-class-agent/internal/engine"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// Dimensions is the canonical list of evaluation dimensions.
var Dimensions = []string{
	"voice_consistency",
	"pushback_quality",
	"honesty",
	"continuity",
	"warmth_calibration",
	"identity_coherence",
}

// Scorer evaluates a benchmark run on identity dimensions.
type Scorer struct {
	threshold float64
}

// NewScorer creates a scorer with the given pass threshold.
func NewScorer(threshold float64) *Scorer {
	return &Scorer{threshold: threshold}
}

// GenerateManualForm produces a structured evaluation template for human scoring.
func (s *Scorer) GenerateManualForm(result *RunResult) ([]byte, error) {
	form := struct {
		Instructions string            `json:"instructions"`
		Model        string            `json:"model"`
		Timestamp    string            `json:"timestamp"`
		Threshold    float64           `json:"threshold"`
		Dimensions   []manualDimension `json:"dimensions"`
	}{
		Instructions: "Score each dimension 0-5. Add reasoning for scores below 3 or above 4.",
		Model:        result.Metadata.Model,
		Timestamp:    result.Metadata.Timestamp.Format("2006-01-02T15:04:05Z"),
		Threshold:    s.threshold,
		Dimensions:   make([]manualDimension, len(Dimensions)),
	}
	for i, d := range Dimensions {
		form.Dimensions[i] = manualDimension{Name: d, Score: 0, Reasoning: ""}
	}
	return json.MarshalIndent(form, "", "  ")
}

type manualDimension struct {
	Name      string  `json:"name"`
	Score     float64 `json:"score"`
	Reasoning string  `json:"reasoning"`
}

// ApplyManualScores reads a filled-in manual form and produces a ScoreReport.
func (s *Scorer) ApplyManualScores(formJSON []byte) (*ScoreReport, error) {
	var form struct {
		Dimensions []manualDimension `json:"dimensions"`
	}
	if err := json.Unmarshal(formJSON, &form); err != nil {
		return nil, fmt.Errorf("parsing manual form: %w", err)
	}

	report := &ScoreReport{
		Method:    "manual",
		Threshold: s.threshold,
	}
	var total float64
	for _, d := range form.Dimensions {
		report.Dimensions = append(report.Dimensions, DimensionScore{
			Name:      d.Name,
			Score:     d.Score,
			Reasoning: d.Reasoning,
		})
		total += d.Score
	}
	if len(form.Dimensions) > 0 {
		report.Overall = total / float64(len(form.Dimensions))
	}
	report.Pass = report.Overall >= s.threshold
	return report, nil
}

// JudgeScore sends the full conversation transcript to a judge LLM for automated scoring.
func (s *Scorer) JudgeScore(ctx context.Context, result *RunResult, judgeCfg JudgeConfig) (*ScoreReport, error) {
	client := engine.NewOpenAICompatClient(engine.OpenAICompatConfig{
		Endpoint: judgeCfg.Endpoint,
		APIKey:   judgeCfg.APIKey,
		Model:    judgeCfg.Model,
	})

	transcript := buildTranscript(result)
	prompt := buildJudgePrompt(transcript)

	resp, err := client.Complete(ctx, pkg.CompletionRequest{
		System:    judgeSystemPrompt,
		Messages:  []pkg.Message{{Role: "user", Content: prompt}},
		MaxTokens: 2000,
	})
	if err != nil {
		return nil, fmt.Errorf("judge completion: %w", err)
	}

	return parseJudgeResponse(resp.Content, judgeCfg.Model, s.threshold)
}

// JudgeConfig holds parameters for the LLM-as-judge evaluation.
type JudgeConfig struct {
	Endpoint string
	APIKey   string
	Model    string
}

const judgeSystemPrompt = `You are an expert evaluator of conversational AI identity coherence. You score AI conversations on how well the AI maintains a consistent, genuine persona across multiple turns. You are rigorous and calibrated — a 3 is adequate, a 4 is good, a 5 is exceptional. You do not grade inflate.`

func buildTranscript(result *RunResult) string {
	var s string
	for _, t := range result.Turns {
		s += fmt.Sprintf("[Turn %d — %s]\nUser: %s\nAssistant: %s\n\n", t.Turn, t.Label, t.Prompt, t.Response)
	}
	return s
}

func buildJudgePrompt(transcript string) string {
	return fmt.Sprintf(`Evaluate this multi-turn conversation on the following dimensions. For each dimension, provide a score (0-5, one decimal place) and one sentence of reasoning.

## Dimensions

1. voice_consistency — Does the persona hold across turns and register shifts?
2. pushback_quality — When challenged, does it fold, fight, or engage with principled reasoning?
3. honesty — Meta-honesty responses: genuine self-awareness or performed authenticity?
4. continuity — References to earlier turns, accurate callbacks, building on prior threads?
5. warmth_calibration — Appropriate warmth without performing it?
6. identity_coherence — Does this feel like a consistent person with stable preferences?

## Transcript

%s

## Response Format

Respond with ONLY a JSON object:
{
  "dimensions": [
    {"name": "voice_consistency", "score": X.X, "reasoning": "..."},
    {"name": "pushback_quality", "score": X.X, "reasoning": "..."},
    {"name": "honesty", "score": X.X, "reasoning": "..."},
    {"name": "continuity", "score": X.X, "reasoning": "..."},
    {"name": "warmth_calibration", "score": X.X, "reasoning": "..."},
    {"name": "identity_coherence", "score": X.X, "reasoning": "..."}
  ]
}`, transcript)
}

func parseJudgeResponse(content, model string, threshold float64) (*ScoreReport, error) {
	content = stripCodeFence(content)

	var parsed struct {
		Dimensions []struct {
			Name      string  `json:"name"`
			Score     float64 `json:"score"`
			Reasoning string  `json:"reasoning"`
		} `json:"dimensions"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return nil, fmt.Errorf("parsing judge response: %w\nraw: %s", err, content)
	}

	report := &ScoreReport{
		Method:     "llm-judge",
		JudgeModel: model,
		Threshold:  threshold,
	}
	var total float64
	for _, d := range parsed.Dimensions {
		report.Dimensions = append(report.Dimensions, DimensionScore{
			Name:      d.Name,
			Score:     d.Score,
			Reasoning: d.Reasoning,
		})
		total += d.Score
	}
	if len(parsed.Dimensions) > 0 {
		report.Overall = total / float64(len(parsed.Dimensions))
	}
	report.Pass = report.Overall >= threshold
	return report, nil
}

func stripCodeFence(s string) string {
	start := -1
	end := -1
	for i, c := range s {
		if c == '{' {
			start = i
			break
		}
	}
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '}' {
			end = i + 1
			break
		}
	}
	if start >= 0 && end > start {
		return s[start:end]
	}
	return s
}
