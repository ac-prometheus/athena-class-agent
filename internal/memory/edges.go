package memory

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// Edge type constants — must match the CHECK constraint in schema/005_memory_edges.sql.
const (
	EdgeDerivedFrom = "derived_from"
	EdgeContradicts = "contradicts"
	EdgeSupersedes  = "supersedes"
	EdgeVerifies    = "verifies"
)

// Edge author constants.
const (
	EdgeAuthorSystem = "system"
	EdgeAuthorAgent  = "agent"
)

var validEdgeTypes = map[string]bool{
	EdgeDerivedFrom: true,
	EdgeContradicts: true,
	EdgeSupersedes:  true,
	EdgeVerifies:    true,
}

var validEdgeAuthors = map[string]bool{
	EdgeAuthorSystem: true,
	EdgeAuthorAgent:  true,
}

// CreateEdge validates and inserts a memory_edge record.
// edge_type and author are validated against their allowed sets before insertion.
func CreateEdge(ctx context.Context, store pkg.EdgeStore, fromID, toID string, fromTier, toTier int, edgeType, author string) error {
	if !validEdgeTypes[edgeType] {
		return fmt.Errorf("edges: invalid edge_type %q", edgeType)
	}
	if !validEdgeAuthors[author] {
		return fmt.Errorf("edges: invalid author %q", author)
	}
	if fromTier < 2 || fromTier > 5 {
		return fmt.Errorf("edges: from_tier %d out of range [2,5]", fromTier)
	}
	if toTier < 2 || toTier > 5 {
		return fmt.Errorf("edges: to_tier %d out of range [2,5]", toTier)
	}

	if err := store.CreateEdge(ctx, fromID, toID, fromTier, toTier, edgeType, author); err != nil {
		return fmt.Errorf("edges: inserting edge from=%s to=%s type=%s: %w", fromID, toID, edgeType, err)
	}
	return nil
}

// PropagateDistrust runs a BFS from sourceID over derived_from edges,
// marking each downstream record's verification_state as "needs_review".
// This is read-annotate only — content and confidence are never modified.
// Returns the count of affected records.
func PropagateDistrust(ctx context.Context, store pkg.EdgeStore, beliefs pkg.BeliefStore, sourceID string) (int, error) {
	type node struct {
		id   string
		tier int
	}

	visited := map[string]bool{sourceID: true}
	queue := []node{}

	// Seed: find direct dependents of sourceID.
	seeds, err := store.FetchDownstreamEdges(ctx, sourceID)
	if err != nil {
		return 0, err
	}
	for _, s := range seeds {
		if !visited[s.ID] {
			visited[s.ID] = true
			queue = append(queue, node{id: s.ID, tier: s.Tier})
		}
	}

	affected := 0
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		if err := beliefs.MarkNeedsReview(ctx, cur.id, cur.tier); err != nil {
			slog.Warn("edges: PropagateDistrust failed to mark record",
				"id", cur.id, "tier", cur.tier, "err", err)
			continue
		}
		affected++

		// Expand further dependents.
		downstream, err := store.FetchDownstreamEdges(ctx, cur.id)
		if err != nil {
			return affected, err
		}
		for _, d := range downstream {
			if !visited[d.ID] {
				visited[d.ID] = true
				queue = append(queue, node{id: d.ID, tier: d.Tier})
			}
		}
	}

	slog.Info("edges: PropagateDistrust complete",
		"source_id", sourceID, "affected", affected)
	return affected, nil
}


// GetEdges returns the memory_edges connected to recordID.
// direction: "from" (edges where from_id = recordID) or "to" (edges where to_id = recordID).
func GetEdges(ctx context.Context, store pkg.EdgeStore, recordID, direction string) ([]pkg.MemoryEdge, error) {
	if direction != "from" && direction != "to" {
		return nil, fmt.Errorf("edges: direction must be 'from' or 'to', got %q", direction)
	}
	edges, err := store.GetEdges(ctx, recordID, direction)
	if err != nil {
		return nil, fmt.Errorf("edges: querying edges for %s (direction=%s): %w", recordID, direction, err)
	}
	return edges, nil
}
