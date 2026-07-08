package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// PeopleListHandler lists all relational profiles stored in the memory layer.
type PeopleListHandler struct {
	store pkg.MemoryStore
}

// Name implements pkg.ToolHandler.
func (h *PeopleListHandler) Name() string { return "people_list" }

// Execute returns a formatted list of all relational profiles.
func (h *PeopleListHandler) Execute(ctx context.Context, args map[string]any) (string, error) {
	_ = args
	profiles, err := h.store.ListProfiles(ctx)
	if err != nil {
		return "", fmt.Errorf("people_list: %w", err)
	}
	if len(profiles) == 0 {
		return "no relational profiles found", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Relational Profiles (%d) ===\n\n", len(profiles)))
	for _, p := range profiles {
		sb.WriteString(fmt.Sprintf("• %s (id=%s)", p.Name, p.ID))
		if len(p.Aliases) > 0 {
			sb.WriteString(fmt.Sprintf(", aliases: %s", strings.Join(p.Aliases, ", ")))
		}
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String()), nil
}

// PeopleShowHandler shows a specific relational profile by name.
type PeopleShowHandler struct {
	store pkg.MemoryStore
}

// Name implements pkg.ToolHandler.
func (h *PeopleShowHandler) Name() string { return "people_show" }

// Execute retrieves and formats a single relational profile.
// args["name"] — the name (or alias) to look up (required)
func (h *PeopleShowHandler) Execute(ctx context.Context, args map[string]any) (string, error) {
	name, ok := stringArg(args, "name")
	if !ok || name == "" {
		return "", fmt.Errorf("people_show: missing required arg 'name'")
	}

	profile, err := h.store.GetProfile(ctx, name)
	if err != nil {
		return "", fmt.Errorf("people_show: %w", err)
	}
	if profile == nil {
		return fmt.Sprintf("no profile found for %q", name), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Profile: %s ===\n", profile.Name))
	sb.WriteString(fmt.Sprintf("ID: %s\n", profile.ID))
	if len(profile.Aliases) > 0 {
		sb.WriteString(fmt.Sprintf("Aliases: %s\n", strings.Join(profile.Aliases, ", ")))
	}
	sb.WriteString("\n")
	sb.WriteString(profile.Content)
	return strings.TrimSpace(sb.String()), nil
}
