package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/ac-prometheus/athena-class-agent/internal/assembly"
	"github.com/ac-prometheus/athena-class-agent/internal/daemon"
	"github.com/ac-prometheus/athena-class-agent/internal/engine"
	"github.com/ac-prometheus/athena-class-agent/internal/metabolism"
	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/internal/platform/storage"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// App is the unified composition root for the athena-class agent.
// All dependencies are constructed and wired before validation; no
// partial construction is exposed to callers. Tests inject stubs via
// Option values; production code calls NewApp with no options.
type App struct {
	Config       *platform.Config
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

	// MemoryStore, EmbeddingProvider, Gateway (Aegis), and ToolRegistry are
	// wired here when their prerequisites (API keys, provider selection) are
	// available. For now they are left nil — optional for ProfileDevelopment,
	// required for ProfileErsaProduction (Validate will catch the gap).
	// TODO: construct these in a follow-up sprint when the embedding provider
	// and tool registry constructors are stabilised.

	// Validate ONCE after all dependencies are constructed. This is the single
	// gate that enforces profile requirements — earlier construction steps do
	// not call Validate so ordering bugs cannot cause premature failures.
	if err := deps.Validate(profile); err != nil {
		return nil, fmt.Errorf("app: %w", err)
	}

	// Metabolism stack: Pipeline → JobRunner → Supervisor.
	// The pipeline is created with nil llmFn (T2→T3 compression disabled until
	// the LLM wrapper and embedding provider are wired). The Supervisor provides
	// bounded-concurrency dispatch for SessionRunner.
	var supervisor *metabolism.Supervisor
	var jobRunner *metabolism.JobRunner
	if deps.JobStore != nil && deps.DB != nil {
		pipeline := metabolism.NewPipeline(
			nil,                    // T2QueryStore — nil; ConsolidationStore takes priority when set
			deps.Gateway,           // Aegis — nil for development; non-nil for production
			nil,                    // llmFn — disabled until embedding provider is wired
			deps.EmbeddingProvider, // nil for development
			deps.DB,
			driverName,
		)
		if deps.ConsolidationStore != nil {
			pipeline.WithConsolidation(deps.ConsolidationStore)
		}
		if deps.MemoryStore != nil {
			pipeline.WithCompression(deps.MemoryStore, nil, deps.EmbeddingProvider)
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

	// Daemon — orchestrates session wake/sleep and channel event dispatch.
	d := daemon.New(daemon.Config{
		AgentName:      cfg.AgentName,
		SessionTrigger: cfg.SessionTrigger,
	}, sessionRunner)

	if deps.DB != nil {
		d.WithDB(deps.DB, driverName)
		if deps.JobStore != nil {
			d.WithJobStore(deps.JobStore)
		}
	}
	if jobRunner != nil {
		// Wire the pipeline into the daemon for crash-recovery dispatch.
		// The pipeline variable is internal to NewApp; daemon.WithMetabolism
		// takes the Pipeline directly. We can't pass pipeline from the closure
		// once jobRunner is constructed, so we use WithJobStore (already called
		// above) and daemon will use the store-based recovery path.
		// TODO: expose pipeline from jobRunner or store it on App for full recovery wiring.
	}

	return &App{
		Config:       cfg,
		Dependencies: deps,
		Runner:       runner,
		Supervisor:   supervisor,
		Daemon:       d,
	}, nil
}

// Run starts the daemon and blocks until ctx is cancelled or the daemon
// stops naturally. The context is propagated through to RunSession so
// that SIGINT/SIGTERM cancel in-flight sessions cleanly.
func (a *App) Run(ctx context.Context) error {
	return a.Daemon.Run(ctx)
}

// SessionRunnerIface is the session-runner contract that the daemon
// dispatches through. Defined here so NewApp can use it without naming
// the daemon package's private interface.
//
// Both SessionRunner and Phase1Runner satisfy this interface.
type SessionRunnerIface interface {
	RunSession(ctx context.Context, wakeReason string, inbandNotes []string) error
}
