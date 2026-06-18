package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// DiscoverToolsHandler lets the agent explore Tier 3 tool categories at runtime.
// It is always registered as Tier 1 so the agent can always invoke it, even
// before any Tier 3 groups are activated. Listing without a category returns
// the available Tier 3 categories; providing a category returns the full tool
// definitions for that group.
type DiscoverToolsHandler struct {
	registry *DefaultRegistry
}

// Compile-time interface check.
var _ pkg.ToolHandler = (*DiscoverToolsHandler)(nil)

// NewDiscoverToolsHandler creates a DiscoverToolsHandler backed by registry.
func NewDiscoverToolsHandler(registry *DefaultRegistry) *DiscoverToolsHandler {
	return &DiscoverToolsHandler{registry: registry}
}

// Name implements pkg.ToolHandler.
func (h *DiscoverToolsHandler) Name() string { return "discover_tools" }

// Execute returns either a category listing (no args) or the tool definitions
// for a specific Tier 3 category.
// args["category"] — optional; if provided, returns tools in that category.
func (h *DiscoverToolsHandler) Execute(_ context.Context, args map[string]any) (string, error) {
	category, hasCategory := stringArg(args, "category")

	groups := h.registry.List()

	// Collect tier-3 groups.
	var tier3 []pkg.ToolGroup
	for _, g := range groups {
		if g.Tier == 3 {
			tier3 = append(tier3, g)
		}
	}

	if !hasCategory || category == "" {
		return h.listCategories(tier3), nil
	}
	return h.describeCategory(tier3, category)
}

// listCategories returns a human-readable summary of available Tier 3 categories.
func (h *DiscoverToolsHandler) listCategories(groups []pkg.ToolGroup) string {
	if len(groups) == 0 {
		return "no Tier 3 tool categories registered"
	}
	var sb strings.Builder
	sb.WriteString("=== Available Tier 3 Tool Categories ===\n\n")
	for _, g := range groups {
		sb.WriteString(fmt.Sprintf("• %s (%d tools)", g.Name, len(g.Tools)))
		if len(g.Keywords) > 0 {
			sb.WriteString(fmt.Sprintf(" — keywords: %s", strings.Join(g.Keywords, ", ")))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\nUse discover_tools category=<name> to see tool details.")
	return sb.String()
}

// describeCategory returns the full tool definitions for a matching Tier 3 group.
func (h *DiscoverToolsHandler) describeCategory(groups []pkg.ToolGroup, category string) (string, error) {
	lc := strings.ToLower(category)
	for _, g := range groups {
		if strings.ToLower(g.Name) == lc {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("=== Tier 3: %s ===\n\n", g.Name))
			for _, t := range g.Tools {
				sb.WriteString(fmt.Sprintf("• %s\n", t.Name))
				if t.Description != "" {
					sb.WriteString(fmt.Sprintf("  %s\n", t.Description))
				}
			}
			return strings.TrimSpace(sb.String()), nil
		}
	}

	// Build list of available categories for the error message.
	names := make([]string, 0, len(groups))
	for _, g := range groups {
		names = append(names, g.Name)
	}
	if len(names) == 0 {
		return "", fmt.Errorf("discover_tools: category %q not found (no Tier 3 groups registered)", category)
	}
	return "", fmt.Errorf("discover_tools: category %q not found; available: %s", category, strings.Join(names, ", "))
}
