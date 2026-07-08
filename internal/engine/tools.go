package engine

import (
	"log/slog"
	"strings"

	"github.com/ac-prometheus/athena-class-agent/internal/tools"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// TierActivator selects Tier 2 tool groups based on session signals such as
// orientation keywords, recent memory content, or explicit trigger strings.
// Tier 1 tools are always active; Tier 2 groups are selectively activated here;
// Tier 3 tools are only surfaced via the discover_tools handler at agent request.
type TierActivator struct {
	registry *tools.DefaultRegistry
}

// NewTierActivator creates a TierActivator backed by the given registry.
func NewTierActivator(registry *tools.DefaultRegistry) *TierActivator {
	return &TierActivator{registry: registry}
}

// ActivateTier2 checks each signal string against the keywords of every Tier 2
// tool group. A group is activated if any of its keywords appears (case-insensitive)
// in any signal. Returns the names of activated groups.
func (ta *TierActivator) ActivateTier2(signals []string) []string {
	groups := ta.registry.List()

	var activated []string
	for _, g := range groups {
		if g.Tier != 2 {
			continue
		}
		if matchesAnySignal(g.Keywords, signals) {
			slog.Debug("tier_activator: activating group",
				"group", g.Name, "tier", g.Tier)
			activated = append(activated, g.Name)
		}
	}
	return activated
}

// BuildToolList returns the combined ToolDef list for the system prompt:
// all Tier 1 tools, plus the tools belonging to each activated Tier 2 group name.
// Tier 3 tools are intentionally excluded — they are opt-in via discover_tools.
func (ta *TierActivator) BuildToolList(activatedGroups []string) []pkg.ToolDef {
	groupSet := make(map[string]bool, len(activatedGroups))
	for _, name := range activatedGroups {
		groupSet[name] = true
	}

	groups := ta.registry.List()
	var out []pkg.ToolDef
	for _, g := range groups {
		switch {
		case g.Tier == 1:
			out = append(out, g.Tools...)
		case g.Tier == 2 && groupSet[g.Name]:
			out = append(out, g.Tools...)
		}
	}
	return out
}

// matchesAnySignal reports whether any keyword appears in any signal (case-insensitive substring match).
func matchesAnySignal(keywords, signals []string) bool {
	for _, kw := range keywords {
		lower := strings.ToLower(kw)
		for _, sig := range signals {
			if strings.Contains(strings.ToLower(sig), lower) {
				return true
			}
		}
	}
	return false
}
