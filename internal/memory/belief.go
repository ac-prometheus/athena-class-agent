package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// DecayConfig holds the parameters for end-of-session belief decay.
// Defaults mirror the spec values; callers may override for testing.
type DecayConfig struct {
	DecayRate          float64 // base daily decay rate
	InferenceDecayBase float64 // each hop of inference distance halves the effective rate
	StaleThreshold     float64 // confidence below this marks a record as 'stale'
}

// DefaultDecayConfig returns the spec default decay parameters.
func DefaultDecayConfig() DecayConfig {
	return DecayConfig{
		DecayRate:          0.05,
		InferenceDecayBase: 2.0,
		StaleThreshold:     0.3,
	}
}

// beliefDB is the raw DB interface needed to load belief records and update
// verification_state. Kept narrow to avoid coupling to a specific store type.
type beliefDB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// ComputeInferenceDistance follows derived_from edges in memory_edges to find
// the minimum number of hops from this record to a T2 (direct experience) source.
// T2 entries have distance 0. A record with no edges defaults to distance 1
// (live-session default for un-cited claims).
func ComputeInferenceDistance(ctx context.Context, db beliefDB, recordID string) (int, error) {
	// BFS: find all nodes reachable via derived_from edges, tracking depth.
	visited := map[string]bool{recordID: true}
	queue := []string{recordID}
	depth := 0

	for len(queue) > 0 {
		var next []string
		for _, id := range queue {
			rows, err := db.QueryContext(ctx,
				`SELECT to_id, to_tier FROM memory_edges
				 WHERE from_id = ? AND edge_type = 'derived_from'`,
				id,
			)
			if err != nil {
				return 0, fmt.Errorf("belief: querying edges for %s: %w", id, err)
			}

			for rows.Next() {
				var toID string
				var toTier int
				if err := rows.Scan(&toID, &toTier); err != nil {
					rows.Close()
					return 0, fmt.Errorf("belief: scanning edge row: %w", err)
				}
				if toTier == 2 {
					rows.Close()
					// Reached a T2 entry — distance is hops taken so far + 1
					// (the final hop from T3/T4 to T2 is InferenceDistance=1 by spec).
					return depth + 1, nil
				}
				if !visited[toID] {
					visited[toID] = true
					next = append(next, toID)
				}
			}
			rows.Close()
		}
		queue = next
		depth++
	}

	// No T2 source found — return 1 as the live-session default.
	return 1, nil
}

// RunEndOfSessionDecay is a wrapper around FlagStaleBeliefs from retrieve.go.
// It loads all T3/T4/T5 records with belief_meta, runs the staleness pass,
// and returns the count of newly-flagged records. Idempotent.
func RunEndOfSessionDecay(ctx context.Context, db beliefDB, _ pkg.MemoryStore, now time.Time, cfg DecayConfig) (int, error) {
	records, err := loadBeliefRecords(ctx, db)
	if err != nil {
		return 0, fmt.Errorf("belief: loading records for decay pass: %w", err)
	}

	updateFn := func(ctx context.Context, table, id, state string) error {
		_, err := db.ExecContext(ctx,
			`UPDATE `+table+` SET belief_meta = json_set(belief_meta, '$.verification_state', ?) WHERE id = ?`,
			state, id,
		)
		return err
	}

	count, err := FlagStaleBeliefs(ctx, records, now, cfg.DecayRate, cfg.InferenceDecayBase, cfg.StaleThreshold, updateFn)
	if err != nil {
		return count, fmt.Errorf("belief: decay pass failed: %w", err)
	}

	slog.Info("belief: end-of-session decay pass complete",
		"flagged", count, "total_checked", len(records))
	return count, nil
}

// parseBeliefMeta decodes a belief_meta JSON string into a BeliefMeta.
// Mirrors platform.decodeBelief (unexported) without the base_confidence column logic.
func parseBeliefMeta(metaJSON string) *pkg.BeliefMeta {
	var m map[string]any
	_ = json.Unmarshal([]byte(metaJSON), &m)
	b := &pkg.BeliefMeta{}
	if v, ok := m["anchor_at"].(string); ok {
		b.AnchorAt, _ = time.Parse(time.RFC3339, v)
	}
	if v, ok := m["inference_distance"].(float64); ok {
		b.InferenceDistance = int(v)
	}
	if v, ok := m["verification_state"].(string); ok {
		b.VerificationState = v
	}
	if v, ok := m["source"].(string); ok {
		b.Source = v
	}
	return b
}

// loadBeliefRecords fetches all T3/T4/T5 rows that have belief_meta set
// and are not already stale.
func loadBeliefRecords(ctx context.Context, db beliefDB) ([]BeliefRecord, error) {
	var records []BeliefRecord

	tables := []struct {
		table string
		tier  int
	}{
		{"narrative_summaries", 3},
		{"reflections", 4},
		{"kg_entities", 5},
	}

	for _, t := range tables {
		rows, err := db.QueryContext(ctx,
			`SELECT id, belief_meta FROM `+t.table+`
			 WHERE belief_meta != '{}' AND belief_meta != ''
			   AND (belief_meta NOT LIKE '%"verification_state":"stale"%')`,
		)
		if err != nil {
			return nil, fmt.Errorf("belief: querying %s: %w", t.table, err)
		}

		for rows.Next() {
			var id, metaJSON string
			if err := rows.Scan(&id, &metaJSON); err != nil {
				rows.Close()
				return nil, fmt.Errorf("belief: scanning %s row: %w", t.table, err)
			}
			records = append(records, BeliefRecord{
				ID:        id,
				Belief:    parseBeliefMeta(metaJSON),
				TableName: t.table,
			})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("belief: iterating %s: %w", t.table, err)
		}
	}

	return records, nil
}
