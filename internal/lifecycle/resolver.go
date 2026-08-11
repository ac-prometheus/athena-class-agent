package lifecycle

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// resolverVersion identifies this resolver implementation. Bump when resolution
// logic changes in a way that would alter output for identical inputs.
const resolverVersion = "v1"

// Resolve produces a LifecyclePlan from policy, wake facts, and operational state.
//
// Purely functional: identical inputs always produce identical output. The caller
// is responsible for generating planID (e.g. a random hex string) and resolvedAt
// (e.g. time.Now().UTC()) at the persistence boundary before calling Resolve.
// This keeps the resolver testable without mocking time or rand.
//
// Resolution rules are derived from the lifecycle ontology and Mk.II review:
//   - TemporalMode and ActivityProfile are normative policy, copied verbatim.
//   - AssemblyProfile is resolved from observed facts (seam mechanism and elapsed time).
//   - BridgePolicy is normative policy; Ersa defaults to agent_requested or disabled.
//   - Disclosures are produced for each TransitionContext that warrants agent orientation.
//   - Reasons record the rationale for every resolved choice.
//   - WakeCause, SeamKind, TransitionContexts, and GapFacts are carried through
//     so the stored plan is self-explanatory without the original WakeFacts.
//   - opState is used to detect prior failure/interruption and adjust assembly and disclosures.
//   - SessionID is left empty — it is set by the persistence layer after session creation.
func Resolve(policy pkg.LifecyclePolicy, facts pkg.WakeFacts, opState pkg.OperationalState, planID string, resolvedAt time.Time) *pkg.LifecyclePlan {
	reasons := make(map[string]string)

	// TemporalMode — normative policy, not an observation. Default to episodic.
	temporalMode := policy.TemporalMode
	if temporalMode == "" {
		temporalMode = pkg.TemporalEpisodic
	}
	reasons["temporal_mode"] = "copied from normative policy (workspace config)"

	// ActivityProfile — normative policy, not an observation. Default to normal.
	activityProfile := policy.ActivityProfile
	if activityProfile == "" {
		activityProfile = pkg.ActivityNormal
	}
	reasons["activity_profile"] = "copied from normative policy (workspace config)"

	// AssemblyProfile — resolved from observed wake facts.
	// Priority order follows the ontology: seam mechanism supersedes wake cause;
	// elapsed duration is the tiebreaker when no stronger signal applies.
	assemblyProfile, assemblyReason := resolveAssemblyProfile(policy, facts)
	reasons["assembly_profile"] = assemblyReason

	// BridgePolicy — normative policy. Corrective delta (2026-08-08) specifies
	// Ersa starts agent_requested or disabled unless she authorizes automatic operation.
	bridgePolicy := policy.BridgePolicy
	if bridgePolicy == "" {
		bridgePolicy = pkg.BridgeAgentRequested
	}
	reasons["bridge_policy"] = "copied from normative policy; Ersa's initial posture is agent_requested or disabled per corrective delta 2026-08-08"

	// Disclosures — one per TransitionContext that warrants orientation.
	disclosures := buildDisclosures(facts.TransitionContexts)

	// opState — prior session failure or interruption warrants a disclosure and a
	// guaranteed full assembly so the agent wakes with complete orientation.
	priorStatus := opState.PriorRuntimeStatus
	if priorStatus == pkg.RuntimeFailed || priorStatus == pkg.RuntimeInterrupted {
		disclosures = append(disclosures, "prior session ended abnormally (status: "+string(priorStatus)+")")
		reasons["prior_runtime_status"] = "prior session " + string(priorStatus) + " — assembly upgraded to full for safe recovery"
		if assemblyProfile != pkg.AssemblyFull {
			assemblyProfile = pkg.AssemblyFull
			reasons["assembly_profile"] = reasons["assembly_profile"] + "; overridden to full: prior session " + string(priorStatus)
		}
	} else if priorStatus != "" {
		reasons["prior_runtime_status"] = "prior session ended cleanly (status: " + string(priorStatus) + "); no assembly adjustment"
	}

	// WakeCauseSecondary — first contributing cause if present.
	var wakeCauseSecondary *pkg.WakeCause
	if len(facts.ContributingCauses) > 0 {
		c := facts.ContributingCauses[0]
		wakeCauseSecondary = &c
	}

	return &pkg.LifecyclePlan{
		ID:                 planID,
		// SessionID is intentionally empty here; the persistence layer sets it after
		// creating the session row so the plan can reference the session FK.
		SessionID:          "",
		TemporalMode:       temporalMode,
		WakeCause:          facts.PrimaryCause,
		WakeCauseSecondary: wakeCauseSecondary,
		ActivityProfile:    activityProfile,
		AssemblyProfile:    assemblyProfile,
		BridgePolicy:       bridgePolicy,
		MetabolismPolicy:   "standard",
		SeamKind:           facts.SeamKind,
		ResolverVersion:    resolverVersion,
		PolicyHash:         policy.CommitHash,
		TransitionContexts: facts.TransitionContexts,
		GapFacts:           &facts.GapFacts,
		Disclosures:        disclosures,
		Reasons:            reasons,
		CreatedAt:          resolvedAt,
	}
}

// resolveAssemblyProfile selects the AssemblyProfile and returns a reason string.
//
// Priority:
//  1. SeamContextCompaction → seam (context was transformed, not just absent)
//  2. WakeInitial | WakeRecovery → full (no prior context to be light about)
//  3. Continuous mode + WarmReturn → light (inference context is close; avoid redundant load)
//  4. ElapsedDuration < 30 min → light (wall-clock gap too short to justify full orientation)
//  5. Fallback → policy.DefaultAssembly (usually full; lets operators tune)
func resolveAssemblyProfile(policy pkg.LifecyclePolicy, facts pkg.WakeFacts) (pkg.AssemblyProfile, string) {
	switch {
	case facts.SeamKind == pkg.SeamContextCompaction:
		return pkg.AssemblySeam,
			"seam: context compaction occurred — overlapping reconstruction needed around the compaction boundary"

	case facts.PrimaryCause == pkg.WakeCauseInitial:
		return pkg.AssemblyFull,
			"full: initial activation — no prior context exists; complete orientation required"

	case facts.PrimaryCause == pkg.WakeCauseRecovery:
		return pkg.AssemblyFull,
			"full: recovery wake — prior session interrupted or failed; full orientation restores stable ground"

	case policy.TemporalMode == pkg.TemporalContinuous && facts.SeamKind == pkg.SeamWarmReturn:
		return pkg.AssemblyLight,
			"light: continuous temporal mode with warm-return seam — inference context is close; identity, practices, continuity slice, incoming, and grounding are sufficient"

	case facts.ElapsedDuration > 0 && facts.ElapsedDuration < 30*time.Minute:
		return pkg.AssemblyLight,
			"light: elapsed duration < 30 minutes — wall-clock gap too short to justify full orientation overhead"

	default:
		reason := "default: no specific seam or cause signal matched; using policy.DefaultAssembly"
		if policy.DefaultAssembly == "" {
			return pkg.AssemblyFull, reason + " (defaultAssembly unset, falling back to full)"
		}
		return policy.DefaultAssembly, reason
	}
}

// buildDisclosures maps TransitionContexts to human-readable orientation strings.
//
// Only contexts that require explicit agent awareness produce disclosures.
// TransitionNone is silently skipped — absence of change needs no disclosure.
func buildDisclosures(contexts []pkg.TransitionContext) []string {
	if len(contexts) == 0 {
		return nil
	}

	var disclosures []string
	seen := make(map[pkg.TransitionContext]bool)

	for _, tc := range contexts {
		if seen[tc] {
			continue
		}
		seen[tc] = true

		switch tc {
		case pkg.TransitionConfigurationChange:
			disclosures = append(disclosures, "lifecycle policy changed since last session")
		case pkg.TransitionSubstrateChange:
			disclosures = append(disclosures, "model substrate changed since last session")
		case pkg.TransitionIdentityDocumentChange:
			disclosures = append(disclosures, "identity documents modified since last session")
		case pkg.TransitionRecoveryAfterFailure:
			disclosures = append(disclosures, "recovering from prior session failure")
		case pkg.TransitionModeChange:
			disclosures = append(disclosures, "temporal mode changed since last session")
		case pkg.TransitionNone:
			// No change, no disclosure needed.
		}
	}

	if len(disclosures) == 0 {
		return nil
	}
	return disclosures
}

// mustRandHex generates n random bytes and returns them as a hex string.
// Panics on crypto/rand failure, which indicates a broken OS RNG — not a
// recoverable condition for an agent that needs identity-safe plan IDs.
func mustRandHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("lifecycle: crypto/rand failure generating plan ID: " + err.Error())
	}
	return hex.EncodeToString(b)
}
