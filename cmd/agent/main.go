package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/ac-prometheus/athena-class-agent/internal/app"
	"github.com/ac-prometheus/athena-class-agent/internal/assembly"
	"github.com/ac-prometheus/athena-class-agent/internal/daemon"
	"github.com/ac-prometheus/athena-class-agent/internal/engine"
	"github.com/ac-prometheus/athena-class-agent/internal/platform"
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

	// Parse runtime profile from environment.
	profile, err := app.ProfileFromEnv()
	if err != nil {
		return err
	}

	slog.Info("agent: starting",
		"agent", cfg.AgentName,
		"profile", profile,
		"llm_provider", cfg.LLMProvider,
		"llm_model", cfg.LLMModel,
		"db_dsn", app.RedactDSN(cfg.DatabaseDSN),
	)

	// Build LLM client (Phase 1: OpenAI-compat only).
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
		RequestTimeout:     cfg.LLMRequestTimeout,
	})

	// Bootstrap dependencies and validate against profile.
	deps, err := app.Bootstrap(cfg, profile)
	if err != nil {
		return err
	}
	deps.LLM = client

	// Re-validate after setting LLM (Bootstrap can't construct it).
	if err := deps.Validate(profile); err != nil {
		return fmt.Errorf("post-bootstrap validation: %w", err)
	}

	// Build session runner.
	assembler := assembly.NewContextAssembler(cfg.IdentityDir, cfg.TokenBudget)

	type sessionRunner interface {
		RunSession(wakeReason string, inbandNotes []string) error
	}
	var runner sessionRunner

	if os.Getenv("PHASE1_MODE") == "true" {
		slog.Info("agent: running in phase1 compatibility mode")
		runner = app.NewPhase1Runner(deps, assembler)
	} else {
		runner = app.NewSessionRunner(deps, assembler, nil, nil)
	}

	// Create daemon and run.
	d := daemon.New(daemon.Config{
		AgentName:      cfg.AgentName,
		SessionTrigger: cfg.SessionTrigger,
	}, runner)

	if deps.DB != nil {
		d.WithDB(deps.DB, app.DriverNameFromDSN(cfg.DatabaseDSN))
	}

	return d.Run(context.Background())
}
