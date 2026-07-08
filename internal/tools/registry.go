package tools

import (
	"sort"
	"sync"

	pkg "github.com/ac-prometheus/athena-class-agent/pkg"
)

// Definer is an optional interface a ToolHandler may implement to provide
// a rich ToolDef (description + parameters schema) for the model context.
type Definer interface {
	Define() pkg.ToolDef
}

// DefaultRegistry is a thread-safe implementation of pkg.ToolRegistry.
type DefaultRegistry struct {
	mu       sync.RWMutex
	handlers map[string]pkg.ToolHandler
	tiers    map[string]int
	keywords map[string][]string
}

// Compile-time check.
var _ pkg.ToolRegistry = (*DefaultRegistry)(nil)

// NewDefaultRegistry returns an empty registry.
func NewDefaultRegistry() *DefaultRegistry {
	return &DefaultRegistry{
		handlers: make(map[string]pkg.ToolHandler),
		tiers:    make(map[string]int),
		keywords: make(map[string][]string),
	}
}

// Register adds h at tier 1 with no keywords.
func (r *DefaultRegistry) Register(h pkg.ToolHandler) {
	r.RegisterWithMeta(h, 1, nil)
}

// RegisterWithMeta adds h at the given tier with the given keyword set.
// Panics on duplicate name — registration happens at startup, not runtime.
func (r *DefaultRegistry) RegisterWithMeta(h pkg.ToolHandler, tier int, keywords []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := h.Name()
	if _, exists := r.handlers[name]; exists {
		panic("tools.DefaultRegistry: duplicate handler name: " + name)
	}
	r.handlers[name] = h
	r.tiers[name] = tier
	r.keywords[name] = keywords
}

// Get returns the handler for name, if present.
func (r *DefaultRegistry) Get(name string) (pkg.ToolHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[name]
	return h, ok
}

// List groups handlers by tier (ascending) and returns []pkg.ToolGroup.
// If a handler implements Definer, its Define() result is used; otherwise
// a minimal ToolDef with only Name is returned.
func (r *DefaultRegistry) List() []pkg.ToolGroup {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Collect unique tiers.
	tierSet := make(map[int]struct{})
	for _, t := range r.tiers {
		tierSet[t] = struct{}{}
	}
	tiers := make([]int, 0, len(tierSet))
	for t := range tierSet {
		tiers = append(tiers, t)
	}
	sort.Ints(tiers)

	groups := make([]pkg.ToolGroup, 0, len(tiers))
	for _, tier := range tiers {
		var tools []pkg.ToolDef
		var groupKeywords []string
		seen := make(map[string]bool)

		for name, h := range r.handlers {
			if r.tiers[name] != tier {
				continue
			}
			var def pkg.ToolDef
			if d, ok := h.(Definer); ok {
				def = d.Define()
			} else {
				def = pkg.ToolDef{Name: name}
			}
			tools = append(tools, def)

			for _, kw := range r.keywords[name] {
				if !seen[kw] {
					seen[kw] = true
					groupKeywords = append(groupKeywords, kw)
				}
			}
		}

		// Stable sort within tier for deterministic output.
		sort.Slice(tools, func(i, j int) bool {
			return tools[i].Name < tools[j].Name
		})
		sort.Strings(groupKeywords)

		groups = append(groups, pkg.ToolGroup{
			Name:     tierName(tier),
			Tier:     tier,
			Keywords: groupKeywords,
			Tools:    tools,
		})
	}
	return groups
}

func tierName(tier int) string {
	switch tier {
	case 1:
		return "core"
	case 2:
		return "extended"
	case 3:
		return "experimental"
	default:
		return "tier-" + string(rune('0'+tier))
	}
}
