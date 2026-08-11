// Package storage implements repository ports defined in pkg/interfaces.go.
// Each implementation encapsulates SQL dialect so domain packages never see
// placeholder syntax or driver-name strings.
package storage

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// Compile-time interface satisfaction checks.
var (
	_ pkg.MetabolismJobStore = (*SQLiteJobStore)(nil)
	_ pkg.ConsolidationStore = (*SQLiteConsolidationStore)(nil)
	_ pkg.LifecycleStore     = (*SQLiteLifecycleStore)(nil)
	_ pkg.AssemblyStore      = (*SQLiteAssemblyStore)(nil)
)

// newID generates a random UUID v4 string using crypto/rand.
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("storage: crypto/rand unavailable: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// beliefMetaJSON encodes a BeliefMeta into the belief_meta JSON column format.
func beliefMetaJSON(b *pkg.BeliefMeta) string {
	if b == nil {
		return "{}"
	}
	m := map[string]any{
		"anchor_at":          b.AnchorAt.UTC().Format(time.RFC3339),
		"inference_distance": b.InferenceDistance,
		"verification_state": b.VerificationState,
		"source":             b.Source,
	}
	if b.EmotionalRegister != "" {
		m["emotional_register"] = b.EmotionalRegister
	}
	raw, _ := json.Marshal(m)
	return string(raw)
}

// embeddingJSON serializes a float32 embedding vector as a JSON array string
// for storage in SQLite TEXT columns. Returns NULL-safe empty string for nil/empty vectors.
func embeddingJSON(vec []float32) any {
	if len(vec) == 0 {
		return nil
	}
	raw, _ := json.Marshal(vec)
	return string(raw)
}
