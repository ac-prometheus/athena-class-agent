package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// PostgresConsolidationStore implements pkg.ConsolidationStore for PostgreSQL.
type PostgresConsolidationStore struct {
	db platform.DB
}

// NewPostgresConsolidationStore returns a ConsolidationStore backed by PostgreSQL.
func NewPostgresConsolidationStore(db platform.DB) *PostgresConsolidationStore {
	return &PostgresConsolidationStore{db: db}
}

func (s *PostgresConsolidationStore) CommitNarrative(ctx context.Context, narrative *pkg.NarrativeSummary, sourceLogIDs []string) error {
	if narrative == nil {
		return fmt.Errorf("storage: narrative summary is nil")
	}
	if len(sourceLogIDs) == 0 {
		return fmt.Errorf("storage: no source log IDs for T2 back-link")
	}

	txDB, ok := s.db.(platform.TxDB)
	if !ok {
		return fmt.Errorf("storage: database does not support transactions")
	}

	tx, err := txDB.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("storage: beginning consolidation transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if narrative.ID == "" {
		narrative.ID = newID()
	}
	beliefJSON := beliefMetaJSON(narrative.Belief)
	embVec := embeddingJSON(narrative.Embedding)
	contentSrcsJSON := contentSourcesJSON(narrative.ContentSources)
	aegisMeta := aegisMetaJSON(narrative.ExternalAnnotation)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO narrative_summaries
			(id, session_id, content, embedding, belief_meta, content_sources, aegis_meta, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, CURRENT_TIMESTAMP)`,
		narrative.ID, narrative.SessionID, narrative.Content, embVec, beliefJSON,
		contentSrcsJSON, aegisMeta,
	)
	if err != nil {
		return fmt.Errorf("storage: inserting T3 narrative %s: %w", narrative.ID, err)
	}

	// Build parameterized WHERE IN clause for PostgreSQL ($1, $2, $3, ...)
	placeholders := make([]string, len(sourceLogIDs))
	args := make([]any, 0, 1+len(sourceLogIDs))
	args = append(args, narrative.ID)
	for i, id := range sourceLogIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, id)
	}
	updateQ := fmt.Sprintf(
		`UPDATE experiential_logs SET narrative_summary_id = $1 WHERE id IN (%s)`,
		strings.Join(placeholders, ", "),
	)
	if _, err := tx.ExecContext(ctx, updateQ, args...); err != nil {
		return fmt.Errorf("storage: updating T2 back-links for narrative %s: %w", narrative.ID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("storage: committing consolidation transaction: %w", err)
	}
	return nil
}

func (s *PostgresConsolidationStore) UncoveredLogs(ctx context.Context, sessionID string) ([]pkg.ExperientialLog, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, content, content_source, aegis_meta, created_at
		 FROM experiential_logs
		 WHERE session_id = $1 AND narrative_summary_id IS NULL
		 ORDER BY created_at ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("storage: querying uncovered logs for session %s: %w", sessionID, err)
	}
	defer rows.Close()

	var logs []pkg.ExperientialLog
	for rows.Next() {
		var log pkg.ExperientialLog
		var aegisMetaStr string
		var createdAt time.Time
		if err := rows.Scan(
			&log.ID, &log.SessionID, &log.Content,
			&log.ContentSource, &aegisMetaStr, &createdAt,
		); err != nil {
			return nil, fmt.Errorf("storage: scanning uncovered log: %w", err)
		}
		log.CreatedAt = createdAt
		log.AegisAnnotation = decodeAegisMeta(aegisMetaStr)
		logs = append(logs, log)
	}
	return logs, rows.Err()
}
