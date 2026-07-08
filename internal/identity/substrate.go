package identity

import (
	"context"
	"fmt"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// RecordTransition inserts a new substrate transition record.
// If entry.PreviousEntryID is empty the caller is recording the first-ever entry.
func RecordTransition(ctx context.Context, store pkg.SubstrateStore, entry pkg.SubstrateTransition) error {
	if entry.ID == "" {
		return fmt.Errorf("identity: SubstrateTransition.ID must be set by the caller")
	}
	if entry.ModelName == "" {
		return fmt.Errorf("identity: SubstrateTransition.ModelName is required")
	}
	if entry.TransitionDate.IsZero() {
		entry.TransitionDate = time.Now().UTC()
	}
	return store.InsertSubstrateTransition(ctx, entry)
}
