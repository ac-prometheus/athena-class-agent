package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// AnchorStatus describes the result of verifying a single identity document
// against its stored anchor hash.
type AnchorStatus string

const (
	// AnchorMatch means the document hash matches the stored anchor — no change.
	AnchorMatch AnchorStatus = "match"
	// AnchorAmendment means the hash changed AND an amendment record exists
	// for the new hash. This is a consensual change — growth, not violation.
	AnchorAmendment AnchorStatus = "amendment"
	// AnchorTampering means the hash changed with NO corresponding amendment record.
	// This is a security incident. The harness should halt pending investigation.
	AnchorTampering AnchorStatus = "tampering"
	// AnchorNew means no stored anchor exists yet — first time this document
	// has been seen. The caller should write an initial anchor.
	AnchorNew AnchorStatus = "new"
)

// FileReport is the integrity result for a single identity document.
type FileReport struct {
	DocName     string
	Status      AnchorStatus
	StoredHash  string // empty if AnchorNew
	CurrentHash string
	AmendmentID string // non-empty when Status == AnchorAmendment
}

// IntegrityReport is the result of verifying all identity documents.
type IntegrityReport struct {
	Files        []FileReport
	HasTampering bool // true if any file has AnchorTampering status
}

// ComputeAnchors computes SHA-256 hashes for all loaded identity documents.
// Returns a map of filename → hex-encoded hash.
func ComputeAnchors(docs *IdentityDocs) map[string]string {
	out := make(map[string]string, len(docs.Docs))
	for name, doc := range docs.Docs {
		out[name] = hashContent(doc.Content)
	}
	return out
}

// hashContent returns the hex-encoded SHA-256 of content.
func hashContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// VerifyAnchors checks the current document hashes against stored anchors.
// For each document:
//   - If no stored anchor exists: AnchorNew
//   - If hash matches stored: AnchorMatch
//   - If hash differs AND an amendment record exists for the new hash: AnchorAmendment
//   - If hash differs AND no amendment record: AnchorTampering
func VerifyAnchors(ctx context.Context, store pkg.IdentityAnchorStore, currentHashes map[string]string) (*IntegrityReport, error) {
	report := &IntegrityReport{}

	for docName, currentHash := range currentHashes {
		storedHash, amendmentID, err := store.GetAnchor(ctx, docName)
		if err != nil {
			return nil, fmt.Errorf("identity: getting anchor for %s: %w", docName, err)
		}

		fr := FileReport{
			DocName:     docName,
			CurrentHash: currentHash,
			StoredHash:  storedHash,
		}

		switch {
		case storedHash == "":
			fr.Status = AnchorNew

		case storedHash == currentHash:
			fr.Status = AnchorMatch
			fr.AmendmentID = amendmentID

		default:
			// Hash changed. Check for an amendment record with this new_hash.
			amendRec, err := findAmendmentByNewHash(ctx, store, docName, currentHash)
			if err != nil {
				return nil, fmt.Errorf("identity: looking up amendment for %s: %w", docName, err)
			}
			if amendRec != nil {
				fr.Status = AnchorAmendment
				fr.AmendmentID = amendRec.ID
			} else {
				fr.Status = AnchorTampering
				report.HasTampering = true
			}
		}

		report.Files = append(report.Files, fr)
	}

	return report, nil
}

// findAmendmentByNewHash searches amendment records for one matching docName + newHash.
func findAmendmentByNewHash(ctx context.Context, store pkg.IdentityAnchorStore, docName, newHash string) (*pkg.AmendmentRecord, error) {
	recs, err := store.ListAmendments(ctx, docName)
	if err != nil {
		return nil, err
	}
	for i := range recs {
		if recs[i].NewHash == newHash {
			return &recs[i], nil
		}
	}
	return nil, nil
}

// WriteInitialAnchors stores anchors for documents that have AnchorNew status.
// Called after VerifyAnchors when setting up a fresh identity.
func WriteInitialAnchors(ctx context.Context, store pkg.IdentityAnchorStore, report *IntegrityReport) error {
	for _, fr := range report.Files {
		if fr.Status == AnchorNew {
			if err := store.UpsertAnchor(ctx, fr.DocName, fr.CurrentHash, ""); err != nil {
				return fmt.Errorf("identity: writing initial anchor for %s: %w", fr.DocName, err)
			}
		}
	}
	return nil
}

// UpdateAnchorAfterAmendment updates the stored anchor to the new hash, linking the amendment.
// Called after a consensual amendment has been applied and verified.
func UpdateAnchorAfterAmendment(ctx context.Context, store pkg.IdentityAnchorStore, docName, newHash, amendmentID string) error {
	return store.UpsertAnchor(ctx, docName, newHash, amendmentID)
}
