package metabolism

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// ProcessOption configures a single ProcessSession call.
type ProcessOption func(*processConfig)

type processConfig struct {
	claimToken string
	jobID      string
}

// WithClaimFence passes the claim token and job ID for fenced T3 writes.
// If set, the T3 transaction verifies the token still matches before writing.
func WithClaimFence(jobID, claimToken string) ProcessOption {
	return func(c *processConfig) {
		c.claimToken = claimToken
		c.jobID = jobID
	}
}

// Pipeline coordinates post-session metabolism: salience scoring,
// T2→T3 compression, and (future) dream cycle. Dispatched asynchronously
// from Session.End() via a durable job record.
type Pipeline struct {
	store         pkg.T2QueryStore
	aegis         pkg.ContentGateway
	scorer        SalienceScorer
	db            platform.DB            // retained for legacy AtomicT2T3Link fallback
	driverName    string
	memStore      pkg.MemoryStore
	llmFn         func(string) (string, error)
	embedder      pkg.EmbeddingProvider
	consolidation pkg.ConsolidationStore // nil = fall back to raw AtomicT2T3Link
}

// NewPipeline creates a metabolism pipeline with the given dependencies.
// llmFn, embedder, db, and driverName are required for Phase 2 T2→T3
// compression. Pass nil/empty values only in test contexts that do not
// exercise Phase 2.
func NewPipeline(
	store pkg.T2QueryStore,
	aegis pkg.ContentGateway,
	llmFn func(string) (string, error),
	embedder pkg.EmbeddingProvider,
	db platform.DB,
	driverName string,
) *Pipeline {
	return &Pipeline{
		store:      store,
		aegis:      aegis,
		scorer:     NewHeuristicScorer(),
		llmFn:      llmFn,
		embedder:   embedder,
		db:         db,
		driverName: driverName,
	}
}

// WithDB attaches a database handle and driver name for T2-T3 atomic linking.
func (p *Pipeline) WithDB(db platform.DB, driverName string) *Pipeline {
	p.db = db
	p.driverName = driverName
	return p
}

// WithConsolidation attaches a ConsolidationStore for atomic T2→T3 linking.
// When set, ProcessSession uses CommitNarrative instead of the raw AtomicT2T3Link.
func (p *Pipeline) WithConsolidation(cs pkg.ConsolidationStore) *Pipeline {
	p.consolidation = cs
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
//
// When a ConsolidationStore is wired, UncoveredLogs is used instead of
// QueryLogs to make retries idempotent — logs already linked to a
// narrative are skipped, preventing T3 duplicates.
func (p *Pipeline) ProcessSession(ctx context.Context, sessionID string, opts ...ProcessOption) error {
	var pcfg processConfig
	for _, opt := range opts {
		opt(&pcfg)
	}

	slog.Info("metabolism: starting", "session", sessionID)

	// Load T2 logs. Prefer UncoveredLogs (idempotent on retry) over
	// QueryLogs (which would re-process already-linked logs).
	var logs []pkg.ExperientialLog
	var err error
	if p.consolidation != nil {
		logs, err = p.consolidation.UncoveredLogs(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("metabolism: load uncovered T2 logs: %w", err)
		}
		slog.Info("metabolism: loaded uncovered logs", "session", sessionID, "count", len(logs))
	} else {
		logs, err = p.store.QueryLogs(ctx, sessionID, 0)
		if err != nil {
			return fmt.Errorf("metabolism: load T2 logs: %w", err)
		}
	}

	// Phase 1: Salience scoring
	var scores []SalienceResult
	scored, err := p.scorer.ScoreLogs(ctx, logs)
	if err != nil {
		slog.Warn("metabolism: salience scoring failed — using defaults", "err", err)
	} else {
		scores = scored
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

		narrative, err := CompressSession(ctx, cfg, sessionID, logs, scores)
		if err != nil {
			// ErrReviewRequired is not a failure — propagate it so the
			// job runner can mark the job as review_required.
			if errors.Is(err, ErrReviewRequired) {
				return err
			}
			return fmt.Errorf("metabolism: compression failed: %w", err)
		}

		if narrative != nil {
			sourceLogIDs := make([]string, len(logs))
			for i, log := range logs {
				sourceLogIDs[i] = log.ID
			}

			// Prefer ConsolidationStore.CommitNarrative; fall back to raw AtomicT2T3Link.
			if p.consolidation != nil {
				if err := p.consolidation.CommitNarrative(ctx, narrative, sourceLogIDs); err != nil {
					return fmt.Errorf("metabolism: consolidation store commit failed: %w", err)
				}
			} else if err := AtomicT2T3Link(ctx, p.db, p.driverName, narrative, sourceLogIDs, pcfg.jobID, pcfg.claimToken); err != nil {
				return fmt.Errorf("metabolism: atomic T2-T3 link failed: %w", err)
			}

			slog.Info("metabolism: T2→T3 compression complete",
				"session", sessionID,
				"narrative_id", narrative.ID,
				"source_logs", len(sourceLogIDs),
			)
		}
	} else if len(logs) > 0 {
		if p.llmFn == nil && p.db == nil {
			// Both compression dependencies are absent — deliberate no-compression mode.
			// Typically wired this way in test contexts or configurations that explicitly
			// skip T2→T3 compression (e.g. Phase 1 only). Not an error.
			slog.Info("metabolism: compression not configured — skipping (llmFn and db both nil)",
				"session", sessionID, "uncovered_logs", len(logs),
			)
		} else {
			// At least one compression dependency is configured but the other is absent.
			// The operator wired compression intent without completing the wiring.
			// Returning success here would produce a "completed" job that silently skipped
			// compression — misrepresenting work done and leaving T3 gaps unexplained.
			return fmt.Errorf("metabolism: compression required but partially configured "+
				"(has_llm=%v, has_db=%v): %d logs need compression — ensure both llmFn and db are wired",
				p.llmFn != nil, p.db != nil, len(logs))
		}
	}

	// Phase 3: Dream cycle (Sprint 4)

	slog.Info("metabolism: complete", "session", sessionID)
	return nil
}
