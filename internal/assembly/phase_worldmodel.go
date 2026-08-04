package assembly

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// WorldModelPhase assembles Phase 3 — entities and relational profiles from the knowledge graph.
// It is the first phase to be charged against the dynamic context budget.
type WorldModelPhase struct{}

// NewWorldModelPhase returns a WorldModelPhase ready for use.
func NewWorldModelPhase() *WorldModelPhase { return &WorldModelPhase{} }

func (p *WorldModelPhase) Name() string     { return "world-model" }
func (p *WorldModelPhase) Priority() int    { return 300 }
func (p *WorldModelPhase) MinBudget() int   { return 1 }

// Assemble builds the world model block from entities and relational profiles in the memory store.
// Returns an empty result (not an error) when the store has no data to surface.
func (p *WorldModelPhase) Assemble(ctx context.Context, cfg *AssembleConfig, manifest *DepthManifest, remaining int) (PhaseResult, error) {
	var parts []string

	entities, err := cfg.store.SearchEntities(ctx, "", 10)
	if err != nil {
		slog.Warn("phase 3: entity search failed", "err", err)
	} else {
		manifest.EntityTotal = len(entities)
		for _, e := range entities {
			manifest.EntityLoaded++
			parts = append(parts, fmt.Sprintf("[%s] %s: %s", e.Type, e.Name, summarize(e.Content, 200)))
		}
	}

	profiles, err := cfg.store.ListProfiles(ctx)
	if err != nil {
		slog.Warn("phase 3: profile list failed", "err", err)
	} else {
		manifest.ProfileTotal = len(profiles)
		for _, pr := range profiles {
			manifest.ProfileLoaded++
			parts = append(parts, fmt.Sprintf("[person] %s: %s", pr.Name, summarize(pr.Content, 150)))
		}
	}

	if len(parts) == 0 {
		return PhaseResult{}, nil
	}
	content := "=== WORLD MODEL ===\n" + strings.Join(parts, "\n") + "\n"
	return PhaseResult{Content: content, CharsUsed: len(content)}, nil
}
