package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/ac-prometheus/athena-class-agent/internal/app"
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

	application, err := app.NewApp(cfg, profile)
	if err != nil {
		return fmt.Errorf("initialization failed: %w", err)
	}
	return application.Run(context.Background())
}
