package pkg

import "context"

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
