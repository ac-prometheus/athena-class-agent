package metabolism

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// AtomicT2T3Link writes the T3 narrative summary and updates T2 source logs
// in a single database transaction. If either operation fails, both roll back.
//
// This replaces the non-atomic write path in CompressSession (tier3.go) for
// the metabolism pipeline. The T2 back-link ensures the T4 -> T3 -> T2
// provenance chain remains unbroken even if the process crashes.
//
// The driverName parameter ("sqlite3" or "postgres") controls placeholder
// syntax in generated SQL.
//
// Schema dependency: experiential_logs.narrative_summary_id column must exist
// (added by a migration that introduces the T2->T3 back-link column).
func AtomicT2T3Link(ctx context.Context, db platform.DB, driverName string, narrative *pkg.NarrativeSummary, sourceLogIDs []string) error {
	if narrative == nil {
		return fmt.Errorf("metabolism: narrative summary is nil")
	}
	if len(sourceLogIDs) == 0 {
		return fmt.Errorf("metabolism: no source log IDs for T2 back-link")
	}

	txDB, ok := db.(platform.TxDB)
	if !ok {
		return fmt.Errorf("metabolism: database does not support transactions (platform.TxDB)")
	}

	tx, err := txDB.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("metabolism: beginning transaction: %w", err)
	}
	// Rollback is a no-op after Commit, so this is safe as a cleanup path.
	defer tx.Rollback() //nolint:errcheck

	// Step 1: INSERT the T3 narrative summary.
	beliefJSON := beliefMetaJSON(narrative.Belief)
	contentSrcsJSON := narrativeContentSourcesJSON(narrative.ContentSources)
	aegisMetaStr := narrativeAegisMetaJSON(narrative.ExternalAnnotation)
	insertQ := narrativeInsertSQL(driverName)
	if _, err := tx.ExecContext(ctx, insertQ,
		narrative.ID, narrative.SessionID, narrative.Content, beliefJSON,
		contentSrcsJSON, aegisMetaStr,
	); err != nil {
		return fmt.Errorf("metabolism: inserting T3 narrative %s: %w", narrative.ID, err)
	}

	// Step 2: UPDATE T2 experiential_logs with the back-link.
	updateQ, args := t2BacklinkSQL(driverName, narrative.ID, sourceLogIDs)
	if _, err := tx.ExecContext(ctx, updateQ, args...); err != nil {
		return fmt.Errorf("metabolism: updating T2 back-links for narrative %s: %w", narrative.ID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("metabolism: committing T2-T3 link transaction: %w", err)
	}
	return nil
}

// narrativeInsertSQL returns the INSERT statement for a T3 narrative summary,
// using the correct placeholder syntax for the given driver.
// Includes content_sources and aegis_meta for WP-C3 provenance carriage.
func narrativeInsertSQL(driver string) string {
	if driver == "postgres" {
		return `INSERT INTO narrative_summaries
			(id, session_id, content, belief_meta, content_sources, aegis_meta, created_at)
			VALUES ($1, $2, $3, $4::jsonb, $5, $6, now())`
	}
	return `INSERT INTO narrative_summaries
		(id, session_id, content, belief_meta, content_sources, aegis_meta, created_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`
}

// narrativeContentSourcesJSON encodes a content sources slice as a JSON array.
// Mirrors contentSourcesJSON in platform/db.go — duplicated here to avoid
// importing an unexported helper across the package boundary.
func narrativeContentSourcesJSON(sources []string) string {
	if len(sources) == 0 {
		return "[]"
	}
	raw, _ := json.Marshal(sources)
	return string(raw)
}

// narrativeAegisMetaJSON encodes an AegisAnnotation for the T3 aegis_meta column.
// Mirrors aegisMetaJSON in platform/db.go — duplicated to avoid cross-package dependency.
func narrativeAegisMetaJSON(ann *pkg.AegisAnnotation) string {
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

// t2BacklinkSQL builds the UPDATE statement and args for setting
// narrative_summary_id on T2 source logs.
func t2BacklinkSQL(driver string, narrativeID string, logIDs []string) (string, []any) {
	args := make([]any, 0, 1+len(logIDs))
	args = append(args, narrativeID)

	if driver == "postgres" {
		placeholders := make([]string, len(logIDs))
		for i, id := range logIDs {
			placeholders[i] = fmt.Sprintf("$%d", i+2)
			args = append(args, id)
		}
		q := fmt.Sprintf(
			`UPDATE experiential_logs SET narrative_summary_id = $1 WHERE id IN (%s)`,
			strings.Join(placeholders, ", "),
		)
		return q, args
	}

	// SQLite: all placeholders are ?
	qmarks := make([]string, len(logIDs))
	for i, id := range logIDs {
		qmarks[i] = "?"
		args = append(args, id)
	}
	q := fmt.Sprintf(
		`UPDATE experiential_logs SET narrative_summary_id = ? WHERE id IN (%s)`,
		strings.Join(qmarks, ", "),
	)
	return q, args
}

// beliefMetaJSON encodes a BeliefMeta into the belief_meta JSON column format.
// Mirrors platform.beliefMetaJSON — duplicated here to avoid exporting an
// internal helper that is only needed within the metabolism transaction.
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

// newMetabolismID generates a random UUID v4 string using crypto/rand.
// Mirrors platform.newPlatformID and memory.newID.
func newMetabolismID() string {
	var b [16]byte
	_, err := rand.Read(b[:])
	if err != nil {
		panic(fmt.Sprintf("metabolism: crypto/rand unavailable: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
