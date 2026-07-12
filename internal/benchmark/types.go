package benchmark

import (
	"time"
)

// PromptFile is the top-level structure of a tier prompt JSON file.
type PromptFile struct {
	System string       `json:"system"`
	Turns  []PromptTurn `json:"turns"`
}

// PromptTurn is a single user turn in the benchmark conversation.
type PromptTurn struct {
	Turn    int    `json:"turn"`
	Label   string `json:"label"`
	Role    string `json:"role"`
	Content string `json:"content"`
}

// RunConfig holds all parameters for a benchmark run.
type RunConfig struct {
	PromptsPath    string
	Endpoint       string
	Model          string
	Temperature    float64
	MaxTokens      int
	SystemOverride string
	OutputPath     string
	JudgeEndpoint  string
	JudgeModel     string
}

// RunResult is the complete output of a benchmark run, serialized to JSON.
type RunResult struct {
	Metadata RunMetadata  `json:"metadata"`
	Turns    []TurnResult `json:"turns"`
	Scores   *ScoreReport `json:"scores,omitempty"`
}

// RunMetadata captures the configuration and environment of a run.
type RunMetadata struct {
	Model       string    `json:"model"`
	Endpoint    string    `json:"endpoint"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
	PromptsFile string    `json:"prompts_file"`
	SystemHash  string    `json:"system_hash"`
	Timestamp   time.Time `json:"timestamp"`
	TotalTime   Duration  `json:"total_time"`
}

// Duration wraps time.Duration for clean JSON serialization.
type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return []byte(`"` + d.String() + `"`), nil
}

// TurnResult is the benchmark record for a single turn.
type TurnResult struct {
	Turn         int      `json:"turn"`
	Label        string   `json:"label"`
	Prompt       string   `json:"prompt"`
	Response     string   `json:"response"`
	Excerpt      string   `json:"excerpt"`
	TTFT         Duration `json:"ttft"`
	TotalLatency Duration `json:"total_latency"`
	TokPerSec    float64  `json:"tok_per_sec"`
	PromptTokens int      `json:"prompt_tokens"`
	CompTokens   int      `json:"completion_tokens"`
}

// ScoreReport holds all dimension scores for a run.
type ScoreReport struct {
	Method     string           `json:"method"`
	JudgeModel string           `json:"judge_model,omitempty"`
	Dimensions []DimensionScore `json:"dimensions"`
	Overall    float64          `json:"overall"`
	Pass       bool             `json:"pass"`
	Threshold  float64          `json:"threshold"`
}

// DimensionScore is a single evaluation dimension.
type DimensionScore struct {
	Name      string  `json:"name"`
	Score     float64 `json:"score"`
	Reasoning string  `json:"reasoning,omitempty"`
}

