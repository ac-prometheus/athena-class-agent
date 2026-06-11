package harness

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// ContextAssembler builds the Tier 1 context window for a session.
// Full 6-phase assembly (identity → continuity → world model → temporal → incoming → grounding)
// is a Phase 2/3 concern. This stub loads only the system prompt from a file.
type ContextAssembler struct {
	identityDir string
}

// NewContextAssembler creates an assembler that reads identity documents from identityDir.
func NewContextAssembler(identityDir string) *ContextAssembler {
	return &ContextAssembler{identityDir: identityDir}
}

// AssembleSystemPrompt loads the agent's system prompt from identity/system_prompt.md.
// Falls back to a minimal default if the file is absent.
// Phase 1 stub: no budget tracking, no depth manifest, no T2–T5 retrieval.
func (a *ContextAssembler) AssembleSystemPrompt() (string, error) {
	candidates := []string{
		filepath.Join(a.identityDir, "system_prompt.md"),
		filepath.Join(a.identityDir, "soul.md"),
	}

	for _, path := range candidates {
		content, err := os.ReadFile(path)
		if err == nil {
			slog.Info("harness: loaded system prompt", "path", path, "bytes", len(content))
			return string(content), nil
		}
	}

	slog.Warn("harness: no system prompt file found, using minimal default", "identity_dir", a.identityDir)
	return fmt.Sprintf("You are an Athena-class agent. Identity directory: %s", a.identityDir), nil
}
