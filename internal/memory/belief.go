package memory

import (
	"context"
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
		InferenceDecayBase: 0.90,
		StaleThreshold:     0.3,
	}
}

// ComputeInferenceDistance follows derived_from edges in memory_edges to find
// the minimum number of hops from this record to a T2 (direct experience) source.
// T2 entries have distance 0. A record with no edges defaults to distance 1
// (live-session default for un-cited claims).
func ComputeInferenceDistance(ctx context.Context, store pkg.BeliefStore, recordID string) (int, error) {
	// BFS: find all nodes reachable via derived_from edges, tracking depth.
	visited := map[string]bool{recordID: true}
	queue := []string{recordID}
	depth := 0

	for len(queue) > 0 {
		var next []string
		for _, id := range queue {
			nodes, err := store.QueryEdgesForBFS(ctx, id)
			if err != nil {
				return 0, fmt.Errorf("belief: querying edges for %s: %w", id, err)
			}

			for _, n := range nodes {
				if n.Tier == 2 {
					// Reached a T2 entry — distance is hops taken so far + 1.
					return depth + 1, nil
				}
				if !visited[n.ID] {
					visited[n.ID] = true
					next = append(next, n.ID)
				}
			}
		}
		queue = next
		depth++
	}

	// No T2 source found — return 1 as the live-session default.
	return 1, nil
}

// DecayReport summarises the outcome of a single end-of-session decay pass.
type DecayReport struct {
	TotalChecked        int
	NewlyStale          int
	ByInferenceDistance map[int]int // inference distance → count of stale records
	ReVerified          int         // records whose state moved from stale to verified this session
}

// RunEndOfSessionDecay is a wrapper around FlagStaleBeliefs from retrieve.go.
// It loads all T3/T4/T5 records with belief_meta, runs the staleness pass,
// and returns a DecayReport. Idempotent.
func RunEndOfSessionDecay(ctx context.Context, store pkg.BeliefStore, now time.Time, cfg DecayConfig) (*DecayReport, error) {
	records, err := store.LoadBeliefRecords(ctx)
	if err != nil {
		return nil, fmt.Errorf("belief: loading records for decay pass: %w", err)
	}

	report := &DecayReport{
		TotalChecked:        len(records),
		ByInferenceDistance: make(map[int]int),
	}

	updateFn := func(ctx context.Context, table, id, state string) error {
		return store.UpdateVerificationState(ctx, table, id, state)
	}

	count, err := FlagStaleBeliefs(ctx, records, now, cfg.DecayRate, cfg.InferenceDecayBase, cfg.StaleThreshold, updateFn)
	if err != nil {
		return report, fmt.Errorf("belief: decay pass failed: %w", err)
	}
	report.NewlyStale = count

	// Compute per-inference-distance breakdown for records that became stale this pass.
	// FlagStaleBeliefs skips records already stale, so any non-stale record below
	// threshold was newly flagged.
	for _, rec := range records {
		if rec.Belief == nil || rec.Belief.VerificationState == "stale" {
			continue
		}
		conf := rec.Belief.Confidence(now, cfg.DecayRate, cfg.InferenceDecayBase)
		if conf >= cfg.StaleThreshold {
			continue
		}
		dist, distErr := ComputeInferenceDistance(ctx, store, rec.ID)
		if distErr != nil {
			dist = -1
		}
		report.ByInferenceDistance[dist]++
	}

	slog.Info("belief: end-of-session decay pass complete",
		"flagged", count, "total_checked", len(records))
	return report, nil
}
