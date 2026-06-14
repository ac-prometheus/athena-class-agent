package memory

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
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

// edgeDB is the narrow raw-DB interface needed for edge operations.
// Kept separate from MemoryStore because the edge table is not part of that interface.
type edgeDB interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (platform.Rows, error)
}

// MemoryEdge is the in-memory representation of a memory_edges row.
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

// CreateEdge validates and inserts a memory_edge record.
// edge_type and author are validated against their allowed sets before insertion.
// driverName must be "sqlite3" or "postgres".
func CreateEdge(ctx context.Context, db edgeDB, driverName, fromID, toID string, fromTier, toTier int, edgeType, author string) error {
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

	ph := func(n int) string { return placeholder(driverName, n) }
	id := newID()
	_, err := db.ExecContext(ctx,
		`INSERT INTO memory_edges
			(id, from_id, from_tier, to_id, to_tier, edge_type, author, created_at)
		 VALUES (`+ph(1)+`, `+ph(2)+`, `+ph(3)+`, `+ph(4)+`, `+ph(5)+`, `+ph(6)+`, `+ph(7)+`, `+ph(8)+`)
		 ON CONFLICT (from_id, to_id, edge_type) DO NOTHING`,
		id, fromID, fromTier, toID, toTier, edgeType, author, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("edges: inserting edge from=%s to=%s type=%s: %w", fromID, toID, edgeType, err)
	}
	return nil
}

// PropagateDistrust runs a BFS from sourceID over derived_from edges,
// marking each downstream record's verification_state as "needs_review".
// This is read-annotate only — content and confidence are never modified.
// Returns the count of affected records.
// driverName must be "sqlite3" or "postgres".
func PropagateDistrust(ctx context.Context, db edgeDB, driverName, sourceID string) (int, error) {
	// BFS state: collect IDs and their tier to know which table to update.
	type node struct {
		id   string
		tier int
	}

	visited := map[string]bool{sourceID: true}
	queue := []node{}

	// Seed: find direct dependents of sourceID.
	seeds, err := fetchDownstreamEdges(ctx, db, driverName, sourceID)
	if err != nil {
		return 0, err
	}
	for _, s := range seeds {
		if !visited[s.id] {
			visited[s.id] = true
			queue = append(queue, s)
		}
	}

	affected := 0
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		if err := markNeedsReview(ctx, db, driverName, cur.id, cur.tier); err != nil {
			slog.Warn("edges: PropagateDistrust failed to mark record",
				"id", cur.id, "tier", cur.tier, "err", err)
			continue
		}
		affected++

		// Expand further dependents.
		downstream, err := fetchDownstreamEdges(ctx, db, driverName, cur.id)
		if err != nil {
			return affected, err
		}
		for _, d := range downstream {
			if !visited[d.id] {
				visited[d.id] = true
				queue = append(queue, d)
			}
		}
	}

	slog.Info("edges: PropagateDistrust complete",
		"source_id", sourceID, "affected", affected)
	return affected, nil
}

// fetchDownstreamEdges returns all records that derive_from the given ID.
func fetchDownstreamEdges(ctx context.Context, db edgeDB, driverName, id string) ([]struct{ id string; tier int }, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT from_id, from_tier FROM memory_edges
		 WHERE to_id = `+placeholder(driverName, 1)+` AND edge_type = 'derived_from'`,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("edges: querying downstream edges of %s: %w", id, err)
	}
	defer rows.Close()

	var out []struct{ id string; tier int }
	for rows.Next() {
		var n struct{ id string; tier int }
		if err := rows.Scan(&n.id, &n.tier); err != nil {
			return nil, fmt.Errorf("edges: scanning downstream edge: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// tierTable maps tier number to DB table name.
func tierTable(tier int) (string, error) {
	switch tier {
	case 2:
		return "experiential_logs", nil
	case 3:
		return "narrative_summaries", nil
	case 4:
		return "reflections", nil
	case 5:
		return "kg_entities", nil
	default:
		return "", fmt.Errorf("edges: unknown tier %d", tier)
	}
}

// markNeedsReview updates verification_state on the given record.
// T4 CONSENT BOUNDARY: base_confidence is never touched here.
func markNeedsReview(ctx context.Context, db edgeDB, driverName, id string, tier int) error {
	table, err := tierTable(tier)
	if err != nil {
		return err
	}
	if tier == 2 {
		// T2 has no belief_meta column — nothing to annotate.
		return nil
	}
	_, err = db.ExecContext(ctx,
		`UPDATE `+table+` SET belief_meta = `+jsonUpdate(driverName, "belief_meta", "$.verification_state", placeholder(driverName, 1))+
			` WHERE id = `+placeholder(driverName, 2),
		"needs_review", id,
	)
	if err != nil {
		return fmt.Errorf("edges: marking needs_review on %s.%s: %w", table, id, err)
	}
	return nil
}

// GetEdges returns the memory_edges connected to recordID.
// direction: "from" (edges where from_id = recordID) or "to" (edges where to_id = recordID).
// driverName must be "sqlite3" or "postgres".
func GetEdges(ctx context.Context, db edgeDB, driverName, recordID string, direction string) ([]MemoryEdge, error) {
	ph := placeholder(driverName, 1)
	var query string
	switch direction {
	case "from":
		query = `SELECT id, from_id, from_tier, to_id, to_tier, edge_type, author, created_at
				 FROM memory_edges WHERE from_id = ` + ph + ` ORDER BY created_at ASC`
	case "to":
		query = `SELECT id, from_id, from_tier, to_id, to_tier, edge_type, author, created_at
				 FROM memory_edges WHERE to_id = ` + ph + ` ORDER BY created_at ASC`
	default:
		return nil, fmt.Errorf("edges: direction must be 'from' or 'to', got %q", direction)
	}

	rows, err := db.QueryContext(ctx, query, recordID)
	if err != nil {
		return nil, fmt.Errorf("edges: querying edges for %s (direction=%s): %w", recordID, direction, err)
	}
	defer rows.Close()

	var out []MemoryEdge
	for rows.Next() {
		var e MemoryEdge
		if err := rows.Scan(&e.ID, &e.FromID, &e.FromTier, &e.ToID, &e.ToTier,
			&e.EdgeType, &e.Author, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("edges: scanning edge row: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
