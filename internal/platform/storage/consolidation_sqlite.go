package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// SQLiteConsolidationStore implements pkg.ConsolidationStore for SQLite.
type SQLiteConsolidationStore struct {
	db platform.DB
}

// NewSQLiteConsolidationStore returns a ConsolidationStore backed by SQLite.
func NewSQLiteConsolidationStore(db platform.DB) *SQLiteConsolidationStore {
	return &SQLiteConsolidationStore{db: db}
}

func (s *SQLiteConsolidationStore) CommitNarrative(ctx context.Context, narrative *pkg.NarrativeSummary, sourceLogIDs []string) error {
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

	beliefJSON := beliefMetaJSON(narrative.Belief)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO narrative_summaries
			(id, session_id, content, belief_meta, created_at)
		 VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		narrative.ID, narrative.SessionID, narrative.Content, beliefJSON,
	)
	if err != nil {
		return fmt.Errorf("storage: inserting T3 narrative %s: %w", narrative.ID, err)
	}

	qmarks := make([]string, len(sourceLogIDs))
	args := make([]any, 0, 1+len(sourceLogIDs))
	args = append(args, narrative.ID)
	for i, id := range sourceLogIDs {
		qmarks[i] = "?"
		args = append(args, id)
	}
	updateQ := fmt.Sprintf(
		`UPDATE experiential_logs SET narrative_summary_id = ? WHERE id IN (%s)`,
		strings.Join(qmarks, ", "),
	)
	if _, err := tx.ExecContext(ctx, updateQ, args...); err != nil {
		return fmt.Errorf("storage: updating T2 back-links for narrative %s: %w", narrative.ID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("storage: committing consolidation transaction: %w", err)
	}
	return nil
}

func (s *SQLiteConsolidationStore) UncoveredLogs(ctx context.Context, sessionID string) ([]pkg.ExperientialLog, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, content, content_source, created_at
		 FROM experiential_logs
		 WHERE session_id = ? AND narrative_summary_id IS NULL
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
		if err := rows.Scan(
			&log.ID, &log.SessionID, &log.Content,
			&log.ContentSource, &log.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("storage: scanning uncovered log: %w", err)
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}
