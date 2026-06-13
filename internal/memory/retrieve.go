package memory

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// ---------------------------------------------------------------------------
// Composite retrieval scoring
// ---------------------------------------------------------------------------

// RetrievalWeights holds the three scoring weights for composite retrieval.
// All weights must be in [0.1, 0.8] and sum to 1.0 (enforced by the
// settings registry; defaults from agent_settings table in schema 006).
type RetrievalWeights struct {
	Recency    float64 // default 0.35
	Relevance  float64 // default 0.45
	Importance float64 // default 0.20
}

// DefaultWeights returns the default retrieval weights from the spec.
func DefaultWeights() RetrievalWeights {
	return RetrievalWeights{
		Recency:    0.35,
		Relevance:  0.45,
		Importance: 0.20,
	}
}

// CompositeScore computes the composite ranking score for a candidate record.
// All three inputs must be normalized to [0, 1] before calling.
//
// Formula: w_recency * recency + w_relevance * relevance + w_importance * importance
// Weights are agent-adjustable via the settings registry (bounds [0.1, 0.8],
// sum-to-1 enforced). An agent prioritizing recent experience over abstract
// importance shifts w_recency up and w_importance down.
func CompositeScore(recency, relevance, importance float64, weights RetrievalWeights) float64 {
	return weights.Recency*recency + weights.Relevance*relevance + weights.Importance*importance
}

// RecencyScore returns an exponentially-decayed recency score in [0, 1].
// anchorAt is the record's anchor timestamp; windowDays is the half-life window
// over which recency decays to ~0 (default: 30 days).
// Score = exp(-(days_elapsed / windowDays) * ln(2))
// At 0 days: 1.0. At windowDays: 0.5. At 2*windowDays: ~0.25.
func RecencyScore(anchorAt time.Time, now time.Time, windowDays float64) float64 {
	if windowDays <= 0 {
		windowDays = 30
	}
	elapsed := now.Sub(anchorAt).Hours() / 24
	if elapsed < 0 {
		return 1.0
	}
	return math.Exp(-elapsed / windowDays * math.Ln2)
}

// ---------------------------------------------------------------------------
// Hybrid search with Reciprocal Rank Fusion
// ---------------------------------------------------------------------------

// RankedHit extends pkg.Hit with per-source rank information for RRF.
type RankedHit struct {
	pkg.Hit
	FTSRank    int // rank in FTS result set (1-indexed, 0 = not in FTS results)
	VectorRank int // rank in vector result set (1-indexed, 0 = not in vector results)
}

// RRFScore computes the Reciprocal Rank Fusion score for a hit.
// Standard RRF formula: 1/(k + rank) for each list, summed.
// k=60 is the standard smoothing constant (Cormack et al., 2009).
func RRFScore(ftsRank, vectorRank int, k float64) float64 {
	if k <= 0 {
		k = 60
	}
	score := 0.0
	if ftsRank > 0 {
		score += 1.0 / (k + float64(ftsRank))
	}
	if vectorRank > 0 {
		score += 1.0 / (k + float64(vectorRank))
	}
	return score
}

// HybridSearch combines FTS and vector results using Reciprocal Rank Fusion.
// ftsHits and vectorHits are ordered result sets from their respective searches.
// The combined, deduplicated, RRF-scored list is returned, limited to topK.
//
// Pure cosine similarity is weak on names, dates, and exact phrases — exactly
// what a persistent agent searches most. RRF fusion recovers this without
// requiring a learned ranking model.
func HybridSearch(ctx context.Context, ftsHits, vectorHits []pkg.Hit, topK int) []pkg.Hit {
	// Build rank maps: ID → 1-indexed rank in each list.
	ftsRankOf := make(map[string]int, len(ftsHits))
	for i, h := range ftsHits {
		ftsRankOf[h.ID] = i + 1
	}
	vecRankOf := make(map[string]int, len(vectorHits))
	for i, h := range vectorHits {
		vecRankOf[h.ID] = i + 1
	}

	// Collect all unique IDs.
	seen := make(map[string]bool)
	var allIDs []string
	for _, h := range ftsHits {
		if !seen[h.ID] {
			allIDs = append(allIDs, h.ID)
			seen[h.ID] = true
		}
	}
	for _, h := range vectorHits {
		if !seen[h.ID] {
			allIDs = append(allIDs, h.ID)
			seen[h.ID] = true
		}
	}

	// Compute RRF scores.
	type scored struct {
		id    string
		score float64
		meta  map[string]any
	}
	results := make([]scored, 0, len(allIDs))

	// Build a meta lookup from whichever list has the entry.
	metaOf := make(map[string]map[string]any)
	for _, h := range ftsHits {
		metaOf[h.ID] = h.Meta
	}
	for _, h := range vectorHits {
		if _, ok := metaOf[h.ID]; !ok {
			metaOf[h.ID] = h.Meta
		}
	}

	for _, id := range allIDs {
		rrf := RRFScore(ftsRankOf[id], vecRankOf[id], 60)
		results = append(results, scored{id: id, score: rrf, meta: metaOf[id]})
	}

	// Sort descending by RRF score.
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	// Trim to topK and convert back to pkg.Hit.
	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}
	out := make([]pkg.Hit, len(results))
	for i, r := range results {
		out[i] = pkg.Hit{ID: r.id, Score: r.score, Meta: r.meta}
	}
	return out
}

// ---------------------------------------------------------------------------
// Staleness flagging
// ---------------------------------------------------------------------------

// BeliefRecord is a minimal struct for the staleness-flagging pass.
// Only the fields needed by FlagStaleBeliefs are included.
type BeliefRecord struct {
	ID         string
	Belief     *pkg.BeliefMeta
	TableName  string
}

// FlagStaleBeliefs runs the end-of-session staleness pass over T3/T4/T5 records.
// For each record, it computes Confidence(now) and flags as 'stale' if below
// staleThreshold — but only updates verification_state, never base_confidence.
// This is idempotent and safe to run multiple times.
//
// T4 CONSENT BOUNDARY: base_confidence on T4 rows is never modified here.
// Only verification_state is updated (time-based staleness is a mechanical fact).
func FlagStaleBeliefs(
	ctx context.Context,
	records []BeliefRecord,
	now time.Time,
	decayRate, inferenceDecayBase, staleThreshold float64,
	updateFn func(ctx context.Context, table, id, state string) error,
) (int, error) {
	flagged := 0
	for _, rec := range records {
		if rec.Belief == nil || rec.Belief.VerificationState == "stale" {
			continue
		}
		conf := rec.Belief.Confidence(now, decayRate, inferenceDecayBase)
		if conf < staleThreshold {
			if err := updateFn(ctx, rec.TableName, rec.ID, "stale"); err != nil {
				return flagged, err
			}
			flagged++
		}
	}
	return flagged, nil
}
