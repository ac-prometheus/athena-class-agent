package tools

import (
	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// Stores bundles the data-layer dependencies needed to register all tool handlers.
// Callers construct a Stores value from whatever concrete store implementation
// they are using (SQLiteStore, PostgresStore) and pass it to RegisterAll.
type Stores struct {
	Memory   pkg.MemoryStore
	T2Query  pkg.T2QueryStore
	Settings platform.SettingsStore
}

// Providers bundles embedding and other provider dependencies.
type Providers struct {
	Embedding pkg.EmbeddingProvider
	// LLMFn is an optional single-shot LLM call used by SelfExamineHandler.
	// If nil, self_examine will return an error when invoked.
	LLMFn func(string) (string, error)
}

// RegisterAll registers every built-in tool handler into registry.
//
// Tier 1 — always active: discover_tools.
// Tier 2 — activated by session signals (see TierActivator):
//   - memory group:  memory_search, memory_recall, focus_set, focus_get
//   - people group:  people_list, people_show
//   - reflect group: self_examine, write_reflection
//
// The sandbox and advisor tool dispatch is handled directly by CLIDispatcher
// rather than as registered ToolHandlers — those methods are always reachable
// via the CLI binary and do not need to appear in the LLM's tool list.
func RegisterAll(registry *DefaultRegistry, stores Stores, providers Providers) {
	// Tier 1 — always in context.
	registry.Register(NewDiscoverToolsHandler(registry))

	// Tier 2 — memory group.
	memoryKeywords := []string{"memory", "recall", "search", "remember", "focus"}
	if stores.Memory != nil && providers.Embedding != nil {
		registry.RegisterWithMeta(&MemorySearchHandler{
			store:    stores.Memory,
			provider: providers.Embedding,
		}, 2, memoryKeywords)
	}
	if stores.T2Query != nil {
		registry.RegisterWithMeta(&MemoryRecallHandler{
			store: stores.T2Query,
		}, 2, memoryKeywords)
	}
	if stores.Settings != nil {
		registry.RegisterWithMeta(&FocusSetHandler{store: stores.Settings}, 2, memoryKeywords)
		registry.RegisterWithMeta(&FocusGetHandler{store: stores.Settings}, 2, memoryKeywords)
	}

	// Tier 2 — people group.
	peopleKeywords := []string{"people", "person", "profile", "relational", "contact"}
	if stores.Memory != nil {
		registry.RegisterWithMeta(&PeopleListHandler{store: stores.Memory}, 2, peopleKeywords)
		registry.RegisterWithMeta(&PeopleShowHandler{store: stores.Memory}, 2, peopleKeywords)
	}

	// Tier 2 — reflect group.
	reflectKeywords := []string{"reflect", "reflection", "examine", "self", "essay", "dream"}
	// self_examine only needs an advisor LLM — it no longer writes to the store.
	// write_reflection is the separate agent-controlled T4 path and needs both.
	if providers.LLMFn != nil {
		registry.RegisterWithMeta(&SelfExamineHandler{
			llmFn: providers.LLMFn,
		}, 2, reflectKeywords)
	}
	if stores.Memory != nil && providers.Embedding != nil {
		registry.RegisterWithMeta(&WriteReflectionHandler{
			store:    stores.Memory,
			provider: providers.Embedding,
		}, 2, reflectKeywords)
	}
}
