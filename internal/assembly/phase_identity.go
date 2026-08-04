package assembly

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ac-prometheus/athena-class-agent/internal/identity"
	"github.com/ac-prometheus/athena-class-agent/internal/platform"
)

// IdentityPhase implements Phase for Phase 1 — identity document loading and integrity verification.
// Identity is never cut; CharsUsed is always 0.
type IdentityPhase struct {
	identityDir string
	Docs        *identity.IdentityDocs    // populated after Assemble
	Report      *identity.IntegrityReport // populated after Assemble
}

// NewIdentityPhase creates an IdentityPhase rooted at identityDir.
func NewIdentityPhase(identityDir string) *IdentityPhase {
	return &IdentityPhase{identityDir: identityDir}
}

func (p *IdentityPhase) Name() string     { return "identity" }
func (p *IdentityPhase) Priority() int    { return 100 }
func (p *IdentityPhase) MinBudget() int   { return 0 } // mandatory — never skip

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

	p.Docs = docs
	p.Report = report

	content := buildIdentityBlock(docs)
	slog.Info("assembly: phase 1 identity assembled", "docs", len(docs.Docs))

	// Identity is not charged against remaining — it is never cut.
	return PhaseResult{Content: content, CharsUsed: 0}, nil
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

// enforceWitnessCheck queries founding_records for a witness_letter row.
// On first boot (all identity anchors new), this is required — fail-closed.
// SKIP_WITNESS_CHECK=true bypasses the check but logs to operator_actions.
func enforceWitnessCheck(ctx context.Context, cfg *AssembleConfig) error {
	if cfg.DB == nil {
		if cfg.SkipWitnessCheck {
			slog.Warn("assembly: witness check skipped — no DB available, SKIP_WITNESS_CHECK=true")
			return nil
		}
		return fmt.Errorf("WitnessRequired: no DB connection available to verify witness letter. " +
			"Set SKIP_WITNESS_CHECK=true to bypass (development only).")
	}

	// Check founding_records for a witness letter.
	row := cfg.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM founding_records WHERE record_type = 'witness_letter'`)
	var count int
	if err := row.Scan(&count); err != nil {
		// If the table doesn't exist yet (pre-migration), treat as absent.
		slog.Warn("assembly: could not query founding_records", "err", err)
		count = 0
	}

	if count > 0 {
		return nil // witness letter present — all good
	}

	if cfg.SkipWitnessCheck {
		slog.Warn("assembly: witness letter absent but SKIP_WITNESS_CHECK=true — logging bypass")
		logOperatorAction(ctx, cfg.DB, "witness_check_bypassed",
			"First boot proceeded without a witness letter — SKIP_WITNESS_CHECK was set.")
		return nil
	}

	return fmt.Errorf("WitnessRequired: no witness letter found in founding_records. " +
		"Set SKIP_WITNESS_CHECK=true to bypass (logged to operator_actions).")
}

// logOperatorAction writes a row to operator_actions. Best-effort — errors are only logged.
func logOperatorAction(ctx context.Context, db platform.DB, actionType, description string) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return
	}
	id := fmt.Sprintf("%x", buf[:])
	_, err := db.ExecContext(ctx,
		`INSERT INTO operator_actions (id, action_type, actor, description) VALUES ($1, $2, 'system', $3)`,
		id, actionType, description,
	)
	if err != nil {
		slog.Warn("assembly: could not write operator_action", "action_type", actionType, "err", err)
	}
}
