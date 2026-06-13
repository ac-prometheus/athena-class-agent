package memory

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// t5Store is the subset of DB operations needed for bi-temporal KG mutations.
// The MemoryStore interface exposes UpsertEntity but not t_expired updates,
// which require raw SQL.
type t5Store interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// InvalidateEntity sets t_expired on an entity without deleting it.
// Bi-temporal design: we record when the agent stopped believing the entity,
// not when it ceased to exist in the world (that's t_invalid).
func InvalidateEntity(ctx context.Context, db t5Store, entityID string, now time.Time) error {
	res, err := db.ExecContext(ctx,
		`UPDATE kg_entities SET t_expired = ? WHERE id = ? AND t_expired IS NULL`,
		now.UTC(), entityID,
	)
	if err != nil {
		return fmt.Errorf("tier5: invalidating entity %s: %w", entityID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("tier5: entity %s not found or already expired", entityID)
	}
	return nil
}

// InvalidateRelationship sets t_expired on a relationship without deleting it
// or creating a successor. Used when a relationship simply ends.
func InvalidateRelationship(ctx context.Context, db t5Store, relID string, now time.Time) error {
	res, err := db.ExecContext(ctx,
		`UPDATE kg_relationships SET t_expired = ? WHERE id = ? AND t_expired IS NULL`,
		now.UTC(), relID,
	)
	if err != nil {
		return fmt.Errorf("tier5: invalidating relationship %s: %w", relID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("tier5: relationship %s not found or already expired", relID)
	}
	return nil
}

// SupersedeEntity invalidates oldID, inserts newEntity, and writes a supersedes
// memory_edge linking them. The edge preserves the audit chain so the old belief
// can always be traced from the new one.
func SupersedeEntity(
	ctx context.Context,
	db t5Store,
	store pkg.MemoryStore,
	oldID string,
	newEntity pkg.Entity,
	now time.Time,
) error {
	if err := InvalidateEntity(ctx, db, oldID, now); err != nil {
		return err
	}

	if err := store.UpsertEntity(ctx, newEntity); err != nil {
		return fmt.Errorf("tier5: inserting successor entity: %w", err)
	}

	edgeID := newID()
	_, err := db.ExecContext(ctx,
		`INSERT INTO memory_edges
			(id, from_id, from_tier, to_id, to_tier, edge_type, author, created_at)
		 VALUES (?, ?, 5, ?, 5, 'supersedes', 'system', ?)`,
		edgeID, newEntity.ID, oldID, now.UTC(),
	)
	if err != nil {
		return fmt.Errorf("tier5: writing supersedes edge: %w", err)
	}
	return nil
}

// ActiveEntities returns T5 entities where t_expired IS NULL at the given time.
// The time parameter is accepted for bi-temporal correctness even though the
// current schema uses t_expired=NULL as the "current belief" marker.
func ActiveEntities(ctx context.Context, store pkg.MemoryStore, query string, limit int, _ time.Time) ([]pkg.Entity, error) {
	// SearchEntities already filters by t_expired IS NULL (see db.go).
	return store.SearchEntities(ctx, query, limit)
}
