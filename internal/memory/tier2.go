package memory

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// validContentSources enumerates the allowed content_source values for T2 entries.
// Enforcement here prevents invalid provenance data from entering the archive,
// which would break MemoryTrap back-tracing and T3 audit chains.
var validContentSources = map[string]bool{
	"operator":       true,
	"self":           true,
	"tool-result":    true,
	"browser-content": true,
	"search-result":  true,
	"forum-content":  true,
}

// AppendLog validates and appends a T2 experiential log entry.
// T2 is append-only — no update, no delete paths exist.
func AppendLog(ctx context.Context, store pkg.MemoryStore, entry pkg.ExperientialLog) error {
	if !validContentSources[entry.ContentSource] {
		return fmt.Errorf("tier2: invalid content_source %q", entry.ContentSource)
	}
	if entry.ID == "" {
		return fmt.Errorf("tier2: entry ID is required")
	}
	if entry.Content == "" {
		return fmt.Errorf("tier2: entry content is required")
	}
	return store.AppendExperiential(ctx, entry)
}

// t2Store is the subset of DB operations needed by QueryLogs.
// Defined here so callers can pass a *sql.DB directly without breaking
// the MemoryStore abstraction boundary.
type t2Store interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// QueryLogs retrieves T2 entries for a session, ordered oldest-first.
// This is the feed that CompressSession consumes to produce T3 summaries.
func QueryLogs(ctx context.Context, db t2Store, sessionID string, limit int) ([]pkg.ExperientialLog, error) {
	if limit <= 0 {
		limit = 200
	}

	rows, err := db.QueryContext(ctx,
		`SELECT id, session_id, content, content_source, created_at
		 FROM experiential_logs
		 WHERE session_id = ?
		 ORDER BY created_at ASC
		 LIMIT ?`,
		sessionID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("tier2: querying logs for session %s: %w", sessionID, err)
	}
	defer rows.Close()

	var out []pkg.ExperientialLog
	for rows.Next() {
		var e pkg.ExperientialLog
		var createdAt time.Time
		if err := rows.Scan(&e.ID, &e.SessionID, &e.Content, &e.ContentSource, &createdAt); err != nil {
			return nil, fmt.Errorf("tier2: scanning log row: %w", err)
		}
		e.CreatedAt = createdAt
		out = append(out, e)
	}
	return out, rows.Err()
}
