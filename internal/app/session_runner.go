package app

import (
	"context"
	"fmt"
	"log/slog"
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
func (r *SessionRunner) RunSession(ctx context.Context, wakeReason string, inbandNotes []string) error {

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
	var disclosureNote string
	if policyHash != "" {
		changed, prevHash, changeErr := policyReader.HasChanged(ctx, policyHash)
		if changeErr != nil {
			r.logger.Warn("lifecycle: policy change check failed", "err", changeErr)
		} else if changed {
			oldPolicy := queryLastPolicy(ctx, r.deps.DB)
			disclosure := session.GenerateDisclosure(oldPolicy, &policy)
			disclosure.PolicyPath = policyReader.PolicyPath()
			disclosure.OldHash = prevHash
			disclosure.NewHash = policyHash
			disclosureNote = disclosure.ForContext()
			r.logger.Info("lifecycle: policy change detected",
				"new_hash", policyHash, "prev_hash", prevHash)

			if r.deps.LifecycleStore != nil {
				if dErr := r.deps.LifecycleStore.RecordDisclosure(ctx, "", policyReader.PolicyPath(), policyHash, prevHash, disclosureNote); dErr != nil {
					r.logger.Warn("lifecycle: RecordDisclosure failed", "err", dErr)
				}
			}
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
	sess := session.NewSession(r.deps.Config.AgentName, string(plan.WakeCause), r.deps.DB)

	// 7. Backfill SessionID into plan.
	plan.SessionID = sess.GetID()

	// 7a. Persist lifecycle artifacts via stores when available.
	if r.deps.LifecycleStore != nil {
		if pErr := r.deps.LifecycleStore.RecordPlan(ctx, plan); pErr != nil {
			r.logger.Warn("lifecycle: RecordPlan failed", "err", pErr)
		}
		if wErr := r.deps.LifecycleStore.RecordWakeFacts(ctx, sess.GetID(), &facts); wErr != nil {
			r.logger.Warn("lifecycle: RecordWakeFacts failed", "err", wErr)
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
	assembleCfg.SkipWitnessCheck = r.deps.Config.SkipWitnessCheck

	// 9. Assemble context.
	assembled, err := r.assembler.Assemble(ctx, assembleCfg)
	if err != nil {
		return fmt.Errorf("lifecycle: assembling context: %w", err)
	}

	// 9a. Persist the assembly manifest via store when available.
	if r.deps.LifecycleStore != nil && assembled.Manifest != nil {
		if mErr := r.deps.LifecycleStore.RecordManifest(ctx, assembled.Manifest); mErr != nil {
			r.logger.Warn("lifecycle: RecordManifest failed", "err", mErr)
		}
	}

	// 10. Create engine with hooks and run the agentic loop.
	budget := assembly.NewTokenBudget(r.deps.Config.TokenBudget, r.deps.Config.HardFloorTokens)
	hooks := engine.NewHookPipeline()
	hooks.Register(engine.NewBudgetMonitorHook(budget))

	eng := engine.NewEngine(r.deps.LLM, nil, hooks)
	if r.deps.Gateway != nil {
		eng.WithAegis(r.deps.Gateway)
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
			level := budget.Add(resp.PromptTokens, resp.CompletionTokens)
			return level == assembly.BudgetHard
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

	// 11. End session.
	if err := sess.End(); err != nil {
		return fmt.Errorf("lifecycle: session end: %w", err)
	}

	// 12. Dispatch metabolism via Supervisor (preferred for concurrency control),
	// falling back to JobRunner or direct store commit when unavailable.
	if r.supervisor != nil {
		if err := r.supervisor.Submit(ctx, sess.GetID(), "standard"); err != nil {
			r.logger.Warn("lifecycle: metabolism supervisor submit failed — skipping pipeline",
				"session", sess.GetID(), "err", err)
		}
	} else if r.jobRunner != nil {
		if err := r.jobRunner.Submit(ctx, sess.GetID(), "standard"); err != nil {
			r.logger.Warn("lifecycle: metabolism submit failed — skipping pipeline",
				"session", sess.GetID(), "err", err)
		}
	} else if r.deps.JobStore != nil {
		// Fallback: store-only path (no pipeline wired).
		if _, err := r.deps.JobStore.Commit(ctx, sess.GetID(), "standard"); err != nil {
			r.logger.Warn("lifecycle: metabolism job commit failed",
				"session", sess.GetID(), "err", err)
		}
	}

	return nil
}
