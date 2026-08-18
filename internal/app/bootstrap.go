package app

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/internal/platform/storage"
)

// Bootstrap constructs a validated Dependencies bundle from configuration.
// It opens the database, creates all repository stores, and validates the
// result against the requested profile. Returns a ready-to-use bundle or
// an error if required dependencies cannot be constructed.
//
// Optional dependencies (per profile) that fail construction are logged
// and left nil — they will surface as warnings during Validate.
func Bootstrap(cfg *platform.Config, profile RuntimeProfile) (*Dependencies, error) {
	deps := &Dependencies{Config: cfg}

	// Database
	driverName := DriverNameFromDSN(cfg.DatabaseDSN)
	if cfg.DatabaseDSN != "" {
		store, err := platform.NewStore(cfg.DatabaseDSN)
		if err != nil {
			if profile == ProfileConnectivityTest || profile == ProfileErsaProduction {
				return nil, fmt.Errorf("bootstrap: DB required for %s: %w", profile, err)
			}
			slog.Warn("bootstrap: DB unavailable", "err", err)
		} else {
			deps.DB = platform.WrapSQLDB(store.DB)
		}
	}

	// Repository stores (require DB)
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
			return nil, fmt.Errorf("bootstrap: postgres driver not yet supported — sqlite3 required for current profiles (driver=%q)", driverName)
		}
	}

	// LLM, Aegis, MemoryStore, EmbeddingProvider, and ToolRegistry are
	// constructed by cmd/agent (they depend on API keys, provider selection,
	// and runtime flags). Bootstrap sets the infrastructure; the executable
	// fills in the rest before calling Validate.
	//
	// When HARN-79 (extract lifecycleRunner) lands, those constructors move
	// here too. For now, the executable sets them on deps directly:
	//   deps.LLM = engine.NewOpenAICompatClient(...)
	//   deps.Gateway = aegis.NewGateway(...)
	//   deps.ToolRegistry = tools.NewRegistry(...)

	// Validate the assembled bundle against the requested profile.
	if err := deps.Validate(profile); err != nil {
		return nil, fmt.Errorf("bootstrap: %w", err)
	}

	return deps, nil
}

// DriverNameFromDSN infers the database driver from the DSN string.
func DriverNameFromDSN(dsn string) string {
	if dsn == "" {
		return "sqlite3"
	}
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		return "postgres"
	}
	return "sqlite3"
}

// ProfileFromEnv reads the AGENT_PROFILE environment variable and returns
// the corresponding RuntimeProfile. Empty defaults to ProfileDevelopment.
func ProfileFromEnv() (RuntimeProfile, error) {
	return ParseProfile(os.Getenv("AGENT_PROFILE"))
}
