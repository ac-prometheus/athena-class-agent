// Package storage implements repository ports defined in pkg/interfaces.go.
// Each implementation encapsulates SQL dialect so domain packages never see
// placeholder syntax or driver-name strings.
//
// SQLITE-ONLY: All current implementations (jobs_sqlite.go, consolidation_sqlite.go,
// lifecycle_sqlite.go, assembly_sqlite.go) target SQLite exclusively. They use
// SQLite placeholder syntax (?) and assume SQLite-specific behaviour. PostgreSQL
// implementations would live in separate files (e.g. jobs_postgres.go) and must
// never be aliased to these SQLite implementations. The bootstrap and NewApp
// constructors enforce this by rejecting postgres:// DSNs with an explicit error
// until PostgreSQL implementations are available (tracked in HARN-73).
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

// parseSQLiteTimestamp parses a SQLite TEXT timestamp into time.Time.
// SQLite stores timestamps as TEXT in "YYYY-MM-DD HH:MM:SS" format via
// CURRENT_TIMESTAMP. Scanning directly into time.Time fails — always scan
// as string first and parse through this helper.
func parseSQLiteTimestamp(s string) (time.Time, error) {
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
		"2006-01-02T15:04:05Z",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("storage: unparseable timestamp %q", s)
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

// aegisMetaJSON encodes an AegisAnnotation into the aegis_meta JSON column format.
// Returns an empty string for nil annotations (stored as '' in the column).
// Mirrors platform.aegisMetaJSON — duplicated here to avoid exporting an
// internal helper across the package boundary.
func aegisMetaJSON(ann *pkg.AegisAnnotation) string {
	if ann == nil {
		return ""
	}
	m := map[string]any{
		"trust_score":    ann.TrustScore,
		"source":         ann.Source,
		"content_source": ann.ContentSource,
		"scan_passed":    ann.ScanPassed,
		"sanitized":      ann.Sanitized,
		"annotated_at":   ann.AnnotatedAt.UTC().Format(time.RFC3339),
	}
	if len(ann.Flags) > 0 {
		m["flags"] = ann.Flags
	}
	raw, _ := json.Marshal(m)
	return string(raw)
}

// decodeAegisMeta decodes an aegis_meta column value into an AegisAnnotation.
// Returns nil for empty or unparseable strings.
func decodeAegisMeta(s string) *pkg.AegisAnnotation {
	if s == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}
	ann := &pkg.AegisAnnotation{}
	if v, ok := m["trust_score"].(float64); ok {
		ann.TrustScore = v
	}
	if v, ok := m["source"].(string); ok {
		ann.Source = v
	}
	if v, ok := m["content_source"].(string); ok {
		ann.ContentSource = v
	}
	if v, ok := m["scan_passed"].(bool); ok {
		ann.ScanPassed = v
	}
	if v, ok := m["sanitized"].(bool); ok {
		ann.Sanitized = v
	}
	if v, ok := m["annotated_at"].(string); ok {
		ann.AnnotatedAt, _ = time.Parse(time.RFC3339, v)
	}
	if v, ok := m["flags"].([]any); ok {
		for _, f := range v {
			if s, ok := f.(string); ok {
				ann.Flags = append(ann.Flags, s)
			}
		}
	}
	return ann
}

// contentSourcesJSON encodes a content sources slice as a JSON array string.
func contentSourcesJSON(sources []string) string {
	if len(sources) == 0 {
		return "[]"
	}
	raw, _ := json.Marshal(sources)
	return string(raw)
}

// decodeContentSources decodes a JSON array string into a content sources slice.
func decodeContentSources(s string) []string {
	if s == "" || s == "[]" {
		return nil
	}
	var sources []string
	if err := json.Unmarshal([]byte(s), &sources); err != nil {
		return nil
	}
	return sources
}
