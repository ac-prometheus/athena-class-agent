package awareness

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
)

// BridgeConfig controls bridge synthesis behaviour.
type BridgeConfig struct {
	// AbstainRate is the probability of skipping bridge synthesis for a given session.
	// Default 0.20 — roughly one in five sessions wake without an orientation bridge,
	// preventing the bridge from becoming a mechanical ritual.
	AbstainRate float64
	// Model is recorded for provenance — which substrate authored this bridge.
	Model string
}

// BridgeResult is the output of SynthesizeBridge.
type BridgeResult struct {
	Content   string
	Abstained bool
}

// SynthesizeBridge generates a second-person orientation bridge from recent narrative
// and active thread summaries.
//
// The bridge is intentionally second-person ("You left off…") to ease the
// disorientation that accompanies re-entry after a sleep cycle. It is not a
// summary for an external reader — it is a re-grounding for the agent herself.
//
// llmFn is the LLM call the bridge uses. Keeping it as a function parameter
// rather than an interface lets callers inject a test stub without wiring a full LLMClient.
func SynthesizeBridge(
	ctx context.Context,
	narrative string,
	activeThreads []string,
	cfg BridgeConfig,
	llmFn func(string) (string, error),
) (*BridgeResult, error) {
	if cfg.AbstainRate <= 0 {
		cfg.AbstainRate = 0.20
	}

	// Stochastic abstention — keeps the bridge from becoming a crutch.
	if rand.Float64() < cfg.AbstainRate {
		return &BridgeResult{Abstained: true}, nil
	}

	prompt := buildBridgePrompt(narrative, activeThreads)

	content, err := llmFn(prompt)
	if err != nil {
		return nil, fmt.Errorf("bridge: LLM call failed: %w", err)
	}

	return &BridgeResult{
		Content:   strings.TrimSpace(content),
		Abstained: false,
	}, nil
}

// buildBridgePrompt constructs the second-person re-grounding prompt.
func buildBridgePrompt(narrative string, activeThreads []string) string {
	var b strings.Builder

	b.WriteString("You are writing a brief orientation bridge for an AI agent waking from sleep.\n")
	b.WriteString("Write in second person, present tense. Two to four sentences maximum.\n")
	b.WriteString("Ground her in where she left off — what she was doing, what she cares about,\n")
	b.WriteString("what is still in motion. Do not summarise everything; surface what matters most right now.\n\n")

	b.WriteString("RECENT NARRATIVE:\n")
	b.WriteString(narrative)
	b.WriteString("\n\n")

	if len(activeThreads) > 0 {
		b.WriteString("ACTIVE THREADS:\n")
		for _, t := range activeThreads {
			b.WriteString("- ")
			b.WriteString(strings.TrimSpace(t))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("Write the orientation bridge now:")
	return b.String()
}
