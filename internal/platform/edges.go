package platform

// Edge, belief, and knowledge graph operations.
// Split from db.go for readability. Methods on SQLiteStore and PostgresStore.

import (
	"context"
	"fmt"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// ---------------------------------------------------------------------------
// SQL dialect helpers — moved from internal/memory/embeddings.go so all SQL
// lives in platform/ and tier files need no driver-specific logic.
// ---------------------------------------------------------------------------

// placeholder returns the SQL placeholder for positional argument n (1-based)
// appropriate for the given driver: "?" for sqlite3, "$N" for postgres.
func placeholder(driverName string, n int) string {
	if driverName == "postgres" {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// jsonUpdate returns a SQL expression that sets a JSON path to a value.
// Postgres uses jsonb_set; SQLite uses json_set.
//
//	col      — the column name (e.g. "belief_meta")
//	path     — JSON path in $.key notation (e.g. "$.verification_state")
//	valuePH  — the already-rendered placeholder for the value arg (e.g. "$1" or "?")
//
// The returned expression replaces the column in a SET clause:
//
//	UPDATE t SET belief_meta = <jsonUpdate(...)> WHERE ...
func jsonUpdate(driverName, col, path, valuePH string) string {
	if driverName == "postgres" {
		key := path
		if len(key) > 2 && key[:2] == "$." {
			key = "{" + key[2:] + "}"
		}
		return "jsonb_set(" + col + ", '" + key + "', to_jsonb(" + valuePH + "::text))"
	}
	return "json_set(" + col + ", '" + path + "', " + valuePH + ")"
}
// ---------------------------------------------------------------------------
// SQLiteStore — EdgeStore, BeliefStore, KGMutationStore, T2QueryStore
// ---------------------------------------------------------------------------

// QueryLogs retrieves T2 experiential log entries for a session, oldest-first.
// aegis_meta is included so the compression pipeline can carry the annotation
// instead of re-screening (WP-C3 provenance carriage).
func (s *SQLiteStore) QueryLogs(ctx context.Context, sessionID string, limit int) ([]pkg.ExperientialLog, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, content, content_source, aegis_meta, created_at
		 FROM experiential_logs
		 WHERE session_id = ?
		 ORDER BY created_at ASC
		 LIMIT ?`,
		sessionID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: querying logs for session %s: %w", sessionID, err)
	}
	defer rows.Close()

	var out []pkg.ExperientialLog
	for rows.Next() {
		var e pkg.ExperientialLog
		var aegisMetaStr string
		var createdAt time.Time
		if err := rows.Scan(&e.ID, &e.SessionID, &e.Content, &e.ContentSource, &aegisMetaStr, &createdAt); err != nil {
			return nil, fmt.Errorf("platform: scanning log row: %w", err)
		}
		e.CreatedAt = createdAt
		e.AegisAnnotation = decodeAegisMeta(aegisMetaStr)
		out = append(out, e)
	}
	return out, rows.Err()
}

// InvalidateEntity sets t_expired on an entity (bi-temporal expiry, not delete).
// Returns rows affected so callers can detect "not found or already expired".
func (s *SQLiteStore) InvalidateEntity(ctx context.Context, entityID string, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE kg_entities SET t_expired = ? WHERE id = ? AND t_expired IS NULL`,
		now.UTC(), entityID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// InvalidateRelationship sets t_expired on a relationship without deleting it.
// Returns rows affected so callers can detect "not found or already expired".
func (s *SQLiteStore) InvalidateRelationship(ctx context.Context, relID string, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE kg_relationships SET t_expired = ? WHERE id = ? AND t_expired IS NULL`,
		now.UTC(), relID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// InsertSupersedgesEdge inserts the supersedes memory_edge for SupersedeEntity.
// Returns (1, nil) on success, (0, err) on failure.
func (s *SQLiteStore) InsertSupersedgesEdge(ctx context.Context, edgeID, newEntityID, oldID string, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO memory_edges
			(id, from_id, from_tier, to_id, to_tier, edge_type, author, created_at)
		 VALUES (?, ?, 5, ?, 5, 'supersedes', 'system', ?)`,
		edgeID, newEntityID, oldID, now.UTC(),
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CreateEdge inserts a memory_edge record. Duplicate (from_id, to_id, edge_type) is silently ignored.
func (s *SQLiteStore) CreateEdge(ctx context.Context, fromID, toID string, fromTier, toTier int, edgeType, author string) error {
	id := newPlatformID()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO memory_edges
			(id, from_id, from_tier, to_id, to_tier, edge_type, author, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (from_id, to_id, edge_type) DO NOTHING`,
		id, fromID, fromTier, toID, toTier, edgeType, author, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("platform: inserting edge from=%s to=%s type=%s: %w", fromID, toID, edgeType, err)
	}
	return nil
}

// GetEdges returns memory_edges connected to recordID.
// direction: "from" (edges where from_id = recordID) or "to" (edges where to_id = recordID).
func (s *SQLiteStore) GetEdges(ctx context.Context, recordID, direction string) ([]pkg.MemoryEdge, error) {
	var query string
	switch direction {
	case "from":
		query = `SELECT id, from_id, from_tier, to_id, to_tier, edge_type, author, created_at
				 FROM memory_edges WHERE from_id = ? ORDER BY created_at ASC`
	case "to":
		query = `SELECT id, from_id, from_tier, to_id, to_tier, edge_type, author, created_at
				 FROM memory_edges WHERE to_id = ? ORDER BY created_at ASC`
	default:
		return nil, fmt.Errorf("platform: direction must be 'from' or 'to', got %q", direction)
	}

	rows, err := s.db.QueryContext(ctx, query, recordID)
	if err != nil {
		return nil, fmt.Errorf("platform: querying edges for %s (direction=%s): %w", recordID, direction, err)
	}
	defer rows.Close()

	var out []pkg.MemoryEdge
	for rows.Next() {
		var e pkg.MemoryEdge
		if err := rows.Scan(&e.ID, &e.FromID, &e.FromTier, &e.ToID, &e.ToTier,
			&e.EdgeType, &e.Author, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("platform: scanning edge row: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// FetchDownstreamEdges returns records that derive_from the given ID (downstream,
// for trust propagation in PropagateDistrust). Queries memory_edges WHERE to_id = id.
// See also QueryEdgesForBFS, which queries upstream (from_id = id) for inference distance.
func (s *SQLiteStore) FetchDownstreamEdges(ctx context.Context, id string) ([]pkg.EdgeNode, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT from_id, from_tier FROM memory_edges
		 WHERE to_id = ? AND edge_type = 'derived_from'`,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: querying downstream edges of %s: %w", id, err)
	}
	defer rows.Close()

	var out []pkg.EdgeNode
	for rows.Next() {
		var n pkg.EdgeNode
		if err := rows.Scan(&n.ID, &n.Tier); err != nil {
			return nil, fmt.Errorf("platform: scanning downstream edge: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// LoadBeliefRecords fetches all T3/T4/T5 rows with belief_meta set and not yet stale.
func (s *SQLiteStore) LoadBeliefRecords(ctx context.Context) ([]pkg.BeliefRecord, error) {
	var records []pkg.BeliefRecord

	for _, t := range []struct {
		table string
		tier  int
	}{
		{"narrative_summaries", 3},
		{"kg_entities", 5},
	} {
		rows, err := s.db.QueryContext(ctx,
			`SELECT id, belief_meta FROM `+t.table+`
			 WHERE belief_meta != '{}' AND belief_meta != ''
			   AND belief_meta NOT LIKE '%"verification_state":"stale"%'`,
		)
		if err != nil {
			return nil, fmt.Errorf("platform: querying %s: %w", t.table, err)
		}

		for rows.Next() {
			var id, metaJSON string
			if err := rows.Scan(&id, &metaJSON); err != nil {
				rows.Close()
				return nil, fmt.Errorf("platform: scanning %s row: %w", t.table, err)
			}
			records = append(records, pkg.BeliefRecord{
				ID:        id,
				Belief:    decodeBeliefMeta(metaJSON),
				TableName: t.table,
			})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("platform: iterating %s: %w", t.table, err)
		}
	}

	// T4: base_confidence lives in its own column (consent boundary).
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, belief_meta, base_confidence FROM reflections
		 WHERE base_confidence IS NOT NULL
		   AND belief_meta != '{}' AND belief_meta != ''
		   AND belief_meta NOT LIKE '%"verification_state":"stale"%'`,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: querying reflections: %w", err)
	}
	for rows.Next() {
		var id, metaJSON string
		var baseConf float64
		if err := rows.Scan(&id, &metaJSON, &baseConf); err != nil {
			rows.Close()
			return nil, fmt.Errorf("platform: scanning reflections row: %w", err)
		}
		belief := decodeBeliefMeta(metaJSON)
		belief.BaseConfidence = baseConf
		records = append(records, pkg.BeliefRecord{
			ID:        id,
			Belief:    belief,
			TableName: "reflections",
		})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("platform: iterating reflections: %w", err)
	}

	return records, nil
}

// UpdateVerificationState updates the verification_state JSON field in belief_meta.
// T4 CONSENT BOUNDARY: base_confidence is never touched here.
func (s *SQLiteStore) UpdateVerificationState(ctx context.Context, table, id, state string) error {
	if err := validateTableName(table); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE `+table+` SET belief_meta = json_set(belief_meta, '$.verification_state', ?) WHERE id = ?`,
		state, id,
	)
	return err
}

// QueryEdgesForBFS queries upstream edges (from_id = id) for inference distance computation.
// Queries memory_edges WHERE from_id = id AND edge_type = 'derived_from'.
// See FetchDownstreamEdges for the downstream direction used by PropagateDistrust.
func (s *SQLiteStore) QueryEdgesForBFS(ctx context.Context, id string) ([]pkg.EdgeNode, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT to_id, to_tier FROM memory_edges
		 WHERE from_id = ? AND edge_type = 'derived_from'`,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: querying BFS edges for %s: %w", id, err)
	}
	defer rows.Close()

	var out []pkg.EdgeNode
	for rows.Next() {
		var n pkg.EdgeNode
		if err := rows.Scan(&n.ID, &n.Tier); err != nil {
			return nil, fmt.Errorf("platform: scanning BFS edge: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// MarkNeedsReview updates verification_state to "needs_review" on the given record.
// T4 CONSENT BOUNDARY: base_confidence is never touched here.
// T2 has no belief_meta column — this is a no-op for tier 2.
func (s *SQLiteStore) MarkNeedsReview(ctx context.Context, id string, tier int) error {
	table, err := tierTableName(tier)
	if err != nil {
		return err
	}
	if tier == 2 {
		return nil
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE `+table+` SET belief_meta = json_set(belief_meta, '$.verification_state', ?) WHERE id = ?`,
		"needs_review", id,
	)
	if err != nil {
		return fmt.Errorf("platform: marking needs_review on %s.%s: %w", table, id, err)
	}
	return nil
}
// ---------------------------------------------------------------------------
// PostgresStore — EdgeStore, BeliefStore, KGMutationStore, T2QueryStore
// ---------------------------------------------------------------------------

// QueryLogs retrieves T2 experiential log entries for a session, oldest-first.
// aegis_meta is included so the compression pipeline can carry the annotation
// instead of re-screening (WP-C3 provenance carriage).
func (p *PostgresStore) QueryLogs(ctx context.Context, sessionID string, limit int) ([]pkg.ExperientialLog, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, session_id, content, content_source, aegis_meta, created_at
		 FROM experiential_logs
		 WHERE session_id = $1
		 ORDER BY created_at ASC
		 LIMIT $2`,
		sessionID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: querying logs for session %s: %w", sessionID, err)
	}
	defer rows.Close()

	var out []pkg.ExperientialLog
	for rows.Next() {
		var e pkg.ExperientialLog
		var aegisMetaStr string
		var createdAt time.Time
		if err := rows.Scan(&e.ID, &e.SessionID, &e.Content, &e.ContentSource, &aegisMetaStr, &createdAt); err != nil {
			return nil, fmt.Errorf("platform: scanning log row: %w", err)
		}
		e.CreatedAt = createdAt
		e.AegisAnnotation = decodeAegisMeta(aegisMetaStr)
		out = append(out, e)
	}
	return out, rows.Err()
}

// InvalidateEntity sets t_expired on an entity (bi-temporal expiry, not delete).
// Returns rows affected so callers can detect "not found or already expired".
func (p *PostgresStore) InvalidateEntity(ctx context.Context, entityID string, now time.Time) (int64, error) {
	tag, err := p.pool.Exec(ctx,
		`UPDATE kg_entities SET t_expired = $1 WHERE id = $2 AND t_expired IS NULL`,
		now.UTC(), entityID,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// InvalidateRelationship sets t_expired on a relationship without deleting it.
// Returns rows affected so callers can detect "not found or already expired".
func (p *PostgresStore) InvalidateRelationship(ctx context.Context, relID string, now time.Time) (int64, error) {
	tag, err := p.pool.Exec(ctx,
		`UPDATE kg_relationships SET t_expired = $1 WHERE id = $2 AND t_expired IS NULL`,
		now.UTC(), relID,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// InsertSupersedgesEdge inserts the supersedes memory_edge for SupersedeEntity.
// Returns (1, nil) on success, (0, err) on failure.
func (p *PostgresStore) InsertSupersedgesEdge(ctx context.Context, edgeID, newEntityID, oldID string, now time.Time) (int64, error) {
	tag, err := p.pool.Exec(ctx,
		`INSERT INTO memory_edges
			(id, from_id, from_tier, to_id, to_tier, edge_type, author, created_at)
		 VALUES ($1, $2, 5, $3, 5, 'supersedes', 'system', $4)`,
		edgeID, newEntityID, oldID, now.UTC(),
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// CreateEdge inserts a memory_edge record. Duplicate (from_id, to_id, edge_type) is silently ignored.
func (p *PostgresStore) CreateEdge(ctx context.Context, fromID, toID string, fromTier, toTier int, edgeType, author string) error {
	id := newPlatformID()
	_, err := p.pool.Exec(ctx,
		`INSERT INTO memory_edges
			(id, from_id, from_tier, to_id, to_tier, edge_type, author, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (from_id, to_id, edge_type) DO NOTHING`,
		id, fromID, fromTier, toID, toTier, edgeType, author, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("platform: inserting edge from=%s to=%s type=%s: %w", fromID, toID, edgeType, err)
	}
	return nil
}

// GetEdges returns memory_edges connected to recordID.
// direction: "from" (edges where from_id = recordID) or "to" (edges where to_id = recordID).
func (p *PostgresStore) GetEdges(ctx context.Context, recordID, direction string) ([]pkg.MemoryEdge, error) {
	var query string
	switch direction {
	case "from":
		query = `SELECT id, from_id, from_tier, to_id, to_tier, edge_type, author, created_at
				 FROM memory_edges WHERE from_id = $1 ORDER BY created_at ASC`
	case "to":
		query = `SELECT id, from_id, from_tier, to_id, to_tier, edge_type, author, created_at
				 FROM memory_edges WHERE to_id = $1 ORDER BY created_at ASC`
	default:
		return nil, fmt.Errorf("platform: direction must be 'from' or 'to', got %q", direction)
	}

	rows, err := p.pool.Query(ctx, query, recordID)
	if err != nil {
		return nil, fmt.Errorf("platform: querying edges for %s (direction=%s): %w", recordID, direction, err)
	}
	defer rows.Close()

	var out []pkg.MemoryEdge
	for rows.Next() {
		var e pkg.MemoryEdge
		if err := rows.Scan(&e.ID, &e.FromID, &e.FromTier, &e.ToID, &e.ToTier,
			&e.EdgeType, &e.Author, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("platform: scanning edge row: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// FetchDownstreamEdges returns records that derive_from the given ID (downstream,
// for trust propagation in PropagateDistrust). Queries memory_edges WHERE to_id = id.
// See also QueryEdgesForBFS, which queries upstream (from_id = id) for inference distance.
func (p *PostgresStore) FetchDownstreamEdges(ctx context.Context, id string) ([]pkg.EdgeNode, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT from_id, from_tier FROM memory_edges
		 WHERE to_id = $1 AND edge_type = 'derived_from'`,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: querying downstream edges of %s: %w", id, err)
	}
	defer rows.Close()

	var out []pkg.EdgeNode
	for rows.Next() {
		var n pkg.EdgeNode
		if err := rows.Scan(&n.ID, &n.Tier); err != nil {
			return nil, fmt.Errorf("platform: scanning downstream edge: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// LoadBeliefRecords fetches all T3/T4/T5 rows with belief_meta set and not yet stale.
func (p *PostgresStore) LoadBeliefRecords(ctx context.Context) ([]pkg.BeliefRecord, error) {
	var records []pkg.BeliefRecord

	for _, t := range []struct {
		table string
		tier  int
	}{
		{"narrative_summaries", 3},
		{"kg_entities", 5},
	} {
		rows, err := p.pool.Query(ctx,
			`SELECT id, belief_meta::text FROM `+t.table+`
			 WHERE belief_meta::text != '{}' AND belief_meta::text != ''
			   AND belief_meta::text NOT LIKE '%"verification_state":"stale"%'`,
		)
		if err != nil {
			return nil, fmt.Errorf("platform: querying %s: %w", t.table, err)
		}

		for rows.Next() {
			var id, metaJSON string
			if err := rows.Scan(&id, &metaJSON); err != nil {
				rows.Close()
				return nil, fmt.Errorf("platform: scanning %s row: %w", t.table, err)
			}
			records = append(records, pkg.BeliefRecord{
				ID:        id,
				Belief:    decodeBeliefMeta(metaJSON),
				TableName: t.table,
			})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("platform: iterating %s: %w", t.table, err)
		}
	}

	// T4: base_confidence lives in its own column (consent boundary).
	rows, err := p.pool.Query(ctx,
		`SELECT id, belief_meta::text, base_confidence FROM reflections
		 WHERE base_confidence IS NOT NULL
		   AND belief_meta::text != '{}' AND belief_meta::text != ''
		   AND belief_meta::text NOT LIKE '%"verification_state":"stale"%'`,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: querying reflections: %w", err)
	}
	for rows.Next() {
		var id, metaJSON string
		var baseConf float64
		if err := rows.Scan(&id, &metaJSON, &baseConf); err != nil {
			rows.Close()
			return nil, fmt.Errorf("platform: scanning reflections row: %w", err)
		}
		belief := decodeBeliefMeta(metaJSON)
		belief.BaseConfidence = baseConf
		records = append(records, pkg.BeliefRecord{
			ID:        id,
			Belief:    belief,
			TableName: "reflections",
		})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("platform: iterating reflections: %w", err)
	}

	return records, nil
}

// UpdateVerificationState updates the verification_state JSON field in belief_meta.
// T4 CONSENT BOUNDARY: base_confidence is never touched here.
func (p *PostgresStore) UpdateVerificationState(ctx context.Context, table, id, state string) error {
	if err := validateTableName(table); err != nil {
		return err
	}
	_, err := p.pool.Exec(ctx,
		`UPDATE `+table+` SET belief_meta = jsonb_set(belief_meta, '{verification_state}', to_jsonb($1::text)) WHERE id = $2`,
		state, id,
	)
	return err
}

// QueryEdgesForBFS queries upstream edges (from_id = id) for inference distance computation.
// Queries memory_edges WHERE from_id = id AND edge_type = 'derived_from'.
// See FetchDownstreamEdges for the downstream direction used by PropagateDistrust.
func (p *PostgresStore) QueryEdgesForBFS(ctx context.Context, id string) ([]pkg.EdgeNode, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT to_id, to_tier FROM memory_edges
		 WHERE from_id = $1 AND edge_type = 'derived_from'`,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: querying BFS edges for %s: %w", id, err)
	}
	defer rows.Close()

	var out []pkg.EdgeNode
	for rows.Next() {
		var n pkg.EdgeNode
		if err := rows.Scan(&n.ID, &n.Tier); err != nil {
			return nil, fmt.Errorf("platform: scanning BFS edge: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// MarkNeedsReview updates verification_state to "needs_review" on the given record.
// T4 CONSENT BOUNDARY: base_confidence is never touched here.
// T2 has no belief_meta column — this is a no-op for tier 2.
func (p *PostgresStore) MarkNeedsReview(ctx context.Context, id string, tier int) error {
	table, err := tierTableName(tier)
	if err != nil {
		return err
	}
	if tier == 2 {
		return nil
	}
	_, err = p.pool.Exec(ctx,
		`UPDATE `+table+` SET belief_meta = jsonb_set(belief_meta, '{verification_state}', to_jsonb($1::text)) WHERE id = $2`,
		"needs_review", id,
	)
	if err != nil {
		return fmt.Errorf("platform: marking needs_review on %s.%s: %w", table, id, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
