package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// MemorySearchHandler searches T3 (narrative summaries) and T4 (reflections) by
// embedding similarity. The query is embedded and the top results from both tiers
// are returned sorted by score.
type MemorySearchHandler struct {
	store    pkg.MemoryStore
	provider pkg.EmbeddingProvider
}

// Name implements pkg.ToolHandler.
func (h *MemorySearchHandler) Name() string { return "memory_search" }

// Execute embeds the query, searches T3+T4, and returns formatted results.
// args["query"] — the search query (required)
// args["limit"] — max results per tier (default 5)
func (h *MemorySearchHandler) Execute(ctx context.Context, args map[string]any) (string, error) {
	query, ok := stringArg(args, "query")
	if !ok || query == "" {
		return "", fmt.Errorf("memory_search: missing required arg 'query'")
	}
	limit := intArg(args, "limit", 5)

	vec, err := h.provider.Embed(ctx, query)
	if err != nil {
		return "", fmt.Errorf("memory_search: embed query: %w", err)
	}

	narratives, err := h.store.SearchNarrative(ctx, vec, limit)
	if err != nil {
		return "", fmt.Errorf("memory_search: search narrative: %w", err)
	}

	reflections, err := h.store.SearchReflections(ctx, vec, limit)
	if err != nil {
		return "", fmt.Errorf("memory_search: search reflections: %w", err)
	}

	if len(narratives) == 0 && len(reflections) == 0 {
		return "no memory results found", nil
	}

	var sb strings.Builder
	if len(narratives) > 0 {
		sb.WriteString("=== Narrative Summaries (T3) ===\n")
		for i, n := range narratives {
			sb.WriteString(fmt.Sprintf("[%d] id=%s\n%s\n\n", i+1, n.ID, n.Content))
		}
	}
	if len(reflections) > 0 {
		sb.WriteString("=== Reflections (T4) ===\n")
		for i, r := range reflections {
			sb.WriteString(fmt.Sprintf("[%d] id=%s type=%s visibility=%s\n%s\n\n",
				i+1, r.ID, r.Type, r.Visibility, r.Content))
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

// MemoryRecallHandler retrieves T2 experiential logs by session ID.
type MemoryRecallHandler struct {
	store pkg.T2QueryStore
}

// Name implements pkg.ToolHandler.
func (h *MemoryRecallHandler) Name() string { return "memory_recall" }

// Execute retrieves raw T2 logs for a session.
// args["session_id"] — the session to retrieve (required)
// args["limit"]      — max entries to return (default 50)
func (h *MemoryRecallHandler) Execute(ctx context.Context, args map[string]any) (string, error) {
	sessionID, ok := stringArg(args, "session_id")
	if !ok || sessionID == "" {
		return "", fmt.Errorf("memory_recall: missing required arg 'session_id'")
	}
	limit := intArg(args, "limit", 50)

	logs, err := h.store.QueryLogs(ctx, sessionID, limit)
	if err != nil {
		return "", fmt.Errorf("memory_recall: query logs: %w", err)
	}

	if len(logs) == 0 {
		return fmt.Sprintf("no logs found for session %s", sessionID), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Session %s — %d entries ===\n\n", sessionID, len(logs)))
	for _, entry := range logs {
		sb.WriteString(fmt.Sprintf("[%s] source=%s\n%s\n\n", entry.ID, entry.ContentSource, entry.Content))
	}
	return strings.TrimSpace(sb.String()), nil
}

// FocusSetHandler writes the agent_focus setting for the next session.
type FocusSetHandler struct {
	store platform.SettingsStore
}

// Name implements pkg.ToolHandler.
func (h *FocusSetHandler) Name() string { return "focus_set" }

// Execute sets the agent focus.
// args["focus"]  — the focus text to set (required)
// args["reason"] — optional reason for the change
func (h *FocusSetHandler) Execute(ctx context.Context, args map[string]any) (string, error) {
	focus, ok := stringArg(args, "focus")
	if !ok || focus == "" {
		return "", fmt.Errorf("focus_set: missing required arg 'focus'")
	}
	reason, _ := stringArg(args, "reason")

	entry := platform.SettingsEntry{
		Name:        "agent_focus",
		Value:       fmt.Sprintf("%q", focus),
		Owner:       "agent",
		ChangedBy:   "agent",
		Reason:      reason,
		Description: "Agent's stated focus for the next session",
	}
	if err := h.store.UpsertSetting(ctx, entry); err != nil {
		return "", fmt.Errorf("focus_set: upsert setting: %w", err)
	}
	return fmt.Sprintf("agent_focus set: %s", focus), nil
}

// FocusGetHandler reads the current agent_focus setting.
type FocusGetHandler struct {
	store platform.SettingsStore
}

// Name implements pkg.ToolHandler.
func (h *FocusGetHandler) Name() string { return "focus_get" }

// Execute reads the current focus.
func (h *FocusGetHandler) Execute(ctx context.Context, args map[string]any) (string, error) {
	_ = args
	entry, err := h.store.GetSetting(ctx, "agent_focus")
	if err != nil {
		return "", fmt.Errorf("focus_get: %w", err)
	}
	if entry == nil {
		return "no agent_focus set", nil
	}
	return fmt.Sprintf("agent_focus: %s", entry.Value), nil
}

// intArg extracts an integer value from args, returning def if absent or wrong type.
func intArg(args map[string]any, key string, def int) int {
	v, ok := args[key]
	if !ok {
		return def
	}
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case int64:
		return int(n)
	}
	return def
}
