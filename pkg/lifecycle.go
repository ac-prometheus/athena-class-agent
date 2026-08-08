package pkg

import "time"

// ---------------------------------------------------------------------------
// Lifecycle Ontology Types — Sprint 3B
//
// These types encode the vocabulary defined in docs/lifecycle/lifecycle_ontology.md.
// Each dimension is independent; the lifecycle resolver combines them into a
// LifecyclePlan without erasing any observed fact.
// ---------------------------------------------------------------------------

// WakeCause describes what triggered an activation.
type WakeCause string

const (
	WakeCauseInitial         WakeCause = "initial"
	WakeCauseExternal        WakeCause = "external"
	WakeCauseScheduled       WakeCause = "scheduled"
	WakeCauseHeartbeat       WakeCause = "heartbeat"
	WakeCauseAgentRequested  WakeCause = "agent_requested"
	WakeCauseContextPressure WakeCause = "context_pressure"
	WakeCauseManual          WakeCause = "manual"
	WakeCauseRecovery        WakeCause = "recovery"
)

// ValidWakeCauses is the set of valid WakeCause values.
var ValidWakeCauses = map[WakeCause]bool{
	WakeCauseInitial:         true,
	WakeCauseExternal:        true,
	WakeCauseScheduled:       true,
	WakeCauseHeartbeat:       true,
	WakeCauseAgentRequested:  true,
	WakeCauseContextPressure: true,
	WakeCauseManual:          true,
	WakeCauseRecovery:        true,
}

// TransitionContext describes what happened at the boundary.
type TransitionContext string

const (
	TransitionNone                   TransitionContext = "none"
	TransitionModeChange             TransitionContext = "mode_change"
	TransitionSubstrateChange        TransitionContext = "substrate_change"
	TransitionConfigurationChange    TransitionContext = "configuration_change"
	TransitionRecoveryAfterFailure   TransitionContext = "recovery_after_failure"
	TransitionIdentityDocumentChange TransitionContext = "identity_document_change"
)

// ValidTransitionContexts is the set of valid TransitionContext values.
var ValidTransitionContexts = map[TransitionContext]bool{
	TransitionNone:                   true,
	TransitionModeChange:             true,
	TransitionSubstrateChange:        true,
	TransitionConfigurationChange:    true,
	TransitionRecoveryAfterFailure:   true,
	TransitionIdentityDocumentChange: true,
}

// SeamKind describes the relationship mechanism between sessions.
type SeamKind string

const (
	SeamColdWake           SeamKind = "cold_wake"
	SeamWarmReturn         SeamKind = "warm_return"
	SeamContextCompaction  SeamKind = "context_compaction"
	SeamRestReturn         SeamKind = "rest_return"
	SeamNone               SeamKind = "none"
)

// ValidSeamKinds is the set of valid SeamKind values.
var ValidSeamKinds = map[SeamKind]bool{
	SeamColdWake:          true,
	SeamWarmReturn:        true,
	SeamContextCompaction: true,
	SeamRestReturn:        true,
	SeamNone:              true,
}

// ActivityProfile describes the purpose and attention posture of the activation.
type ActivityProfile string

const (
	ActivityNormal   ActivityProfile = "normal"
	ActivityFree     ActivityProfile = "free"
	ActivitySentinel ActivityProfile = "sentinel"
)

// ValidActivityProfiles is the set of valid ActivityProfile values.
var ValidActivityProfiles = map[ActivityProfile]bool{
	ActivityNormal:   true,
	ActivityFree:     true,
	ActivitySentinel: true,
}

// AssemblyProfile is resolved behavior describing context assembly depth.
type AssemblyProfile string

const (
	AssemblyFull    AssemblyProfile = "full"
	AssemblyLight   AssemblyProfile = "light"
	AssemblyMinimal AssemblyProfile = "minimal"
	AssemblySeam    AssemblyProfile = "seam"
)

// ValidAssemblyProfiles is the set of valid AssemblyProfile values.
var ValidAssemblyProfiles = map[AssemblyProfile]bool{
	AssemblyFull:    true,
	AssemblyLight:   true,
	AssemblyMinimal: true,
	AssemblySeam:    true,
}

// GapClass categorizes the duration between sessions for policy decisions.
type GapClass string

const (
	GapNone      GapClass = "none"
	GapShort     GapClass = "short"
	GapOvernight GapClass = "overnight"
	GapLong      GapClass = "long"
)

// GapFacts captures observed temporal gap data between sessions.
// The measured duration remains canonical; gap_class is a derived policy convenience.
type GapFacts struct {
	PreviousActivityAt *time.Time    `json:"previous_activity_at,omitempty"`
	WakeAt             time.Time     `json:"wake_at"`
	ElapsedDuration    time.Duration `json:"elapsed_duration"`
	ClockBasis         string        `json:"clock_basis"` // "wall" or "monotonic"
	GapClass           GapClass      `json:"gap_class"`
	MessagesPending    int           `json:"messages_pending"`
	VaultChanges       int           `json:"vault_changes"`
}

// LifecyclePlan is the output of the lifecycle resolver.
// Given identical inputs, the resolver produces identical output.
// It performs no I/O and changes no state.
type LifecyclePlan struct {
	ID                string          `json:"id"`
	SessionID         string          `json:"session_id"`
	TemporalMode      string          `json:"temporal_mode"`
	WakeCause         WakeCause       `json:"wake_cause"`
	WakeCauseSecondary *WakeCause     `json:"wake_cause_secondary,omitempty"`
	ActivityProfile   ActivityProfile `json:"activity_profile"`
	AssemblyProfile   AssemblyProfile `json:"assembly_profile"`
	BridgePolicy      string          `json:"bridge_policy"`
	MetabolismPolicy  string          `json:"metabolism_policy"`
	SeamKind          SeamKind        `json:"seam_kind"`
	ResolverVersion   string          `json:"resolver_version"`
	ResolvedAt        time.Time       `json:"resolved_at"`
	PolicyHash        string          `json:"policy_hash,omitempty"`
	GapFacts          *GapFacts       `json:"gap_facts,omitempty"`
	TransitionContext TransitionContext `json:"transition_context"`
	Reasons           []string        `json:"reasons,omitempty"`
}

// RuntimeStatus describes the session process state machine.
type RuntimeStatus string

const (
	RuntimeStarting    RuntimeStatus = "starting"
	RuntimeActive      RuntimeStatus = "active"
	RuntimeWaiting     RuntimeStatus = "waiting"
	RuntimeEnding      RuntimeStatus = "ending"
	RuntimeEnded       RuntimeStatus = "ended"
	RuntimeInterrupted RuntimeStatus = "interrupted"
	RuntimeFailed      RuntimeStatus = "failed"
)

// AttentionProfile describes the agent's current attention posture.
type AttentionProfile string

const (
	AttentionNormal  AttentionProfile = "normal"
	AttentionFocused AttentionProfile = "focused"
)

// MetabolismStatus describes the state of end-of-session metabolism.
type MetabolismStatus string

const (
	MetabolismNotRequired    MetabolismStatus = "not_required"
	MetabolismQueued         MetabolismStatus = "queued"
	MetabolismRunning        MetabolismStatus = "running"
	MetabolismComplete       MetabolismStatus = "complete"
	MetabolismPartial        MetabolismStatus = "partial"
	MetabolismFailedRetry    MetabolismStatus = "failed_retryable"
	MetabolismFailedTerminal MetabolismStatus = "failed_terminal"
)
