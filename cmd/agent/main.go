package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ac-prometheus/athena-class-agent/internal/assembly"
	"github.com/ac-prometheus/athena-class-agent/internal/daemon"
	"github.com/ac-prometheus/athena-class-agent/internal/engine"
	"github.com/ac-prometheus/athena-class-agent/internal/lifecycle"
	"github.com/ac-prometheus/athena-class-agent/internal/metabolism"
	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/internal/session"
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

	// PHASE1_MODE=true gates the legacy single-turn runner for backward compat.
	// Default is the full lifecycle runner.
	// sessionRunner is unexported in daemon; use an anonymous interface here.
	type sessionRunner interface {
		RunSession(wakeReason string, inbandNotes []string) error
	}
	var runner sessionRunner
	var db platform.DB
	var driverName string
	if os.Getenv("PHASE1_MODE") == "true" {
		slog.Info("agent: running in phase1 compatibility mode")
		runner = &phase1Runner{
			cfg:       cfg,
			client:    client,
			assembler: assembler,
		}
	} else {
		// Open DB for lifecycle state. Gracefully degrade to nil if unavailable.
		driverName = driverNameFromDSN(cfg.DatabaseDSN)
		if store, err := platform.NewStore(cfg.DatabaseDSN); err != nil {
			slog.Warn("agent: DB unavailable — lifecycle runner will skip persistence", "err", err)
		} else {
			db = platform.WrapSQLDB(store.DB)
		}

		// Pipeline is nil when no T2QueryStore is available (no DB or no memory store).
		// TODO: wire T2QueryStore (platform.NewMemoryStore) in Phase 4+ when always available.
		var pipeline *metabolism.Pipeline
		if db != nil {
			// NewPipeline needs a T2QueryStore; defer until MemoryStore is wired.
			// For now pass nil — CommitJob will still record the job, goroutine skipped.
			pipeline = nil
		}

		runner = &lifecycleRunner{
			cfg:           cfg,
			client:        client,
			assembler:     assembler,
			db:            db,
			jobs:          nil, // nil until HARN-73 (SQLite store impl)
			consolidation: nil, // nil until HARN-73
			lifecycle:     nil, // nil until HARN-73
			assembly:      nil, // nil until HARN-73
			driverName:    driverName,
			pipeline:      pipeline,
			gateway:       nil, // TODO: wire Aegis gateway in Phase 5
		}
	}

	d := daemon.New(daemon.Config{
		AgentName:      cfg.AgentName,
		SessionTrigger: cfg.SessionTrigger,
	}, runner)

	if db != nil {
		d.WithDB(db, driverName)
	}

	return d.Run(context.Background())
}

// ---------------------------------------------------------------------------
// lifecycleRunner — full session lifecycle composition root (Sprint 3B+)
// ---------------------------------------------------------------------------

// lifecycleRunner implements daemon.SessionRunner with full lifecycle resolution,
// context assembly, engine loop, and end-of-session metabolism dispatch.
//
// Sprint 3E: repository ports replace raw platform.DB for domain operations.
// Each store may be nil — RunSession degrades gracefully when stores are absent.
// The db field is retained for subsystems not yet ported to store interfaces
// (session checkpoint, assembly witness check, policy reader).
type lifecycleRunner struct {
	cfg           *platform.Config
	client        pkg.LLMClient
	assembler     *assembly.ContextAssembler
	db            platform.DB               // retained for session/assembly/policy (pre-port)
	jobs          pkg.MetabolismJobStore     // nil = skip metabolism job commit
	consolidation pkg.ConsolidationStore     // nil = skip T2→T3 (passed to pipeline)
	lifecycle     pkg.LifecycleStore         // nil = skip plan/manifest/wake persistence
	assembly      pkg.AssemblyStore          // nil = skip witness/audit via store
	driverName    string
	pipeline      *metabolism.Pipeline
	gateway       pkg.ContentGateway // Aegis, may be nil
}

// RunSession executes a full agent session: resolve lifecycle plan → assemble
// context → run engine loop → end session → dispatch metabolism.
func (r *lifecycleRunner) RunSession(wakeReason string, inbandNotes []string) error {
	ctx := context.Background()

	// 1. Read policy from workspace file. Missing file is not an error — use defaults.
	policyReader := session.NewPolicyReader(r.cfg.WorkspaceDir, r.db)
	localPolicy, policyHash, err := policyReader.Read()
	if err != nil {
		slog.Warn("lifecycle: policy read error — using defaults", "err", err)
		localPolicy = nil
		policyHash = ""
	}
	pkgPolicy := localPolicyToPkg(localPolicy, policyHash)

	// 2. Check for policy change and produce a disclosure note if changed.
	var disclosureNote string
	if localPolicy != nil {
		changed, prevHash, changeErr := policyReader.HasChanged(ctx, policyHash)
		if changeErr != nil {
			slog.Warn("lifecycle: policy change check failed", "err", changeErr)
		} else if changed {
			disclosure := session.GenerateDisclosure(nil, localPolicy)
			disclosure.PolicyPath = policyReader.PolicyPath()
			disclosure.OldHash = prevHash
			disclosure.NewHash = policyHash
			disclosureNote = disclosure.ForContext()
			slog.Info("lifecycle: policy change detected",
				"new_hash", policyHash, "prev_hash", prevHash)

			// Persist the disclosure via store if available.
			if r.lifecycle != nil {
				if dErr := r.lifecycle.RecordDisclosure(ctx, "", policyReader.PolicyPath(), policyHash, prevHash, disclosureNote); dErr != nil {
					slog.Warn("lifecycle: RecordDisclosure failed", "err", dErr)
				}
			}
		}
	}

	// 3. Build WakeFacts.
	wakeAt := time.Now()
	wakeCause := wakeReasonToCause(wakeReason)

	var prevActivityAt *time.Time
	if r.lifecycle != nil {
		if t, err := r.lifecycle.LastWakeAt(ctx); err != nil {
			slog.Debug("lifecycle: LastWakeAt via store failed", "err", err)
		} else if !t.IsZero() {
			prevActivityAt = &t
		}
	} else if r.db != nil {
		prevActivityAt = queryLastWakeAt(ctx, r.db)
	}

	var elapsed time.Duration
	if prevActivityAt != nil {
		elapsed = wakeAt.Sub(*prevActivityAt)
	}

	facts := pkg.WakeFacts{
		PrimaryCause: wakeCause,
		GapFacts: pkg.GapFacts{
			PreviousActivityAt: prevActivityAt,
			WakeAt:             wakeAt,
			ElapsedDuration:    elapsed,
			ClockBasis:         "wall",
			GapClass:           classifyGap(elapsed, prevActivityAt),
		},
		ElapsedDuration: elapsed,
		SeamKind:        classifySeam(elapsed, prevActivityAt),
	}

	// 4. Build OperationalState from stores (preferred) or raw DB (fallback).
	// Zero-value on failure — the resolver handles the empty case gracefully.
	opState := r.queryOperationalState(ctx)

	// 5. Resolve lifecycle plan — purely functional, no I/O.
	plan := lifecycle.Resolve(pkgPolicy, facts, opState, mustRandHex(16), wakeAt.UTC())

	// 6. Create session.
	sess := session.NewSession(r.cfg.AgentName, string(plan.WakeCause), r.db)

	// 7. Backfill SessionID into plan (resolver intentionally leaves it empty).
	plan.SessionID = sess.GetID()

	// 7a. Persist lifecycle artifacts via stores when available.
	if r.lifecycle != nil {
		if pErr := r.lifecycle.RecordPlan(ctx, plan); pErr != nil {
			slog.Warn("lifecycle: RecordPlan failed", "err", pErr)
		}
		if wErr := r.lifecycle.RecordWakeFacts(ctx, sess.GetID(), &facts); wErr != nil {
			slog.Warn("lifecycle: RecordWakeFacts failed", "err", wErr)
		}
	}

	// 8. Build AssembleConfig. Merge inband notes with any disclosure generated above.
	notes := make([]string, 0, len(inbandNotes)+1)
	notes = append(notes, inbandNotes...)
	if disclosureNote != "" {
		notes = append(notes, disclosureNote)
	}

	assembleCfg := assembly.MinimalAssembleConfig()
	assembleCfg.Plan = plan
	assembleCfg.SessionID = sess.GetID()
	assembleCfg.InbandNotes = notes
	assembleCfg.DB = r.db
	assembleCfg.SkipWitnessCheck = r.cfg.SkipWitnessCheck

	// 9. Assemble context. Identity phase failure is fatal.
	assembled, err := r.assembler.Assemble(ctx, assembleCfg)
	if err != nil {
		return fmt.Errorf("lifecycle: assembling context: %w", err)
	}

	// 9a. Persist the assembly manifest via store when available.
	if r.lifecycle != nil && assembled.Manifest != nil {
		if mErr := r.lifecycle.RecordManifest(ctx, assembled.Manifest); mErr != nil {
			slog.Warn("lifecycle: RecordManifest failed", "err", mErr)
		}
	}

	// 10. Create engine with hooks and run the agentic loop.
	budget := assembly.NewTokenBudget(r.cfg.TokenBudget, r.cfg.HardFloorTokens)
	hooks := engine.NewHookPipeline()
	hooks.Register(engine.NewBudgetMonitorHook(budget))
	// TODO: register T2LoggerHook once MemoryStore is always wired (Phase 4+):
	//   hooks.Register(engine.NewT2LoggerHook(memStore))

	eng := engine.NewEngine(r.client, nil, hooks)
	if r.gateway != nil {
		eng.WithAegis(r.gateway)
	}

	req := pkg.CompletionRequest{
		System: assembled.SystemPrompt,
		Messages: []pkg.Message{
			{Role: "user", Content: "[session start]"},
		},
		MaxTokens: 4096,
	}

	loopResult, err := eng.RunLoop(ctx, req, engine.EngineConfig{
		MaxIterations: 25,
		ParallelTools: true,
		ShouldStop: func(resp *pkg.CompletionResponse, _ []pkg.ToolResult) bool {
			// Halt early when the hard budget floor is reached.
			level := budget.Add(resp.PromptTokens, resp.CompletionTokens)
			return level == assembly.BudgetHard
		},
	})
	if err != nil {
		return fmt.Errorf("lifecycle: engine loop: %w", err)
	}
	slog.Info("lifecycle: engine loop complete",
		"iterations", loopResult.Iterations,
		"terminated", loopResult.Terminated,
		"tokens_remaining", budget.Remaining(),
	)

	// 11. End session.
	if err := sess.End(); err != nil {
		return fmt.Errorf("lifecycle: session end: %w", err)
	}

	// 12. Dispatch metabolism: commit a durable job record first, then run async.
	// Prefer the MetabolismJobStore interface; fall back to raw DB+SQL.
	if r.jobs != nil {
		jobID, jobErr := r.jobs.Commit(ctx, sess.GetID(), "standard")
		if jobErr != nil {
			slog.Warn("lifecycle: metabolism job commit (store) failed — skipping pipeline",
				"session", sess.GetID(), "err", jobErr)
		} else {
			slog.Info("lifecycle: metabolism job committed", "job_id", jobID, "session", sess.GetID())
			if r.pipeline != nil {
				go func() {
					if pErr := r.pipeline.ProcessSession(context.Background(), sess.GetID()); pErr != nil {
						slog.Error("lifecycle: metabolism pipeline error",
							"session", sess.GetID(), "err", pErr)
						if fErr := r.jobs.Fail(ctx, jobID, pErr.Error()); fErr != nil {
							slog.Error("lifecycle: metabolism job fail record failed",
								"job_id", jobID, "err", fErr)
						}
						return
					}
					if cErr := r.jobs.Complete(ctx, jobID); cErr != nil {
						slog.Error("lifecycle: metabolism job complete record failed",
							"job_id", jobID, "err", cErr)
					}
				}()
			}
		}
	} else if r.db != nil {
		// Legacy path: raw SQL via metabolism package functions.
		job, jobErr := metabolism.CommitJob(ctx, r.db, r.driverName, sess.GetID(), "standard")
		if jobErr != nil {
			slog.Warn("lifecycle: metabolism job commit failed — skipping pipeline",
				"session", sess.GetID(), "err", jobErr)
		} else {
			slog.Info("lifecycle: metabolism job committed", "job_id", job.ID, "session", sess.GetID())
			if r.pipeline != nil {
				go func() {
					if pErr := r.pipeline.ProcessSession(context.Background(), sess.GetID()); pErr != nil {
						slog.Error("lifecycle: metabolism pipeline error",
							"session", sess.GetID(), "err", pErr)
					}
				}()
			}
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// phase1Runner — legacy single-turn connectivity runner
// ---------------------------------------------------------------------------

// phase1Runner implements daemon.SessionRunner for the Phase 1 single-turn loop.
// Kept as a fallback gated by PHASE1_MODE=true.
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

	sess := session.NewSession(r.cfg.AgentName, wakeReason, nil)
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

	sess.RecordTurn(turn.Response.PromptTokens, turn.Response.CompletionTokens)
	level := budget.Add(turn.Response.PromptTokens, turn.Response.CompletionTokens)

	slog.Info("agent: turn result",
		"content", turn.Response.TextContent(),
		"budget_level", level,
		"tokens_remaining", budget.Remaining(),
	)

	// Run post-turn hooks (no hooks registered in Phase 1).
	hookErr := hooks.RunAll(ctx, engine.TurnResult{
		SessionID:        sess.GetID(),
		TurnNumber:       sess.TurnCount(),
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

	if err := sess.WriteCheckpoint(ctx); err != nil {
		return fmt.Errorf("writing checkpoint: %w", err)
	}

	if err := sess.End(); err != nil {
		return fmt.Errorf("session end: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// localPolicyToPkg converts the session-layer LifecyclePolicy (file-parsed) to
// the pkg.LifecyclePolicy that the lifecycle resolver expects.
// Fields with no direct counterpart (ActivityProfile, CommitHash) use defaults.
func localPolicyToPkg(p *session.LifecyclePolicy, hash string) pkg.LifecyclePolicy {
	out := pkg.LifecyclePolicy{
		TemporalMode:    pkg.TemporalEpisodic,   // safe default
		DefaultAssembly: pkg.AssemblyFull,         // safe default
		BridgePolicy:    pkg.BridgeAgentRequested, // corrective delta 2026-08-08
		ActivityProfile: pkg.ActivityNormal,
		CommitHash:      hash,
	}
	if p == nil {
		return out
	}
	if p.TemporalMode != "" {
		out.TemporalMode = pkg.TemporalMode(p.TemporalMode)
	}
	if p.AssemblyProfile != "" {
		out.DefaultAssembly = pkg.AssemblyProfile(p.AssemblyProfile)
	}
	if p.BridgePolicy != "" {
		out.BridgePolicy = pkg.BridgePolicy(p.BridgePolicy)
	}
	return out
}

// wakeReasonToCause maps the daemon's freeform wake reason string to a
// canonical pkg.WakeCause. Unknown reasons default to WakeCauseScheduled.
func wakeReasonToCause(reason string) pkg.WakeCause {
	switch {
	case reason == "initial" || reason == "first-boot":
		return pkg.WakeCauseInitial
	case reason == "recovery":
		return pkg.WakeCauseRecovery
	case reason == "heartbeat" || reason == "daemon-startup":
		return pkg.WakeCauseScheduled
	case reason == "agent_requested" || reason == "agent-requested":
		return pkg.WakeCauseAgentRequested
	case reason == "context_pressure" || reason == "context-pressure":
		return pkg.WakeCauseContextPressure
	case reason == "manual":
		return pkg.WakeCauseManual
	case strings.HasPrefix(reason, "channel:"):
		return pkg.WakeCauseExternal
	default:
		return pkg.WakeCauseScheduled
	}
}

// classifyGap bins an elapsed duration into a GapClass.
// prevActivityAt == nil implies this is the first session ever (GapNone).
func classifyGap(elapsed time.Duration, prevActivityAt *time.Time) pkg.GapClass {
	if prevActivityAt == nil {
		return pkg.GapNone
	}
	switch {
	case elapsed < 2*time.Hour:
		return pkg.GapShort
	case elapsed < 16*time.Hour:
		return pkg.GapOvernight
	default:
		return pkg.GapLong
	}
}

// classifySeam maps observed gap facts to a SeamKind.
// Context compaction is detected by the daemon/keeper before calling RunSession
// and passed via wakeReason; here we use elapsed duration as a proxy.
func classifySeam(elapsed time.Duration, prevActivityAt *time.Time) pkg.SeamKind {
	if prevActivityAt == nil {
		return pkg.SeamColdWake
	}
	switch {
	case elapsed < 30*time.Minute:
		return pkg.SeamWarmReturn
	case elapsed < 16*time.Hour:
		return pkg.SeamRestReturn
	default:
		return pkg.SeamColdWake
	}
}

// queryLastWakeAt returns the most recent wake_at from wake_facts, or nil if
// the table is absent, empty, or the DB is unavailable.
// This serves as a best-effort proxy for the previous session's end time.
func queryLastWakeAt(ctx context.Context, db platform.DB) *time.Time {
	if db == nil {
		return nil
	}
	row := db.QueryRowContext(ctx,
		`SELECT wake_at FROM wake_facts ORDER BY wake_at DESC LIMIT 1`,
	)
	var t time.Time
	if err := row.Scan(&t); err != nil {
		if err != sql.ErrNoRows {
			slog.Debug("lifecycle: queryLastWakeAt error (table may not exist yet)", "err", err)
		}
		return nil
	}
	return &t
}

// queryOperationalState reads the previous session's runtime and metabolism
// status for resolver input. Prefers store interfaces; falls back to raw DB.
// Returns zero-value on any error — the resolver handles the empty case gracefully.
func (r *lifecycleRunner) queryOperationalState(ctx context.Context) pkg.OperationalState {
	var metaStatus pkg.MetabolismStatus
	var runtimeStatus pkg.RuntimeStatus

	// Metabolism status: prefer MetabolismJobStore, fall back to raw DB.
	if r.jobs != nil {
		if s, err := r.jobs.LastStatus(ctx); err == nil && s != "" {
			metaStatus = pkg.MetabolismStatus(s)
		}
	} else if r.db != nil {
		metaRow := r.db.QueryRowContext(ctx,
			`SELECT status FROM metabolism_jobs ORDER BY created_at DESC LIMIT 1`,
		)
		var rawStatus string
		if err := metaRow.Scan(&rawStatus); err == nil {
			metaStatus = pkg.MetabolismStatus(rawStatus)
		}
	}

	// Runtime status: prefer LifecycleStore, fall back to raw DB.
	if r.lifecycle != nil {
		if s, err := r.lifecycle.LastCheckpointState(ctx); err == nil && s != "" {
			switch s {
			case "interrupted", "active":
				runtimeStatus = pkg.RuntimeInterrupted
			default:
				runtimeStatus = pkg.RuntimeEnded
			}
		}
	} else if r.db != nil {
		cpRow := r.db.QueryRowContext(ctx,
			`SELECT state FROM session_checkpoints ORDER BY created_at DESC LIMIT 1`,
		)
		var rawState string
		if err := cpRow.Scan(&rawState); err == nil {
			switch rawState {
			case "interrupted":
				runtimeStatus = pkg.RuntimeInterrupted
			case "active":
				runtimeStatus = pkg.RuntimeInterrupted
			default:
				runtimeStatus = pkg.RuntimeEnded
			}
		}
	}

	return pkg.OperationalState{
		PriorRuntimeStatus:    runtimeStatus,
		PriorMetabolismStatus: metaStatus,
	}
}

// mustRandHex generates n random bytes and returns them as a hex string.
// Panics on crypto/rand failure — a broken OS RNG is not recoverable.
func mustRandHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("main: crypto/rand failure generating ID: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// driverNameFromDSN returns the database driver name ("sqlite3" or "postgres")
// from the DSN prefix. Returns "sqlite3" for unknown prefixes as a safe default.
func driverNameFromDSN(dsn string) string {
	switch {
	case strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://"):
		return "postgres"
	default:
		return "sqlite3"
	}
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
