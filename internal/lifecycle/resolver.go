package lifecycle

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// Resolve produces a LifecyclePlan from policy, wake facts, and operational state.
//
// Pure function: no I/O, no state mutation. Given identical inputs, identical
// output. Its output must be persisted before assembly begins. Rationale: the
// resolver's determinism makes it auditable — any downstream assembly decision
// can be traced back to a stored plan with a clear reason for each choice.
//
// Resolution rules are derived from the lifecycle ontology and Mk.II review:
//   - TemporalMode and ActivityProfile are normative policy, copied verbatim.
//   - AssemblyProfile is resolved from observed facts (seam mechanism and elapsed time).
//   - BridgePolicy is normative policy; Ersa defaults to agent_requested or disabled.
//   - Disclosures are produced for each TransitionContext that warrants agent orientation.
//   - Reasons record the rationale for every resolved choice.
func Resolve(policy pkg.LifecyclePolicy, facts pkg.WakeFacts, opState pkg.OperationalState) *pkg.LifecyclePlan {
	reasons := make(map[string]string)

	// TemporalMode — normative policy, not an observation.
	temporalMode := policy.TemporalMode
	reasons["temporal_mode"] = "copied from normative policy (workspace config)"

	// ActivityProfile — normative policy, not an observation.
	activityProfile := policy.ActivityProfile
	reasons["activity_profile"] = "copied from normative policy (workspace config)"

	// AssemblyProfile — resolved from observed wake facts.
	// Priority order follows the ontology: seam mechanism supersedes wake cause;
	// elapsed duration is the tiebreaker when no stronger signal applies.
	assemblyProfile, assemblyReason := resolveAssemblyProfile(policy, facts)
	reasons["assembly_profile"] = assemblyReason

	// BridgePolicy — normative policy. Corrective delta (2026-08-08) specifies
	// Ersa starts agent_requested or disabled unless she authorizes automatic operation.
	bridgePolicy := policy.BridgePolicy
	reasons["bridge_policy"] = "copied from normative policy; Ersa's initial posture is agent_requested or disabled per corrective delta 2026-08-08"

	// Disclosures — one per TransitionContext that warrants orientation.
	disclosures := buildDisclosures(facts.TransitionContexts)

	// ID — 16 random bytes encoded as hex. Not derived from inputs so that two
	// plans produced from identical inputs are still independently addressable.
	id := mustRandHex(16)

	return &pkg.LifecyclePlan{
		ID:              id,
		TemporalMode:    temporalMode,
		ActivityProfile: activityProfile,
		AssemblyProfile: assemblyProfile,
		BridgePolicy:    bridgePolicy,
		Disclosures:     disclosures,
		Reasons:         reasons,
		CreatedAt:       time.Now().UTC(),
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

	case facts.ElapsedDuration < 30*time.Minute:
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
		case pkg.TransitionNone, pkg.TransitionModeChange:
			// TransitionNone: no change, no disclosure needed.
			// TransitionModeChange: mode changes are reflected in TemporalMode itself;
			// the agent sees the new mode directly — no separate disclosure required.
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
