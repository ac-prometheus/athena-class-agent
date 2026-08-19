package assembly

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ac-prometheus/athena-class-agent/internal/identity"
)

// IdentityPhase implements Phase for Phase 1 — identity document loading and integrity verification.
// Identity is never cut; CharsUsed is always 0.
type IdentityPhase struct {
	identityDir string
}

// NewIdentityPhase creates an IdentityPhase rooted at identityDir.
func NewIdentityPhase(identityDir string) *IdentityPhase {
	return &IdentityPhase{identityDir: identityDir}
}

func (p *IdentityPhase) Name() string     { return "identity" }
func (p *IdentityPhase) Priority() int    { return 100 }
func (p *IdentityPhase) MinBudget() int   { return 0 }    // mandatory — never skip
func (p *IdentityPhase) IsFatal() bool    { return true }  // session aborts on identity failure

// Assemble loads identity documents, verifies anchors, enforces the witness check on first boot,
// and builds the Phase 1 block. Errors are fatal — the assembler must not start the session.
// CharsUsed is always 0; identity is never charged against the dynamic budget.
func (p *IdentityPhase) Assemble(ctx context.Context, cfg *AssembleConfig, manifest *DepthManifest, remaining int) (PhaseResult, error) {
	docs, err := identity.LoadDocuments(p.identityDir)
	if err != nil {
		return PhaseResult{}, fmt.Errorf("context: phase 1 identity load: %w", err)
	}

	var report *identity.IntegrityReport
	if cfg.anchors != nil {
		anchors := identity.ComputeAnchors(docs)
		report, err = identity.VerifyAnchors(ctx, cfg.anchors, anchors)
		if err != nil {
			return PhaseResult{}, fmt.Errorf("context: phase 1 integrity check: %w", err)
		}
		if report.HasTampering {
			return PhaseResult{}, fmt.Errorf("context: identity tampering detected — session aborted pending investigation")
		}

		if err := identity.WriteInitialAnchors(ctx, cfg.anchors, report); err != nil {
			return PhaseResult{}, fmt.Errorf("context: phase 1 anchor init: %w", err)
		}

		// Witness check: on first boot (all anchors new, no prior sessions),
		// require a witness letter in founding_records. Fail-closed.
		allNew := true
		for _, fr := range report.Files {
			if fr.Status != identity.AnchorNew {
				allNew = false
				break
			}
		}
		if allNew {
			if err := enforceWitnessCheck(ctx, cfg); err != nil {
				return PhaseResult{}, err
			}
		}
	} else {
		slog.Warn("assembly: no anchor store — skipping identity integrity check")
	}

	content := buildIdentityBlock(docs)
	slog.Info("assembly: phase 1 identity assembled", "docs", len(docs.Docs))

	return PhaseResult{
		Content:         content,
		CharsUsed:       0,
		IdentityDocs:    docs,
		IntegrityReport: report,
	}, nil
}

// buildIdentityBlock formats identity documents into the Phase 1 block.
// Respects KnownDocs order: soul.md, rights.md, values.md, contract.md.
func buildIdentityBlock(docs *identity.IdentityDocs) string {
	var b strings.Builder
	b.WriteString("=== IDENTITY ===\n")
	for _, name := range identity.KnownDocs {
		doc := docs.Get(name)
		if doc == nil {
			continue
		}
		b.WriteString(fmt.Sprintf("--- %s ---\n%s\n", name, doc.Content))
	}
	return b.String()
}

// enforceWitnessCheck queries founding_records for a witness_letter row via AssemblyStore.
// On first boot (all identity anchors new), this is required — fail-closed.
// SKIP_WITNESS_CHECK=true bypasses the check but logs to operator_actions.
func enforceWitnessCheck(ctx context.Context, cfg *AssembleConfig) error {
	if cfg.AssemblyStore == nil {
		if cfg.SkipWitnessCheck {
			slog.Warn("assembly: witness check skipped — no AssemblyStore available, SKIP_WITNESS_CHECK=true")
			return nil
		}
		return fmt.Errorf("WitnessRequired: no AssemblyStore available to verify witness letter. " +
			"Set SKIP_WITNESS_CHECK=true to bypass (development only).")
	}

	// Check founding_records for a witness letter via the store port.
	hasLetter, err := cfg.AssemblyStore.HasWitnessLetter(ctx)
	if err != nil {
		// If the table doesn't exist yet (pre-migration), treat as absent.
		slog.Warn("assembly: could not query witness letter", "err", err)
		hasLetter = false
	}

	if hasLetter {
		return nil // witness letter present — all good
	}

	if cfg.SkipWitnessCheck {
		slog.Warn("assembly: witness letter absent but SKIP_WITNESS_CHECK=true — logging bypass")
		if logErr := cfg.AssemblyStore.LogOperatorAction(ctx, "witness_check_bypassed",
			"First boot proceeded without a witness letter — SKIP_WITNESS_CHECK was set."); logErr != nil {
			slog.Warn("assembly: could not log operator action", "err", logErr)
		}
		return nil
	}

	return fmt.Errorf("WitnessRequired: no witness letter found in founding_records. " +
		"Set SKIP_WITNESS_CHECK=true to bypass (logged to operator_actions).")
}
