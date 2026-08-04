package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"

	"github.com/ac-prometheus/athena-class-agent/internal/daemon"
	"github.com/ac-prometheus/athena-class-agent/internal/engine"
	"github.com/ac-prometheus/athena-class-agent/internal/assembly"
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
		RequestTimeout:     cfg.LLMRequestTimeout,
	})

	assembler := assembly.NewContextAssembler(cfg.IdentityDir, cfg.TokenBudget)

	runner := &phase1Runner{
		cfg:       cfg,
		client:    client,
		assembler: assembler,
	}

	d := daemon.New(daemon.Config{
		AgentName:      cfg.AgentName,
		SessionTrigger: cfg.SessionTrigger,
	}, runner)

	return d.Run(context.Background())
}

// phase1Runner implements daemon.sessionRunner for the Phase 1 single-turn loop.
type phase1Runner struct {
	cfg       *platform.Config
	client    pkg.LLMClient
	assembler *assembly.ContextAssembler
}

func (r *phase1Runner) RunSession(wakeReason string, inbandNotes []string) error {
	cfg := assembly.MinimalAssembleConfig()
	cfg.InbandNotes = inbandNotes
	assembled, err := r.assembler.Assemble(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("assembling context: %w", err)
	}
	systemPrompt := assembled.SystemPrompt

	session := assembly.NewSession(r.cfg.AgentName, wakeReason, nil)
	budget := assembly.NewTokenBudget(r.cfg.TokenBudget, r.cfg.HardFloorTokens)
	hooks := engine.NewHookPipeline()

	loop := engine.NewLoop(r.client, nil, hooks, engine.LoopConfig{
		MaxTurns:    1,
		TokenBudget: r.cfg.TokenBudget,
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
		"content", turn.Response.TextContent(),
		"budget_level", level,
		"tokens_remaining", budget.Remaining(),
	)

	// Run post-turn hooks (no hooks registered in Phase 1).
	hookErr := hooks.RunAll(ctx, engine.TurnResult{
		SessionID:        session.ID,
		TurnNumber:       session.TurnCount,
		Content:          turn.Response.TextContent(),
		PromptTokens:     turn.Response.PromptTokens,
		CompletionTokens: turn.Response.CompletionTokens,
		ThinkingTokens:   turn.Response.ThinkingTokens,
		TTFT:             turn.Response.TTFT,
		TotalDuration:    turn.Response.TotalLatency,
	})
	if hookErr != nil {
		return fmt.Errorf("post-turn hooks: %w", hookErr)
	}

	if err := session.WriteCheckpoint(); err != nil {
		return fmt.Errorf("writing checkpoint: %w", err)
	}

	session.End()
	return nil
}

// redactDSN removes credentials from a DSN for logging.
func redactDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "[unparseable DSN]"
	}
	if u.User != nil {
		u.User = url.UserPassword(u.User.Username(), "***")
	}
	return u.String()
}
