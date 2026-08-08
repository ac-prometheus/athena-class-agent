package metabolism

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// Pipeline coordinates post-session metabolism: salience scoring,
// T2→T3 compression, and (future) dream cycle. Dispatched asynchronously
// from Session.End() via a durable job record.
type Pipeline struct {
	store      pkg.T2QueryStore
	aegis      pkg.ContentGateway
	scorer     SalienceScorer
	db         platform.DB
	driverName string
	memStore   pkg.MemoryStore
	llmFn      func(string) (string, error)
	embedder   pkg.EmbeddingProvider
}

// NewPipeline creates a metabolism pipeline with the given dependencies.
func NewPipeline(store pkg.T2QueryStore, aegis pkg.ContentGateway) *Pipeline {
	return &Pipeline{
		store:  store,
		aegis:  aegis,
		scorer: NewHeuristicScorer(),
	}
}

// WithDB attaches a database handle and driver name for T2-T3 atomic linking.
func (p *Pipeline) WithDB(db platform.DB, driverName string) *Pipeline {
	p.db = db
	p.driverName = driverName
	return p
}

// WithCompression attaches the dependencies needed for T2→T3 compression.
func (p *Pipeline) WithCompression(memStore pkg.MemoryStore, llmFn func(string) (string, error), embedder pkg.EmbeddingProvider) *Pipeline {
	p.memStore = memStore
	p.llmFn = llmFn
	p.embedder = embedder
	return p
}

// ProcessSession runs the metabolism pipeline for a completed session.
// Phase 1: Score salience on all T2 logs.
// Phase 2: Aegis-gated T2→T3 compression (separate file).
// Phase 3: Dream cycle (Sprint 4 — not implemented here).
func (p *Pipeline) ProcessSession(ctx context.Context, sessionID string) error {
	slog.Info("metabolism: starting", "session", sessionID)

	// Phase 1: Salience scoring
	// QueryLogs with limit=0 returns all logs for the session.
	logs, err := p.store.QueryLogs(ctx, sessionID, 0)
	if err != nil {
		return fmt.Errorf("metabolism: load T2 logs: %w", err)
	}

	scores, err := p.scorer.ScoreLogs(ctx, logs)
	if err != nil {
		slog.Warn("metabolism: salience scoring failed — using defaults", "err", err)
	} else {
		// ExperientialLog has no SalienceScore field — log the scored results
		// for observability. Compression (Phase 2) will consume them directly
		// from the SalienceResult slice rather than via a field on the log.
		for i, score := range scores {
			if i < len(logs) {
				slog.Debug("metabolism: salience score",
					"log_id", logs[i].ID,
					"score", score.Score,
					"compression_resist", score.CompressionResist,
					"reason", score.Reason,
				)
			}
		}
		slog.Info("metabolism: salience scored", "session", sessionID, "logs", len(logs))
	}

	// Phase 2: T2→T3 compression
	if p.llmFn != nil && p.db != nil && len(logs) > 0 {
		cfg := CompressConfig{
			Store:    p.memStore,
			Aegis:    p.aegis,
			LLMFn:    p.llmFn,
			Embedder: p.embedder,
		}

		narrative, err := CompressSession(ctx, cfg, sessionID, logs)
		if err != nil {
			return fmt.Errorf("metabolism: compression failed: %w", err)
		}

		if narrative != nil {
			// Collect source log IDs for the atomic back-link.
			sourceLogIDs := make([]string, len(logs))
			for i, log := range logs {
				sourceLogIDs[i] = log.ID
			}

			if err := AtomicT2T3Link(ctx, p.db, p.driverName, narrative, sourceLogIDs); err != nil {
				return fmt.Errorf("metabolism: atomic T2-T3 link failed: %w", err)
			}

			slog.Info("metabolism: T2→T3 compression complete",
				"session", sessionID,
				"narrative_id", narrative.ID,
				"source_logs", len(sourceLogIDs),
			)
		}
	} else if len(logs) > 0 {
		slog.Info("metabolism: skipping compression — dependencies not configured",
			"session", sessionID,
			"has_llm", p.llmFn != nil,
			"has_db", p.db != nil,
		)
	}

	// Phase 3: Dream cycle (Sprint 4)

	slog.Info("metabolism: complete", "session", sessionID)
	return nil
}
