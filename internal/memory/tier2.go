package memory

import (
	"context"
	"fmt"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// validContentSources enumerates the allowed content_source values for T2 entries.
// Enforcement here prevents invalid provenance data from entering the archive,
// which would break MemoryTrap back-tracing and T3 audit chains.
var validContentSources = map[string]bool{
	"operator":        true,
	"self":            true,
	"tool-result":     true,
	"browser-content": true,
	"search-result":   true,
	"forum-content":   true,
	"discord":         true,
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

// QueryLogs retrieves T2 entries for a session, ordered oldest-first.
// This is the feed that CompressSession consumes to produce T3 summaries.
func QueryLogs(ctx context.Context, store pkg.T2QueryStore, sessionID string, limit int) ([]pkg.ExperientialLog, error) {
	if limit <= 0 {
		limit = 200
	}
	return store.QueryLogs(ctx, sessionID, limit)
}
