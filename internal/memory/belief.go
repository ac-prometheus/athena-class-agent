package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
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
	QueryContext(ctx context.Context, query string, args ...any) (platform.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// ComputeInferenceDistance follows derived_from edges in memory_edges to find
// the minimum number of hops from this record to a T2 (direct experience) source.
// T2 entries have distance 0. A record with no edges defaults to distance 1
// (live-session default for un-cited claims).
// driverName must be "sqlite3" or "postgres".
func ComputeInferenceDistance(ctx context.Context, db beliefDB, driverName, recordID string) (int, error) {
	// BFS: find all nodes reachable via derived_from edges, tracking depth.
	visited := map[string]bool{recordID: true}
	queue := []string{recordID}
	depth := 0

	for len(queue) > 0 {
		var next []string
		for _, id := range queue {
			rows, err := db.QueryContext(ctx,
				`SELECT to_id, to_tier FROM memory_edges
				 WHERE from_id = `+placeholder(driverName, 1)+` AND edge_type = 'derived_from'`,
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
// driverName must be "sqlite3" or "postgres".
func RunEndOfSessionDecay(ctx context.Context, db beliefDB, driverName string, _ pkg.MemoryStore, now time.Time, cfg DecayConfig) (int, error) {
	records, err := loadBeliefRecords(ctx, db, driverName)
	if err != nil {
		return 0, fmt.Errorf("belief: loading records for decay pass: %w", err)
	}

	updateFn := func(ctx context.Context, table, id, state string) error {
		_, err := db.ExecContext(ctx,
			`UPDATE `+table+` SET belief_meta = `+jsonUpdate(driverName, "belief_meta", "$.verification_state", placeholder(driverName, 1))+
				` WHERE id = `+placeholder(driverName, 2),
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
// driverName must be "sqlite3" or "postgres".
func loadBeliefRecords(ctx context.Context, db beliefDB, driverName string) ([]BeliefRecord, error) {
	var records []BeliefRecord

	// jsonbTextExpr returns an expression that produces a TEXT value from a JSONB
	// column — needed for LIKE comparisons on Postgres where JSONB does not
	// support LIKE directly.
	jsonbText := func(col string) string {
		if driverName == "postgres" {
			return col + "::text"
		}
		return col
	}

	// T3 and T5: belief_meta JSONB contains BaseConfidence (system-set).
	// SELECT returns belief_meta as text for consistent Scan into string.
	for _, t := range []struct {
		table string
		tier  int
	}{
		{"narrative_summaries", 3},
		{"kg_entities", 5},
	} {
		rows, err := db.QueryContext(ctx,
			`SELECT id, `+jsonbText("belief_meta")+` FROM `+t.table+`
			 WHERE `+jsonbText("belief_meta")+` != '{}' AND `+jsonbText("belief_meta")+` != ''
			   AND `+jsonbText("belief_meta")+` NOT LIKE '%"verification_state":"stale"%'`,
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

	// T4: base_confidence lives in its own column (consent boundary).
	// If NULL (agent hasn't set it), skip — can't compute staleness without an anchor.
	// If set, use it. The system never writes this value, only reads it here.
	{
		rows, err := db.QueryContext(ctx,
			`SELECT id, `+jsonbText("belief_meta")+`, base_confidence FROM reflections
			 WHERE base_confidence IS NOT NULL
			   AND `+jsonbText("belief_meta")+` != '{}' AND `+jsonbText("belief_meta")+` != ''
			   AND `+jsonbText("belief_meta")+` NOT LIKE '%"verification_state":"stale"%'`,
		)
		if err != nil {
			return nil, fmt.Errorf("belief: querying reflections: %w", err)
		}

		for rows.Next() {
			var id, metaJSON string
			var baseConf float64
			if err := rows.Scan(&id, &metaJSON, &baseConf); err != nil {
				rows.Close()
				return nil, fmt.Errorf("belief: scanning reflections row: %w", err)
			}
			belief := parseBeliefMeta(metaJSON)
			belief.BaseConfidence = baseConf
			records = append(records, BeliefRecord{
				ID:        id,
				Belief:    belief,
				TableName: "reflections",
			})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("belief: iterating reflections: %w", err)
		}
	}

	return records, nil
}
