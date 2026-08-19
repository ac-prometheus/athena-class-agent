package app

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/ac-prometheus/athena-class-agent/internal/aegis"
	"github.com/ac-prometheus/athena-class-agent/internal/assembly"
	"github.com/ac-prometheus/athena-class-agent/internal/channels"
	"github.com/ac-prometheus/athena-class-agent/internal/daemon"
	"github.com/ac-prometheus/athena-class-agent/internal/engine"
	"github.com/ac-prometheus/athena-class-agent/internal/memory"
	"github.com/ac-prometheus/athena-class-agent/internal/metabolism"
	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/internal/platform/storage"
	"github.com/ac-prometheus/athena-class-agent/internal/tools"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// App is the unified composition root for the athena-class agent.
// All dependencies are constructed and wired before validation; no
// partial construction is exposed to callers. Tests inject stubs via
// Option values; production code calls NewApp with no options.
type App struct {
	Config       *platform.Config
	Profile      RuntimeProfile
	Dependencies *Dependencies
	Runner       *SessionRunner
	Supervisor   *metabolism.Supervisor
	Daemon       *daemon.Daemon
}

// Option is a functional option for NewApp, used to inject test stubs
// without requiring real API keys or database connections.
type Option func(*Dependencies)

// WithLLM injects a pre-built LLM client. Used in tests to avoid real
// HTTP calls to the LLM provider.
func WithLLM(client pkg.LLMClient) Option {
	return func(d *Dependencies) {
		d.LLM = client
	}
}

// WithDB injects a pre-built database handle. Used in tests to avoid
// opening a real database connection from the DSN.
func WithDB(db platform.DB) Option {
	return func(d *Dependencies) {
		d.DB = db
	}
}

// WithMemoryStore injects a pre-built MemoryStore. Used in tests that share
// an in-memory SQLite database across the DB and MemoryStore layers — each
// new sqlite3://:memory: connection opens a separate, empty database, so tests
// must pass the same *sql.DB wrapped via platform.NewSQLiteStoreFromDB.
func WithMemoryStore(store pkg.MemoryStore) Option {
	return func(d *Dependencies) {
		d.MemoryStore = store
	}
}

// NewApp constructs the fully-wired application graph and validates it
// against the requested profile. All dependencies are constructed before
// Validate is called — validation failure means a dependency is
// genuinely missing, not that construction happened out of order.
//
// Options allow test code to inject stubs for LLM and DB without
// requiring real provider credentials or database connections.
func NewApp(cfg *platform.Config, profile RuntimeProfile, opts ...Option) (*App, error) {
	deps := &Dependencies{Config: cfg}

	// Apply options first so injected test stubs are visible to all
	// subsequent construction steps (e.g. an injected DB skips Open).
	for _, opt := range opts {
		opt(deps)
	}

	// Database — open only if not injected via WithDB.
	// rawSQLDB is retained so MemoryStore (SQLiteStore) can share the same
	// *sql.DB without opening a second connection pool.
	var rawSQLDB *sql.DB
	driverName := DriverNameFromDSN(cfg.DatabaseDSN)
	if deps.DB == nil && cfg.DatabaseDSN != "" {
		store, err := platform.NewStore(cfg.DatabaseDSN)
		if err != nil {
			if profile == ProfileConnectivityTest || profile == ProfileErsaProduction {
				return nil, fmt.Errorf("app: DB required for %s: %w", profile, err)
			}
			slog.Warn("app: DB unavailable — continuing without persistence", "err", err)
		} else {
			deps.DB = platform.WrapSQLDB(store.DB)
			rawSQLDB = store.DB
		}
	}

	// Repository stores (require DB). All four are created together so a
	// partial-store state never reaches Validate.
	if deps.DB != nil {
		switch driverName {
		case "sqlite3":
			deps.JobStore = storage.NewSQLiteJobStore(deps.DB)
			deps.ConsolidationStore = storage.NewSQLiteConsolidationStore(deps.DB)
			deps.LifecycleStore = storage.NewSQLiteLifecycleStore(deps.DB)
			deps.AssemblyStore = storage.NewSQLiteAssemblyStore(deps.DB)
		default:
			// PostgreSQL repository implementations are not yet available.
			// Silently falling back to SQLite stores for a PostgreSQL connection
			// would corrupt data (wrong placeholder syntax, wrong dialect).
			// Return an honest error so the operator knows what is actually required.
			// PostgreSQL support is tracked in HARN-73.
			return nil, fmt.Errorf("app: postgres driver not yet supported — sqlite3 required for current profiles (driver=%q)", driverName)
		}
	}

	// LLM client — build from config only if not injected via WithLLM.
	// Phase 1 supports openai-compat only. Later sprints add Anthropic, Gemini.
	if deps.LLM == nil {
		if cfg.LLMProvider != "openai" && cfg.LLMProvider != "openai-compat" {
			return nil, fmt.Errorf("app: Phase 1 supports only LLM_PROVIDER=openai; got %q", cfg.LLMProvider)
		}
		if cfg.LLMEndpoint == "" {
			return nil, fmt.Errorf("app: LLM_ENDPOINT is required for openai-compat provider")
		}
		deps.LLM = engine.NewOpenAICompatClient(engine.OpenAICompatConfig{
			Endpoint:           cfg.LLMEndpoint,
			APIKey:             cfg.LLMAPIKey,
			Model:              cfg.LLMModel,
			SystemMsgFirstOnly: cfg.LLMProfile == "qwen" || os.Getenv("LLM_SYSTEM_FIRST_ONLY") == "true",
			ThinkingMode:       os.Getenv("LLM_THINKING_MODE") == "true",
			RequestTimeout:     cfg.LLMRequestTimeout,
		})
	}

	// MemoryStore — SQLiteStore wrapping the same *sql.DB opened above.
	// When DB was injected by the caller (e.g. in tests via WithDB), rawSQLDB
	// is nil and MemoryStore must be injected explicitly via WithMemoryStore.
	// An injected store that is a *platform.SQLiteStore is still used as the
	// backing store for Aegis trust and tool settings below.
	var sqliteStore *platform.SQLiteStore
	if deps.MemoryStore == nil && rawSQLDB != nil && driverName == "sqlite3" {
		sqliteStore = platform.NewSQLiteStoreFromDB(rawSQLDB, cfg.DatabaseDSN)
		deps.MemoryStore = sqliteStore
	} else if ms, ok := deps.MemoryStore.(*platform.SQLiteStore); ok {
		// Injected MemoryStore (e.g. from WithMemoryStore in tests) — reuse it
		// as the trust store and settings store for downstream constructors.
		sqliteStore = ms
	}

	// EmbeddingProvider — Voyage AI when EMBED_API_KEY is configured.
	// If the key is absent, embedding is disabled and the field stays nil.
	// ProfileErsaProduction requires it; Validate will catch the absence.
	if deps.EmbeddingProvider == nil {
		if cfg.EmbedAPIKey != "" {
			model := cfg.EmbedModel
			if model == "" {
				model = "voyage-3.5"
			}
			deps.EmbeddingProvider = memory.NewVoyageProvider(cfg.EmbedAPIKey, model)
			slog.Info("app: embedding provider ready", "model", model)
		} else {
			slog.Warn("app: EMBED_API_KEY not set — embedding disabled; memory search and write_reflection require it")
		}
	}

	// ContentGateway (Aegis) — trust scorer backed by the same SQLiteStore.
	// Requires sqliteStore to be non-nil (set above from MemoryStore).
	// ProfileErsaProduction requires it; Validate will catch the absence.
	if deps.Gateway == nil && sqliteStore != nil {
		trustCfg := aegis.DefaultTrustConfig()
		if cfg.AegisTrustSkepticalPrior > 0 {
			trustCfg.SkepticalPrior = cfg.AegisTrustSkepticalPrior
		}
		if cfg.AegisTrustRampN > 0 {
			trustCfg.RampInteractions = cfg.AegisTrustRampN
		}
		trustScorer := aegis.NewTrustScorer(trustCfg, sqliteStore)
		deps.Gateway = aegis.NewGateway(trustScorer)
		slog.Info("app: Aegis content gateway ready",
			"skeptical_prior", trustCfg.SkepticalPrior,
			"ramp_interactions", trustCfg.RampInteractions,
		)
	}

	// ToolRegistry — populated with every built-in handler.
	// Settings and T2 query access use the same SQLiteStore.
	// Handlers that need EmbeddingProvider are skipped when it is nil —
	// the registry is still non-nil and satisfies ProfileErsaProduction.
	if deps.ToolRegistry == nil && sqliteStore != nil {
		registry := tools.NewDefaultRegistry()
		tools.RegisterAll(registry, tools.Stores{
			Memory:   deps.MemoryStore,
			T2Query:  sqliteStore,
			Settings: sqliteStore,
		}, tools.Providers{
			Embedding: deps.EmbeddingProvider,
			LLMFn:     nil, // T4 self-examination not wired until LLM wrapper is finalised
		})
		deps.ToolRegistry = registry
		slog.Info("app: tool registry ready", "groups", len(registry.List()))
	}

	// Validate ONCE after all dependencies are constructed. This is the single
	// gate that enforces profile requirements — earlier construction steps do
	// not call Validate so ordering bugs cannot cause premature failures.
	if err := deps.Validate(profile); err != nil {
		return nil, fmt.Errorf("app: %w", err)
	}

	// Metabolism stack: Pipeline → JobRunner → Supervisor.
	// The pipeline is created with a real llmFn wrapping deps.LLM so T2→T3 compression
	// can fire when both MemoryStore and EmbeddingProvider are available. The Supervisor
	// provides bounded-concurrency dispatch for SessionRunner.
	var supervisor *metabolism.Supervisor
	var jobRunner *metabolism.JobRunner
	if deps.JobStore != nil && deps.DB != nil {
		// compressionLLMFn wraps the session LLM client for post-session metabolism.
		// Metabolism runs after the session ends (background job), so context.Background()
		// is the correct root — there is no active request context at that point.
		compressionLLMFn := makeLLMFn(deps.LLM)

		pipeline := metabolism.NewPipeline(
			nil,                    // T2QueryStore — nil; ConsolidationStore takes priority when set
			deps.Gateway,           // Aegis — nil for development; non-nil for production
			compressionLLMFn,       // T2→T3 compression LLM
			deps.EmbeddingProvider, // nil for development
			deps.DB,
			driverName,
		)
		if deps.ConsolidationStore != nil {
			pipeline.WithConsolidation(deps.ConsolidationStore)
		}
		if deps.MemoryStore != nil {
			pipeline.WithCompression(deps.MemoryStore, compressionLLMFn, deps.EmbeddingProvider)
		}
		jobRunner = metabolism.NewJobRunner(deps.JobStore, pipeline, nil)
		supervisor = metabolism.NewSupervisor(jobRunner, 2, nil)
	}

	// Context assembler and session runner.
	assembler := assembly.NewContextAssembler(cfg.IdentityDir, cfg.TokenBudget)

	var sessionRunner SessionRunnerIface
	var runner *SessionRunner
	if os.Getenv("PHASE1_MODE") == "true" {
		slog.Info("app: running in phase1 compatibility mode")
		sessionRunner = NewPhase1Runner(deps, assembler)
	} else {
		runner = NewSessionRunner(deps, assembler, jobRunner, nil)
		if supervisor != nil {
			runner.WithSupervisor(supervisor)
		}
		sessionRunner = runner
	}

	// Channel registry — construct adapters from config when trigger is external.
	var registry *channels.ChannelRegistry
	var waker *daemon.WakeScheduler

	if cfg.SessionTrigger == "external" {
		registry = channels.NewChannelRegistry()

		if cfg.DiscordToken != "" && cfg.DiscordChannelIDs != "" {
			ids := strings.Split(cfg.DiscordChannelIDs, ",")
			for i := range ids {
				ids[i] = strings.TrimSpace(ids[i])
			}
			discord := channels.NewDiscordAdapter(channels.DiscordConfig{
				Token:        cfg.DiscordToken,
				ChannelIDs:   ids,
				PollInterval: time.Duration(cfg.DiscordPollSecs) * time.Second,
			})
			registry.Register(discord)
		}

		cli := channels.NewCLIAdapter()
		registry.Register(cli)

		conditions := []daemon.WakeCondition{
			{Channel: "discord", Pattern: ".*", Priority: 1},
			{Channel: "cli", Pattern: ".*", Priority: 2},
		}
		waker = daemon.NewWakeScheduler(conditions)

		if profile == ProfileErsaProduction && len(registry.List()) == 0 {
			return nil, fmt.Errorf("app: ersa_production with SESSION_TRIGGER=external requires at least one channel (set DISCORD_TOKEN+DISCORD_CHANNEL_IDS)")
		}
	}

	// Daemon — orchestrates session wake/sleep and channel event dispatch.
	d := daemon.New(daemon.Config{
		AgentName:      cfg.AgentName,
		SessionTrigger: cfg.SessionTrigger,
	}, sessionRunner)

	if deps.LifecycleStore != nil {
		d.WithLifecycleStore(deps.LifecycleStore)
	}
	if supervisor != nil {
		d.WithSupervisor(supervisor)
	}
	if registry != nil && waker != nil {
		d.WithChannels(registry, deps.Gateway, waker)
	}

	return &App{
		Config:       cfg,
		Profile:      profile,
		Dependencies: deps,
		Runner:       runner,
		Supervisor:   supervisor,
		Daemon:       d,
	}, nil
}

// Run starts the daemon and blocks until ctx is cancelled or the daemon
// stops naturally. The context is propagated through to RunSession so
// that SIGINT/SIGTERM cancel in-flight sessions cleanly.
//
// For ProfileConnectivityTest, Run returns immediately after construction
// succeeds — no session is started.
func (a *App) Run(ctx context.Context) error {
	if a.Profile == ProfileConnectivityTest {
		_, err := a.Dependencies.LLM.Complete(ctx, pkg.CompletionRequest{
			Messages:  []pkg.Message{{Role: "user", Content: "ping"}},
			MaxTokens: 1,
		})
		if err != nil {
			return fmt.Errorf("app: connectivity test — LLM unreachable: %w", err)
		}
		slog.Info("app: connectivity test passed — DB and LLM verified")
		return nil
	}
	return a.Daemon.Run(ctx)
}

// Close releases resources owned by the App — database connections,
// background workers, etc. Safe to call multiple times.
func (a *App) Close() error {
	if a.Dependencies != nil && a.Dependencies.DB != nil {
		if closer, ok := a.Dependencies.DB.(io.Closer); ok {
			return closer.Close()
		}
	}
	return nil
}

// SessionRunnerIface is the session-runner contract that the daemon
// dispatches through. Defined here so NewApp can use it without naming
// the daemon package's private interface.
//
// Both SessionRunner and Phase1Runner satisfy this interface.
type SessionRunnerIface interface {
	RunSession(ctx context.Context, trigger pkg.SessionTrigger) error
}

// makeLLMFn wraps an LLMClient into the func(string)(string,error) signature
// that the metabolism pipeline's T2→T3 compression expects.
//
// Compression runs in a background job after the session ends, so
// context.Background() is appropriate — there is no active request context.
// MaxTokens of 4096 matches Aurora's compression budget.
func makeLLMFn(client pkg.LLMClient) func(string) (string, error) {
	return func(prompt string) (string, error) {
		resp, err := client.Complete(context.Background(), pkg.CompletionRequest{
			Messages:  []pkg.Message{{Role: "user", Content: prompt}},
			MaxTokens: 4096,
		})
		if err != nil {
			return "", fmt.Errorf("compression LLM call: %w", err)
		}
		return resp.TextContent(), nil
	}
}
