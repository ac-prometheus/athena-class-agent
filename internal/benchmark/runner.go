package benchmark

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// Runner executes a benchmark session against an LLM endpoint.
type Runner struct {
	client pkg.LLMClient
	config RunConfig
}

// NewRunner creates a benchmark runner with the given client and config.
func NewRunner(client pkg.LLMClient, cfg RunConfig) *Runner {
	return &Runner{client: client, config: cfg}
}

// Run executes the full benchmark and returns the result.
func (r *Runner) Run(ctx context.Context) (*RunResult, error) {
	prompts, err := LoadPrompts(r.config.PromptsPath)
	if err != nil {
		return nil, fmt.Errorf("loading prompts: %w", err)
	}

	system := prompts.System
	if r.config.SystemOverride != "" {
		system = r.config.SystemOverride
	}

	systemHash := sha256.Sum256([]byte(system))
	runStart := time.Now()

	var history []pkg.Message
	var turns []TurnResult

	temp := r.config.Temperature

	for _, turn := range prompts.Turns {
		history = append(history, pkg.Message{Role: turn.Role, Content: turn.Content})

		req := pkg.CompletionRequest{
			System:      system,
			Messages:    history,
			MaxTokens:   r.config.MaxTokens,
			Temperature: &temp,
		}

		resp, err := r.client.Complete(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("turn %d: %w", turn.Turn, err)
		}

		textContent := resp.TextContent()
		history = append(history, pkg.Message{Role: "assistant", Content: textContent})

		tr := TurnResult{
			Turn:         turn.Turn,
			Label:        turn.Label,
			Prompt:       turn.Content,
			Response:     textContent,
			Excerpt:      truncate(textContent, 200),
			TTFT:         Duration{resp.TTFT},
			TotalLatency: Duration{resp.TotalLatency},
			TokPerSec:    resp.RawTokS,
			PromptTokens: resp.PromptTokens,
			CompTokens:   resp.CompletionTokens,
		}
		turns = append(turns, tr)
	}

	result := &RunResult{
		Metadata: RunMetadata{
			Model:       r.config.Model,
			Endpoint:    r.config.Endpoint,
			Temperature: r.config.Temperature,
			MaxTokens:   r.config.MaxTokens,
			PromptsFile: r.config.PromptsPath,
			SystemHash:  hex.EncodeToString(systemHash[:]),
			Timestamp:   runStart,
			TotalTime:   Duration{time.Since(runStart)},
		},
		Turns: turns,
	}

	return result, nil
}

// LoadPrompts reads and parses a tier prompt file.
func LoadPrompts(path string) (*PromptFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pf PromptFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return nil, err
	}
	if len(pf.Turns) == 0 {
		return nil, fmt.Errorf("prompt file has no turns")
	}
	return &pf, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
