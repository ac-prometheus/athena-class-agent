package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ac-prometheus/athena-class-agent/internal/assembly"
	"github.com/ac-prometheus/athena-class-agent/internal/engine"
	"github.com/ac-prometheus/athena-class-agent/internal/lifecycle"
	"github.com/ac-prometheus/athena-class-agent/internal/metabolism"
	"github.com/ac-prometheus/athena-class-agent/internal/session"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// SessionRunner implements daemon.SessionRunner with full lifecycle resolution,
// context assembly, engine loop, and end-of-session metabolism dispatch.
type SessionRunner struct {
	deps       *Dependencies
	assembler  *assembly.ContextAssembler
	jobRunner  *metabolism.JobRunner  // nil when metabolism not wired
	supervisor *metabolism.Supervisor // preferred over jobRunner when set (bounded concurrency)
	logger     *slog.Logger
}

// NewSessionRunner creates a lifecycle session runner. The jobRunner may be nil
// if metabolism is not yet wired (pipeline or store missing).
func NewSessionRunner(deps *Dependencies, assembler *assembly.ContextAssembler, jobRunner *metabolism.JobRunner, logger *slog.Logger) *SessionRunner {
	if logger == nil {
		logger = slog.Default()
	}
	return &SessionRunner{
		deps:      deps,
		assembler: assembler,
		jobRunner: jobRunner,
		logger:    logger,
	}
}

// WithSupervisor attaches a Supervisor for bounded-concurrency dispatch.
// When set, metabolism jobs are submitted through the supervisor instead of
// the job runner directly, providing concurrency control and graceful drain.
func (r *SessionRunner) WithSupervisor(s *metabolism.Supervisor) *SessionRunner {
	r.supervisor = s
	return r
}

// RunSession executes a full agent session: resolve lifecycle plan → assemble
// context → run engine loop → end session → dispatch metabolism.
//
// ctx is the daemon's context — cancellation propagates into the engine loop
// so external signals (SIGINT, test deadlines) can terminate sessions cleanly.
// trigger carries the wake reason and any inbound event content; when
// InboundContent is non-empty the content is routed as the engine's initial
// user message so the agent can respond to the triggering event.
func (r *SessionRunner) RunSession(ctx context.Context, trigger pkg.SessionTrigger) error {
	wakeReason := trigger.WakeReason
	inbandNotes := trigger.InbandNotes

	// 1. Read policy from workspace file. Missing file is not an error — use defaults.
	policyReader := session.NewPolicyReader(r.deps.Config.WorkspaceDir, r.deps.DB)
	policy, policyHash, err := policyReader.Read()
	if err != nil {
		r.logger.Warn("lifecycle: policy read error — loading last valid snapshot", "err", err)
		lastPolicy := queryLastPolicy(ctx, r.deps.DB)
		if lastPolicy != nil {
			policy = *lastPolicy
			r.logger.Info("lifecycle: fell back to last valid policy from lifecycle_plans")
		} else {
			policy = pkg.LifecyclePolicy{
				TemporalMode:    pkg.TemporalEpisodic,
				DefaultAssembly: pkg.AssemblyFull,
				BridgePolicy:    pkg.BridgeAgentRequested,
				ActivityProfile: pkg.ActivityNormal,
			}
		}
		policyHash = ""
	}

	// 2. Check for policy change and produce a disclosure note if changed.
	// Disclosure is recorded after session creation (step 7b) so it gets the real session ID.
	var disclosureNote string
	var prevHash string
	if policyHash != "" {
		changed, ph, changeErr := policyReader.HasChanged(ctx, policyHash)
		if changeErr != nil {
			r.logger.Warn("lifecycle: policy change check failed", "err", changeErr)
		} else if changed {
			prevHash = ph
			oldPolicy := queryLastPolicy(ctx, r.deps.DB)
			disclosure := session.GenerateDisclosure(oldPolicy, &policy)
			disclosure.PolicyPath = policyReader.PolicyPath()
			disclosure.OldHash = prevHash
			disclosure.NewHash = policyHash
			disclosureNote = disclosure.ForContext()
			r.logger.Info("lifecycle: policy change detected",
				"new_hash", policyHash, "prev_hash", prevHash)
		}
	}

	// 3. Build WakeFacts.
	wakeAt := time.Now()
	wakeCause := wakeReasonToCause(wakeReason)

	var prevActivityAt *time.Time
	if r.deps.LifecycleStore != nil {
		if t, err := r.deps.LifecycleStore.LastWakeAt(ctx); err != nil {
			r.logger.Debug("lifecycle: LastWakeAt via store failed", "err", err)
		} else if !t.IsZero() {
			prevActivityAt = &t
		}
	} else if r.deps.DB != nil {
		prevActivityAt = queryLastWakeAt(ctx, r.deps.DB)
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
	opState := queryOperationalState(ctx, r.deps)

	// 5. Resolve lifecycle plan — purely functional, no I/O.
	plan := lifecycle.Resolve(policy, facts, opState, mustRandHex(16), wakeAt.UTC())

	// 6. Create session.
	sess := session.NewSession(r.deps.Config.AgentName, string(plan.WakeCause))

	// 7. Backfill SessionID into plan.
	plan.SessionID = sess.GetID()

	// 7a. Persist lifecycle artifacts — fail-closed for production integrity.
	if r.deps.LifecycleStore != nil {
		if pErr := r.deps.LifecycleStore.RecordPlan(ctx, plan); pErr != nil {
			return fmt.Errorf("lifecycle: RecordPlan failed (fail-closed): %w", pErr)
		}
		if wErr := r.deps.LifecycleStore.RecordWakeFacts(ctx, sess.GetID(), &facts); wErr != nil {
			return fmt.Errorf("lifecycle: RecordWakeFacts failed (fail-closed): %w", wErr)
		}
	}

	// 7b. Record disclosure with real session ID (moved from step 2).
	if disclosureNote != "" && r.deps.LifecycleStore != nil {
		if dErr := r.deps.LifecycleStore.RecordDisclosure(ctx, sess.GetID(), policyReader.PolicyPath(), policyHash, prevHash, disclosureNote); dErr != nil {
			return fmt.Errorf("lifecycle: RecordDisclosure failed (fail-closed): %w", dErr)
		}
	}

	// 8. Build AssembleConfig.
	notes := make([]string, 0, len(inbandNotes)+1)
	notes = append(notes, inbandNotes...)
	if disclosureNote != "" {
		notes = append(notes, disclosureNote)
	}

	assembleCfg := assembly.MinimalAssembleConfig()
	assembleCfg.Plan = plan
	assembleCfg.SessionID = sess.GetID()
	assembleCfg.InbandNotes = notes
	assembleCfg.DB = r.deps.DB
	assembleCfg.AssemblyStore = r.deps.AssemblyStore
	assembleCfg.SkipWitnessCheck = r.deps.Config.SkipWitnessCheck

	// Wire memory dependencies into assembly — enables continuity phase (T3 retrieval)
	// and echo-pool phase (semantic echo retrieval). Bridge is set to the designed
	// default AbstainRate of 0.20; BridgePolicy on the plan gates whether synthesis fires.
	// SetLLMFn is required alongside SetBridge: without it cfg.llmFn stays nil and
	// bridge synthesis never fires regardless of BridgePolicy (Fable 4B carry-forward C-1).
	if r.deps.MemoryStore != nil {
		assembleCfg.SetStore(r.deps.MemoryStore)
		assembleCfg.SetBridge(assembly.DefaultBridgeConfig())
	}
	if r.deps.LLM != nil {
		assembleCfg.SetLLMFn(makeLLMFn(r.deps.LLM))
	}
	if r.deps.EmbeddingProvider != nil {
		assembleCfg.SetProvider(r.deps.EmbeddingProvider)
	}

	// 9. Assemble context.
	assembled, err := r.assembler.Assemble(ctx, assembleCfg)
	if err != nil {
		return fmt.Errorf("lifecycle: assembling context: %w", err)
	}

	// 9a. Persist the assembly manifest — fail-closed for production integrity.
	if r.deps.LifecycleStore != nil && assembled.Manifest != nil {
		if mErr := r.deps.LifecycleStore.RecordManifest(ctx, assembled.Manifest); mErr != nil {
			return fmt.Errorf("lifecycle: RecordManifest failed (fail-closed): %w", mErr)
		}
	}

	// 10. Create engine with hooks and run the agentic loop.
	budget := assembly.NewTokenBudget(r.deps.Config.TokenBudget, r.deps.Config.HardFloorTokens)
	hooks := engine.NewHookPipeline()

	// T2LoggerHook: critical — a failed T2 write is data loss, not an advisory event.
	// Registered before BudgetMonitor so T2 persistence fires even when budget is tight.
	if r.deps.MemoryStore != nil {
		hooks.RegisterCritical(engine.NewT2LoggerHook(r.deps.MemoryStore))
	}

	// BudgetMonitorHook: advisory — warnings are useful but failure must not halt the session.
	hooks.Register(engine.NewBudgetMonitorHook(budget))

	// RecordTurn: advisory — updates the session turn counter used in checkpoint writes.
	hooks.Register(engine.NewFuncHook("record-turn", func(_ context.Context, tr engine.TurnResult) error {
		sess.RecordTurn(tr.PromptTokens, tr.CompletionTokens)
		return nil
	}))

	eng := engine.NewEngine(r.deps.LLM, nil, hooks)
	eng.WithSessionID(sess.GetID())
	if r.deps.Gateway != nil {
		eng.WithAegis(r.deps.Gateway)
	}

	// Build the initial user message. When the session was triggered by an
	// inbound channel event, route the content so the agent can respond to it
	// directly rather than treating every wake as a generic heartbeat.
	initialContent := "[session start]"
	if trigger.InboundContent != "" {
		var header strings.Builder
		header.WriteString("[inbound event]")
		if trigger.InboundSender != "" {
			header.WriteString("\nfrom: ")
			header.WriteString(trigger.InboundSender)
		}
		if trigger.InboundChannel != "" {
			header.WriteString("\nchannel: ")
			header.WriteString(trigger.InboundChannel)
		}
		initialContent = header.String() + "\n\n" + trigger.InboundContent
	}

	req := pkg.CompletionRequest{
		System: assembled.SystemPrompt,
		Messages: []pkg.Message{
			{Role: "user", Content: initialContent},
		},
		MaxTokens: 4096,
	}

	loopResult, err := eng.RunLoop(ctx, req, engine.EngineConfig{
		MaxIterations: 25,
		ParallelTools: true,
		ShouldStop: func(resp *pkg.CompletionResponse, _ []pkg.ToolResult) bool {
			return budget.Level() == assembly.BudgetHard
		},
	})
	if err != nil {
		return fmt.Errorf("lifecycle: engine loop: %w", err)
	}
	r.logger.Info("lifecycle: engine loop complete",
		"iterations", loopResult.Iterations,
		"terminated", loopResult.Terminated,
		"tokens_remaining", budget.Remaining(),
	)

	// 10a. Write active turn checkpoint for crash recovery.
	if r.deps.LifecycleStore != nil {
		tokensUsed := r.deps.Config.TokenBudget - budget.Remaining()
		if cpErr := r.deps.LifecycleStore.WriteCheckpoint(ctx, sess.GetID(), sess.TurnCount(), "", tokensUsed, "active"); cpErr != nil {
			r.logger.Warn("lifecycle: WriteCheckpoint failed", "err", cpErr)
		}
	}

	// 11. End session.
	if err := sess.End(); err != nil {
		return fmt.Errorf("lifecycle: session end: %w", err)
	}

	// 11a. Mark checkpoint completed.
	if r.deps.LifecycleStore != nil {
		tokensUsed := r.deps.Config.TokenBudget - budget.Remaining()
		if cpErr := r.deps.LifecycleStore.WriteCheckpoint(ctx, sess.GetID(), sess.TurnCount(), "", tokensUsed, "completed"); cpErr != nil {
			r.logger.Warn("lifecycle: completed checkpoint failed", "err", cpErr)
		}
	}

	// 12. Dispatch metabolism via Supervisor (preferred for concurrency control),
	// falling back to JobRunner or direct store commit when unavailable.
	// plan.MetabolismPolicy carries the validated policy value from the resolver;
	// the resolver guarantees it is non-empty (defaults to "standard").
	if r.supervisor != nil {
		if err := r.supervisor.Submit(ctx, sess.GetID(), plan.MetabolismPolicy); err != nil {
			r.logger.Warn("lifecycle: metabolism supervisor submit failed — skipping pipeline",
				"session", sess.GetID(), "err", err)
		}
	} else if r.jobRunner != nil {
		if err := r.jobRunner.Submit(ctx, sess.GetID(), plan.MetabolismPolicy); err != nil {
			r.logger.Warn("lifecycle: metabolism submit failed — skipping pipeline",
				"session", sess.GetID(), "err", err)
		}
	} else if r.deps.JobStore != nil {
		// Fallback: store-only path (no pipeline wired).
		if _, err := r.deps.JobStore.Commit(ctx, sess.GetID(), plan.MetabolismPolicy); err != nil {
			r.logger.Warn("lifecycle: metabolism job commit failed",
				"session", sess.GetID(), "err", err)
		}
	}

	return nil
}
