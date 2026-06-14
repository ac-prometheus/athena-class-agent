package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// InvalidateEntity sets t_expired on an entity without deleting it.
// Bi-temporal design: we record when the agent stopped believing the entity,
// not when it ceased to exist in the world (that's t_invalid).
func InvalidateEntity(ctx context.Context, store pkg.KGMutationStore, entityID string, now time.Time) error {
	n, err := store.InvalidateEntity(ctx, entityID, now)
	if err != nil {
		return fmt.Errorf("tier5: invalidating entity %s: %w", entityID, err)
	}
	if n == 0 {
		return fmt.Errorf("tier5: entity %s not found or already expired", entityID)
	}
	return nil
}

// InvalidateRelationship sets t_expired on a relationship without deleting it
// or creating a successor. Used when a relationship simply ends.
func InvalidateRelationship(ctx context.Context, store pkg.KGMutationStore, relID string, now time.Time) error {
	n, err := store.InvalidateRelationship(ctx, relID, now)
	if err != nil {
		return fmt.Errorf("tier5: invalidating relationship %s: %w", relID, err)
	}
	if n == 0 {
		return fmt.Errorf("tier5: relationship %s not found or already expired", relID)
	}
	return nil
}

// SupersedeEntity invalidates oldID, inserts newEntity, and writes a supersedes
// memory_edge linking them. The edge preserves the audit chain so the old belief
// can always be traced from the new one.
//
// Takes two stores:
//   - kg pkg.KGMutationStore — for invalidating the old entity and inserting the supersedes edge
//   - mem pkg.MemoryStore — for upserting the new entity
func SupersedeEntity(
	ctx context.Context,
	kg pkg.KGMutationStore,
	mem pkg.MemoryStore,
	oldID string,
	newEntity pkg.Entity,
	now time.Time,
) error {
	if err := InvalidateEntity(ctx, kg, oldID, now); err != nil {
		return err
	}

	if err := mem.UpsertEntity(ctx, newEntity); err != nil {
		return fmt.Errorf("tier5: inserting successor entity: %w", err)
	}

	edgeID := newID()
	if _, err := kg.InsertSupersedgesEdge(ctx, edgeID, newEntity.ID, oldID, now); err != nil {
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
