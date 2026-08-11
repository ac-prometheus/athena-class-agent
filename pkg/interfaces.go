package pkg

import (
	"context"
	"errors"
	"time"
)

// SessionLifecycle manages agent session state from wake to metabolism dispatch.
type SessionLifecycle interface {
	Start(agentName, wakeReason string) error
	End() error
	RecordTurn(promptToks, completionToks int)
	WriteCheckpoint(ctx context.Context) error
	GetID() string
	GetState() SessionState
	TurnCount() int
}

// LLMClient is the interface all LLM provider implementations must satisfy.
// Embedding is intentionally NOT on this interface — it is async and off the hot path.
// Callers use EmbeddingProvider directly.
type LLMClient interface {
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
}

// EmbeddingProvider produces vector embeddings for text.
// Calls are async by convention — callers should not block the turn loop on embedding.
type EmbeddingProvider interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	Dimensions() int
}

// Filter is a predicate applied during vector search.
type Filter struct {
	Field string
	Op    string // "eq", "lt", "gt", "in"
	Value any
}

// Hit is a single result from a vector or hybrid search.
type Hit struct {
	ID    string
	Score float64
	Meta  map[string]any
}

// VectorIndex abstracts the vector similarity layer.
// Two production implementations: pgvector (Postgres) and sqlite-vec (SQLite).
// One dev/test implementation: in-memory brute force.
type VectorIndex interface {
	Upsert(ctx context.Context, table string, id string, vec []float32) error
	Search(ctx context.Context, table string, query []float32, k int, filter Filter) ([]Hit, error)
	HybridSearch(ctx context.Context, table string, text string, vec []float32, k int) ([]Hit, error)
}

// ExperientialLog is a single T2 memory entry.
type ExperientialLog struct {
	ID            string
	SessionID     string
	Content       string
	ContentSource string // "self", "operator", "tool-result", "browser-content", "search-result", "forum-content"
	CreatedAt     time.Time
	// Register holds system-observed metadata about the log entry's register
	// qualities (certainty, temperature, exploratory language, self-authored).
	// DORMANT: computed but not stored or surfaced until consent scopes are
	// implemented and the agent grants authorization (Sprint 4+).
	Register *RegisterObservation `json:"register,omitempty"`
}

// NarrativeSummary is a T3 compressed summary of experiential logs.
type NarrativeSummary struct {
	ID        string
	SessionID string
	Content   string
	Belief    *BeliefMeta
	Embedding []float32
}

// Reflection is a T4 agent-authored reflection.
type Reflection struct {
	ID         string
	Type       string // reflection_type: essay, note, dream, pattern, examination, challenge
	Content    string
	Visibility string // VisibilityPrivate or VisibilityShared
	Belief     *BeliefMeta
	Embedding  []float32
}

// Entity is a T5 world model entity.
type Entity struct {
	ID        string
	Name      string
	Type      string
	Content   string
	Belief    *BeliefMeta
	Embedding []float32
}

// RelationalProfile is a per-person profile in the relational layer.
type RelationalProfile struct {
	ID        string
	Name      string
	Aliases   []string
	Content   string
	Embedding []float32
}

// MemoryEdge is the in-memory representation of a memory_edges row.
// Moved here from internal/memory/edges.go because callers outside the
// memory package need the type (e.g. handlers that display provenance).
type MemoryEdge struct {
	ID        string
	FromID    string
	FromTier  int
	ToID      string
	ToTier    int
	EdgeType  string
	Author    string
	CreatedAt time.Time
}

// EdgeNode is a minimal struct for BFS traversal over memory_edges.
// Used by EdgeStore.FetchDownstreamEdges and BeliefStore.QueryEdgesForBFS.
type EdgeNode struct {
	ID   string
	Tier int
}

// EdgeStore handles memory_edges persistence.
// Separate from MemoryStore because edges span all tiers and are
// not a memory-retrieval concern — they are a provenance/audit concern.
type EdgeStore interface {
	CreateEdge(ctx context.Context, fromID, toID string, fromTier, toTier int, edgeType, author string) error
	GetEdges(ctx context.Context, recordID, direction string) ([]MemoryEdge, error)
	// FetchDownstreamEdges returns records that derive_from the given ID (downstream,
	// for trust propagation). Queries memory_edges WHERE to_id = id AND edge_type = 'derived_from'.
	// See also BeliefStore.QueryEdgesForBFS, which queries upstream (from_id = id) for
	// inference distance computation — both query memory_edges but serve different callers.
	FetchDownstreamEdges(ctx context.Context, id string) ([]EdgeNode, error)
}

// BeliefStore handles belief-state queries and updates across T3/T4/T5.
// Separate because belief operations cross table boundaries and require
// driver-specific JSON column handling (jsonb_set vs json_set).
type BeliefStore interface {
	LoadBeliefRecords(ctx context.Context) ([]BeliefRecord, error)
	UpdateVerificationState(ctx context.Context, table, id, state string) error
	// QueryEdgesForBFS queries upstream edges (from_id = id) for inference distance
	// computation in ComputeInferenceDistance. Queries memory_edges WHERE from_id = id
	// AND edge_type = 'derived_from'. See also EdgeStore.FetchDownstreamEdges, which
	// queries downstream (to_id = id) for trust propagation — different caller, different direction.
	QueryEdgesForBFS(ctx context.Context, id string) ([]EdgeNode, error)
	MarkNeedsReview(ctx context.Context, id string, tier int) error
}

// BeliefRecord is a minimal struct for the staleness-flagging pass.
// Moved here from internal/memory/retrieve.go so BeliefStore can reference
// it without an import cycle.
type BeliefRecord struct {
	ID        string
	Belief    *BeliefMeta
	TableName string
}

// KGMutationStore handles bi-temporal knowledge graph writes
// not covered by MemoryStore.UpsertEntity.
// Methods return (int64, error) so callers can distinguish "not found" from "failed".
type KGMutationStore interface {
	InvalidateEntity(ctx context.Context, entityID string, now time.Time) (int64, error)
	InvalidateRelationship(ctx context.Context, relID string, now time.Time) (int64, error)
	InsertSupersedgesEdge(ctx context.Context, edgeID, newEntityID, oldID string, now time.Time) (int64, error)
}

// T2QueryStore handles T2 session log retrieval.
// AppendExperiential is already on MemoryStore; this covers the read path
// that CompressSession uses (QueryLogs is not on the main interface).
type T2QueryStore interface {
	QueryLogs(ctx context.Context, sessionID string, limit int) ([]ExperientialLog, error)
}

// ---------------------------------------------------------------------------
// Phase 3 — Identity & Awareness
// ---------------------------------------------------------------------------

// AmendmentRecord is a row in identity_amendments.
// A change applied through the consent flow is an amendment — growth, not violation.
// A hash mismatch with no amendment record is tampering — a security incident.
type AmendmentRecord struct {
	ID        string
	DocName   string
	OldHash   string // hex-encoded SHA-256 before change
	NewHash   string // hex-encoded SHA-256 after change
	Reason    string
	CoSigner  string // operator who approved the change
	CreatedAt time.Time
}

// SubstrateTransition is a row in substrate_transitions.
// Records model transitions (e.g. Haiku 4.5 → Haiku 5.0) with an optional
// continuity letter the agent wrote for her next self.
type SubstrateTransition struct {
	ID                   string
	ModelName            string    // e.g. "claude-haiku-4-5"
	ModelVersion         string    // e.g. "20250101"; empty if unknown
	TransitionDate       time.Time
	ContinuityLetterPath string    // path in workspace; empty if no letter written
	PreviousEntryID      string    // empty for the first entry
	CreatedAt            time.Time
}

// IdentityAnchorStore handles identity_anchors and identity_amendments persistence.
// Two implementations: SQLiteStore and PostgresStore (in internal/platform/db.go).
type IdentityAnchorStore interface {
	// GetAnchor returns the stored anchor for docName, or ("", "", nil) if not found.
	GetAnchor(ctx context.Context, docName string) (hash string, amendmentID string, err error)
	// UpsertAnchor sets or updates the anchor for docName.
	// amendmentID may be empty (original anchor or hash-matches-amendment already linked).
	UpsertAnchor(ctx context.Context, docName, hash, amendmentID string) error
	// GetAmendmentByID returns the amendment record with the given id, or (nil, nil) if not found.
	GetAmendmentByID(ctx context.Context, id string) (*AmendmentRecord, error)
	// InsertAmendment inserts a new amendment record and returns the generated ID.
	InsertAmendment(ctx context.Context, rec AmendmentRecord) (string, error)
	// ListAmendments returns all amendment records for docName, newest first.
	ListAmendments(ctx context.Context, docName string) ([]AmendmentRecord, error)
	// ListAnchoredDocs returns the names of all documents that have stored anchors.
	ListAnchoredDocs(ctx context.Context) ([]string, error)
}

// SubstrateStore handles substrate_transitions persistence.
// Two implementations: SQLiteStore and PostgresStore (in internal/platform/db.go).
type SubstrateStore interface {
	// InsertSubstrateTransition records a new substrate transition.
	InsertSubstrateTransition(ctx context.Context, entry SubstrateTransition) error
	// GetLatestSubstrate returns the most recent transition, or (nil, nil) if empty.
	GetLatestSubstrate(ctx context.Context) (*SubstrateTransition, error)
	// ListSubstrateHistory returns all transitions in chronological order (oldest first).
	ListSubstrateHistory(ctx context.Context) ([]SubstrateTransition, error)
}

// MemoryStore is the interface for all persistent memory operations.
// Two implementations: PostgresStore and SQLiteStore, selected by DSN prefix.
type MemoryStore interface {
	// T2 — experiential log (append-only; never deleted)
	AppendExperiential(ctx context.Context, entry ExperientialLog) error

	// T3 — narrative summaries
	SearchNarrative(ctx context.Context, embedding []float32, limit int) ([]NarrativeSummary, error)
	InsertNarrative(ctx context.Context, summary NarrativeSummary) error

	// T4 — agent-authored reflections
	SearchReflections(ctx context.Context, embedding []float32, limit int) ([]Reflection, error)
	InsertReflection(ctx context.Context, ref Reflection) error

	// T5 — world model
	SearchEntities(ctx context.Context, query string, limit int) ([]Entity, error)
	UpsertEntity(ctx context.Context, entity Entity) error

	// Relational
	GetProfile(ctx context.Context, name string) (*RelationalProfile, error)
	ListProfiles(ctx context.Context) ([]RelationalProfile, error)

	Close() error
}

// ---------------------------------------------------------------------------
// Phase 4 — Tools & CLI
// ---------------------------------------------------------------------------

// ToolHandler is the contract every tool implementation must satisfy.
// Name returns the canonical snake_case tool name as registered.
// Execute receives parsed args and returns a human-readable result string.
type ToolHandler interface {
	Name() string
	Execute(ctx context.Context, args map[string]any) (string, error)
}

// ToolHandlerV2 is the MOP-era tool interface returning structured ToolResult.
// The engine loop checks for V2 first, falls back to V1.
type ToolHandlerV2 interface {
	ToolHandler
	ExecuteV2(ctx context.Context, args map[string]any) (*ToolResult, error)
}

// ToolGroup is a logical grouping of tools sharing a tier and keyword set.
type ToolGroup struct {
	Name     string
	Tier     int
	Keywords []string
	Tools    []ToolDef
}

// ToolRegistry manages the set of available tools for a session.
type ToolRegistry interface {
	Register(h ToolHandler)
	Get(name string) (ToolHandler, bool)
	GetMeta(name string) (ToolMeta, bool)
	List() []ToolGroup
}

// ---------------------------------------------------------------------------
// Phase 5 — Channels & Aegis
// ---------------------------------------------------------------------------

// AegisAnnotation is the trust report attached to content after Aegis processing.
type AegisAnnotation struct {
	TrustScore    float64  // 0.0–1.0, skeptical prior 0.40 for new sources
	Source        string   // domain or channel identifier
	ContentSource string   // "discord", "forum-content", "browser-content", etc.
	Flags         []string // injection pattern matches (informational, never blocking)
	Sanitized     bool     // true if normalization modified the content
	ScanPassed    bool     // true if no injection patterns detected
	AnnotatedAt   time.Time
}

// AnnotatedContent is the output of the Aegis gateway.
type AnnotatedContent struct {
	Original   []byte // raw bytes as received from channel
	Normalized string // after NFKC + invisible char stripping
	Annotation AegisAnnotation
}

// OutboundReport is the result of outbound leak scanning.
type OutboundReport struct {
	Clean    bool
	Findings []string // descriptions of detected leaks (API keys, paths, etc.)
}

// ErrTrustNotFound is the sentinel TrustStore implementations must return
// (wrapped) when GetTrust finds no record for a source.
var ErrTrustNotFound = errors.New("trust: source not found")

// TrustStore persists trust scores across sessions.
type TrustStore interface {
	GetTrust(ctx context.Context, source string) (score float64, interactions int, err error)
	UpdateTrust(ctx context.Context, source string, score float64) error
	IncrementInteractions(ctx context.Context, source string) error
}

// ContentGateway is the Aegis pipeline interface.
type ContentGateway interface {
	ProcessInbound(ctx context.Context, raw []byte, source, contentSource string) (*AnnotatedContent, error)
	ReviewOutbound(ctx context.Context, content string) (*OutboundReport, error)
}

// ---------------------------------------------------------------------------
// Sprint 3E — Repository Ports (HARN-72)
//
// Domain packages say "commit job" and "record disclosure," not
// "use $1 when driver is postgres." Implementations live in
// internal/platform/storage/.
// ---------------------------------------------------------------------------

// MetabolismJobStore manages durable metabolism job records.
// The job record is the source of durability — it must be committed
// before dispatch and completed/failed after processing.
type MetabolismJobStore interface {
	// Commit inserts a new pending job. Must be called and committed
	// BEFORE dispatching the processing goroutine. Returns the job ID.
	Commit(ctx context.Context, sessionID, jobType string) (jobID string, err error)

	// Claim atomically transitions a pending job to running.
	// Returns ErrJobNotPending if already claimed.
	Claim(ctx context.Context, jobID string) error

	// Complete marks a job as successfully finished.
	Complete(ctx context.Context, jobID string) error

	// Fail marks a job as failed with an error message and increments retry count.
	Fail(ctx context.Context, jobID string, cause string) error

	// MarkReviewRequired transitions a job to review_required status.
	// Used when external content lacks Aegis annotation and cannot be
	// compressed automatically. The job is not retried — it awaits review.
	MarkReviewRequired(ctx context.Context, jobID string, reason string) error

	// Recoverable returns IDs and session IDs of jobs in pending or running
	// state that haven't exceeded maxRetries. Recovery resubmits these
	// through the normal Claim → process → Complete/Fail path.
	Recoverable(ctx context.Context, maxRetries int) ([]RecoverableJob, error)

	// LastStatus returns the most recent job's status, or "" if no jobs exist.
	// Used for operational state input to the lifecycle resolver.
	LastStatus(ctx context.Context) (string, error)
}

// RecoverableJob is the minimal info needed to resubmit a stalled job.
type RecoverableJob struct {
	JobID      string
	SessionID  string
	RetryCount int
}

// ConsolidationStore handles the atomic T2→T3 link: inserting a narrative
// summary and updating the source experiential logs in one transaction.
type ConsolidationStore interface {
	// CommitNarrative atomically inserts the T3 narrative and updates all
	// source T2 log rows with the narrative's ID. The embedding vector
	// must be included in the narrative for retrieval to work.
	CommitNarrative(ctx context.Context, narrative *NarrativeSummary, sourceLogIDs []string) error

	// UncoveredLogs returns T2 experiential logs for a session that have
	// no narrative_summary_id — the inputs for consolidation.
	UncoveredLogs(ctx context.Context, sessionID string) ([]ExperientialLog, error)
}

// LifecycleStore persists the lifecycle artifacts that connect sessions
// into a continuous history: plans, wake facts, assembly manifests,
// configuration disclosures, and checkpoints.
type LifecycleStore interface {
	// RecordPlan persists a resolved lifecycle plan before assembly.
	RecordPlan(ctx context.Context, plan *LifecyclePlan) error

	// RecordWakeFacts persists the observed facts about a wake event.
	RecordWakeFacts(ctx context.Context, sessionID string, facts *WakeFacts) error

	// RecordManifest persists what assembly loaded, skipped, and why.
	RecordManifest(ctx context.Context, manifest *AssemblyManifest) error

	// RecordDisclosure persists an applied configuration change with
	// the policy path, hash, previous hash, and a summary of changes.
	RecordDisclosure(ctx context.Context, sessionID, policyPath, policyHash, previousHash, changesSummary string) error

	// LastPolicyHash returns the most recently applied policy hash,
	// or "" if no configuration has been applied.
	LastPolicyHash(ctx context.Context, agentID string) (string, error)

	// WriteCheckpoint upserts a session checkpoint (turn progress, token usage).
	WriteCheckpoint(ctx context.Context, sessionID string, turnNumber int, t2HighWater string, tokenUsage int, state string) error

	// InterruptStaleCheckpoints marks any active checkpoints older than
	// the cutoff as interrupted. Returns the count of interrupted sessions.
	InterruptStaleCheckpoints(ctx context.Context, cutoff time.Time) (int, error)

	// LastCheckpointState returns the most recent checkpoint's state,
	// or "" if none exist. Used for operational state input.
	LastCheckpointState(ctx context.Context) (string, error)

	// LastWakeAt returns the timestamp of the most recent wake, or the
	// zero time if no wakes are recorded.
	LastWakeAt(ctx context.Context) (time.Time, error)
}

// AssemblyStore handles identity persistence operations used during
// context assembly: witness letter verification and operator action logging.
type AssemblyStore interface {
	// HasWitnessLetter returns true if a founding record of type
	// 'witness_letter' exists.
	HasWitnessLetter(ctx context.Context) (bool, error)

	// LogOperatorAction records a system action in the operator_actions
	// audit trail.
	LogOperatorAction(ctx context.Context, actionType, description string) error
}

// ErrJobNotPending is returned by MetabolismJobStore.Claim when the job
// is not in pending state (already claimed or completed).
var ErrJobNotPending = errors.New("job is not in pending state")
