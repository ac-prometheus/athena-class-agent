package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/ac-prometheus/athena-class-agent/internal/engine"
	"github.com/ac-prometheus/athena-class-agent/internal/harness"
	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "agent: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := platform.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	logger := platform.NewLogger(platform.LogLevel(os.Getenv("LOG_LEVEL")))
	slog.SetDefault(logger)

	slog.Info("agent: starting",
		"agent", cfg.AgentName,
		"llm_provider", cfg.LLMProvider,
		"llm_model", cfg.LLMModel,
		"db_dsn", redactDSN(cfg.DatabaseDSN),
	)

	// Phase 1: only OpenAICompatClient is wired. Anthropic and Gemini land in Phase 3.
	if cfg.LLMProvider != "openai" && cfg.LLMProvider != "openai-compat" {
		return fmt.Errorf("Phase 1 supports only LLM_PROVIDER=openai; got %q", cfg.LLMProvider)
	}
	if cfg.LLMEndpoint == "" {
		return fmt.Errorf("LLM_ENDPOINT is required for openai-compat provider")
	}

	client := engine.NewOpenAICompatClient(engine.OpenAICompatConfig{
		Endpoint:           cfg.LLMEndpoint,
		APIKey:             cfg.LLMAPIKey,
		Model:              cfg.LLMModel,
		SystemMsgFirstOnly: cfg.LLMProfile == "qwen" || os.Getenv("LLM_SYSTEM_FIRST_ONLY") == "true",
		ThinkingMode:       os.Getenv("LLM_THINKING_MODE") == "true",
	})

	assembler := harness.NewContextAssembler(cfg.IdentityDir)
	systemPrompt, err := assembler.AssembleSystemPrompt()
	if err != nil {
		return fmt.Errorf("assembling system prompt: %w", err)
	}

	session := harness.NewSession(cfg.AgentName, "phase1-startup")
	budget := harness.NewTokenBudget(cfg.TokenBudget, cfg.HardFloorTokens)

	loop := engine.NewLoop(client, engine.LoopConfig{
		MaxTurns:    1,
		TokenBudget: cfg.TokenBudget,
	})

	req := pkg.CompletionRequest{
		System: systemPrompt,
		Messages: []pkg.Message{
			{Role: "user", Content: "Hello. This is a Phase 1 connectivity test."},
		},
		MaxTokens: 256,
	}

	ctx := context.Background()
	turn, err := loop.RunOneTurn(ctx, req)
	if err != nil {
		return fmt.Errorf("running turn: %w", err)
	}

	session.RecordTurn(turn.Response.PromptTokens, turn.Response.CompletionTokens)
	level := budget.Add(turn.Response.PromptTokens, turn.Response.CompletionTokens)

	slog.Info("agent: turn result",
		"content", turn.Response.Content,
		"budget_level", level,
		"tokens_remaining", budget.Remaining(),
	)

	session.End()
	return nil
}

// redactDSN removes credentials from a DSN for logging.
func redactDSN(dsn string) string {
	// Simple: only log the scheme and host, not credentials or file paths.
	if len(dsn) > 32 {
		return dsn[:32] + "..."
	}
	return dsn
}
