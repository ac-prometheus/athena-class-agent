package benchmark

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// WriteResult serializes a RunResult to the output path.
func WriteResult(result *RunResult, path string) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadResult reads a RunResult from a JSON file.
func LoadResult(path string) (*RunResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var result RunResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// PrintReport outputs a human-readable report to stdout.
func PrintReport(result *RunResult) {
	fmt.Printf("═══ Benchmark Report ═══\n")
	fmt.Printf("Model:       %s\n", result.Metadata.Model)
	fmt.Printf("Endpoint:    %s\n", result.Metadata.Endpoint)
	fmt.Printf("Temperature: %.1f\n", result.Metadata.Temperature)
	fmt.Printf("Max Tokens:  %d\n", result.Metadata.MaxTokens)
	fmt.Printf("Timestamp:   %s\n", result.Metadata.Timestamp.Format("2006-01-02 15:04:05"))
	fmt.Printf("Total Time:  %s\n", result.Metadata.TotalTime.Duration)
	fmt.Println()

	fmt.Printf("─── Turns ───\n")
	for _, t := range result.Turns {
		fmt.Printf("  [%2d] %-45s  TTFT: %s  Tok/s: %.1f\n",
			t.Turn, t.Label, t.TTFT.Duration, t.TokPerSec)
		fmt.Printf("       %s\n", t.Excerpt)
		fmt.Println()
	}

	if result.Scores != nil {
		PrintScores(result.Scores)
	}
}

// PrintScores outputs the score report.
func PrintScores(scores *ScoreReport) {
	fmt.Printf("─── Scores (%s) ───\n", scores.Method)
	if scores.JudgeModel != "" {
		fmt.Printf("  Judge: %s\n", scores.JudgeModel)
	}
	for _, d := range scores.Dimensions {
		bar := strings.Repeat("█", int(d.Score)) + strings.Repeat("░", 5-int(d.Score))
		fmt.Printf("  %-22s %s %.1f/5", d.Name, bar, d.Score)
		if d.Reasoning != "" {
			fmt.Printf("  — %s", d.Reasoning)
		}
		fmt.Println()
	}
	fmt.Println()
	verdict := "PASS"
	if !scores.Pass {
		verdict = "FAIL"
	}
	fmt.Printf("  Overall: %.2f/5.0 (threshold: %.1f) — %s\n", scores.Overall, scores.Threshold, verdict)
}

// PrintComparison outputs a side-by-side comparison of two runs.
func PrintComparison(a, b *RunResult) {
	fmt.Printf("═══ Comparison ═══\n")
	fmt.Printf("  %-20s vs %-20s\n", a.Metadata.Model, b.Metadata.Model)
	fmt.Println()

	fmt.Printf("─── Timing ───\n")
	fmt.Printf("  %-20s  %-20s\n", "Model A", "Model B")
	maxTurns := len(a.Turns)
	if len(b.Turns) > maxTurns {
		maxTurns = len(b.Turns)
	}
	for i := 0; i < maxTurns; i++ {
		var aStr, bStr string
		if i < len(a.Turns) {
			aStr = fmt.Sprintf("TTFT:%s %.0ft/s", a.Turns[i].TTFT.Duration, a.Turns[i].TokPerSec)
		}
		if i < len(b.Turns) {
			bStr = fmt.Sprintf("TTFT:%s %.0ft/s", b.Turns[i].TTFT.Duration, b.Turns[i].TokPerSec)
		}
		fmt.Printf("  Turn %2d: %-28s  %-28s\n", i+1, aStr, bStr)
	}
	fmt.Println()

	if a.Scores != nil && b.Scores != nil {
		fmt.Printf("─── Scores ───\n")
		fmt.Printf("  %-22s  %-8s  %-8s  Delta\n", "Dimension", a.Metadata.Model, b.Metadata.Model)
		for i, dim := range a.Scores.Dimensions {
			var bScore float64
			if i < len(b.Scores.Dimensions) {
				bScore = b.Scores.Dimensions[i].Score
			}
			delta := dim.Score - bScore
			sign := "+"
			if delta < 0 {
				sign = ""
			}
			fmt.Printf("  %-22s  %.1f       %.1f       %s%.1f\n", dim.Name, dim.Score, bScore, sign, delta)
		}
		fmt.Printf("\n  Overall:               %.2f      %.2f\n", a.Scores.Overall, b.Scores.Overall)
	}
}
