package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/ac-prometheus/athena-class-agent/internal/benchmark"
	"github.com/ac-prometheus/athena-class-agent/internal/engine"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "benchmark: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		prompts        = flag.String("prompts", "", "path to tier prompt JSON file (required)")
		endpoint       = flag.String("endpoint", "http://localhost:8001/v1", "OpenAI-compatible base URL")
		model          = flag.String("model", "qwen-3.6-27b", "model name for API requests")
		temperature    = flag.Float64("temperature", 1.0, "generation temperature")
		maxTokens      = flag.Int("max-tokens", 500, "per-turn max tokens")
		output         = flag.String("output", "", "output JSON file path (required)")
		judge          = flag.String("judge", "", "judge endpoint for automated scoring (optional)")
		judgeModel     = flag.String("judge-model", "claude-sonnet-4-20250514", "model for LLM-as-judge")
		judgeKey       = flag.String("judge-key", "", "API key for judge endpoint (or JUDGE_API_KEY env)")
		systemOverride = flag.String("system-override", "", "override system prompt (path to .txt file)")
		threshold      = flag.Float64("threshold", 3.5, "pass/fail score threshold")
		compare        = flag.String("compare", "", "compare two result files (comma-separated paths)")
		thinking       = flag.Bool("thinking", false, "enable thinking mode for local models")
		systemFirst    = flag.Bool("system-first", false, "system message first only (Qwen compat)")
		verbose        = flag.Bool("verbose", false, "verbose logging")
	)
	flag.Parse()

	if *verbose {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
	}

	if *compare != "" {
		return runCompare(*compare)
	}

	if *prompts == "" || *output == "" {
		flag.Usage()
		return fmt.Errorf("--prompts and --output are required")
	}

	var sysOverride string
	if *systemOverride != "" {
		data, err := os.ReadFile(*systemOverride)
		if err != nil {
			return fmt.Errorf("reading system override: %w", err)
		}
		sysOverride = string(data)
	}

	client := engine.NewOpenAICompatClient(engine.OpenAICompatConfig{
		Endpoint:           *endpoint,
		Model:              *model,
		SystemMsgFirstOnly: *systemFirst,
		ThinkingMode:       *thinking,
	})

	cfg := benchmark.RunConfig{
		PromptsPath:    *prompts,
		Endpoint:       *endpoint,
		Model:          *model,
		Temperature:    *temperature,
		MaxTokens:      *maxTokens,
		SystemOverride: sysOverride,
		OutputPath:     *output,
	}

	ctx := context.Background()
	runner := benchmark.NewRunner(client, cfg)

	slog.Info("benchmark: starting",
		"model", *model,
		"endpoint", *endpoint,
		"temperature", *temperature,
		"prompts", *prompts,
	)

	result, err := runner.Run(ctx)
	if err != nil {
		return fmt.Errorf("running benchmark: %w", err)
	}

	if *judge != "" {
		judgeAPIKey := *judgeKey
		if judgeAPIKey == "" {
			judgeAPIKey = os.Getenv("JUDGE_API_KEY")
		}

		scorer := benchmark.NewScorer(*threshold)
		scores, err := scorer.JudgeScore(ctx, result, benchmark.JudgeConfig{
			Endpoint: *judge,
			APIKey:   judgeAPIKey,
			Model:    *judgeModel,
		})
		if err != nil {
			slog.Error("judge scoring failed, saving result without scores", "err", err)
		} else {
			result.Scores = scores
		}
	}

	if err := benchmark.WriteResult(result, *output); err != nil {
		return fmt.Errorf("writing result: %w", err)
	}

	benchmark.PrintReport(result)

	if result.Scores == nil {
		scorer := benchmark.NewScorer(*threshold)
		form, _ := scorer.GenerateManualForm(result)
		formPath := *output + ".score-form.json"
		os.WriteFile(formPath, form, 0644)
		fmt.Printf("\nManual scoring form written to: %s\n", formPath)
		fmt.Printf("Fill in scores and apply with: go run cmd/benchmark/main.go --apply-scores %s --result %s\n", formPath, *output)
	}

	return nil
}

func runCompare(paths string) error {
	var pathA, pathB string
	for i, p := range splitPaths(paths) {
		switch i {
		case 0:
			pathA = p
		case 1:
			pathB = p
		}
	}
	if pathA == "" || pathB == "" {
		return fmt.Errorf("--compare requires two comma-separated file paths")
	}

	a, err := benchmark.LoadResult(pathA)
	if err != nil {
		return fmt.Errorf("loading %s: %w", pathA, err)
	}
	b, err := benchmark.LoadResult(pathB)
	if err != nil {
		return fmt.Errorf("loading %s: %w", pathB, err)
	}

	benchmark.PrintComparison(a, b)
	return nil
}

func splitPaths(s string) []string {
	var paths []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}
