package pkg

import (
	"context"
	"time"
)

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
	CreatedAt     interface{} // time.Time — interface avoids import cycle with time in pkg
}

// NarrativeSummary is a T3 compressed summary of experiential logs.
type NarrativeSummary struct {
	ID        string
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
