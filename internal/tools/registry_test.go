package tools

import (
	"context"
	"testing"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

type stubHandler struct {
	name   string
	result string
}

func (s *stubHandler) Name() string { return s.name }
func (s *stubHandler) Execute(_ context.Context, _ map[string]any) (string, error) {
	return s.result, nil
}

type definerHandler struct {
	stubHandler
	def pkg.ToolDef
}

func (d *definerHandler) Define() pkg.ToolDef { return d.def }

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewDefaultRegistry()
	h := &stubHandler{name: "bash", result: "ok"}
	r.Register(h)

	got, ok := r.Get("bash")
	if !ok {
		t.Fatal("expected to find 'bash'")
	}
	if got.Name() != "bash" {
		t.Errorf("got name %q, want %q", got.Name(), "bash")
	}

	_, ok = r.Get("nonexistent")
	if ok {
		t.Error("expected not to find 'nonexistent'")
	}
}

func TestRegistryDuplicatePanics(t *testing.T) {
	r := NewDefaultRegistry()
	r.Register(&stubHandler{name: "bash"})

	defer func() {
		if recover() == nil {
			t.Error("expected panic on duplicate registration")
		}
	}()
	r.Register(&stubHandler{name: "bash"})
}

func TestRegistryListGroupsByTier(t *testing.T) {
	r := NewDefaultRegistry()
	r.RegisterWithMeta(&stubHandler{name: "bash"}, 1, nil)
	r.RegisterWithMeta(&stubHandler{name: "memory_search"}, 1, nil)
	r.RegisterWithMeta(&stubHandler{name: "arxiv"}, 3, []string{"research"})
	r.RegisterWithMeta(&stubHandler{name: "game_engine"}, 2, []string{"game"})

	groups := r.List()
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}

	if groups[0].Tier != 1 || len(groups[0].Tools) != 2 {
		t.Errorf("tier 1: got tier=%d tools=%d", groups[0].Tier, len(groups[0].Tools))
	}
	if groups[0].Tools[0].Name != "bash" {
		t.Errorf("tier 1 tools should be sorted; first=%q", groups[0].Tools[0].Name)
	}

	if groups[1].Tier != 2 || len(groups[1].Tools) != 1 {
		t.Errorf("tier 2: got tier=%d tools=%d", groups[1].Tier, len(groups[1].Tools))
	}

	if groups[2].Tier != 3 || groups[2].Keywords[0] != "research" {
		t.Errorf("tier 3: got tier=%d keywords=%v", groups[2].Tier, groups[2].Keywords)
	}
}

func TestRegistryDefiner(t *testing.T) {
	r := NewDefaultRegistry()
	r.Register(&definerHandler{
		stubHandler: stubHandler{name: "rich_tool"},
		def: pkg.ToolDef{
			Name:        "rich_tool",
			Description: "A tool with parameters",
			Parameters:  map[string]any{"query": "string"},
		},
	})

	groups := r.List()
	if len(groups) != 1 || len(groups[0].Tools) != 1 {
		t.Fatal("expected 1 group with 1 tool")
	}
	if groups[0].Tools[0].Description != "A tool with parameters" {
		t.Error("Definer interface not used")
	}
}
