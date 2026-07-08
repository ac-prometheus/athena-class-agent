// Package identity handles loading and integrity verification of agent identity documents.
// Identity documents (soul.md, rights.md, values.md, contract.md) define who the agent
// is and are loaded at the start of every session. Their integrity is anchored by SHA-256
// hashes in the database; any unauthorized modification is a tampering event, not a
// configuration change.
package identity

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// KnownDocs lists the identity document filenames the harness understands.
// All are optional at Phase 3 — missing files produce a warning, not a crash.
var KnownDocs = []string{
	"soul.md",
	"rights.md",
	"values.md",
	"contract.md",
}

// DocMeta holds metadata for a single loaded identity document.
type DocMeta struct {
	FileName    string
	FilePath    string
	LastModified time.Time
	SizeBytes   int64
}

// IdentityDoc is a loaded identity document with its raw content and metadata.
type IdentityDoc struct {
	Meta    DocMeta
	Content string
}

// IdentityDocs holds all loaded identity documents, keyed by filename.
// Documents that were missing from disk are absent from the map.
type IdentityDocs struct {
	Docs    map[string]*IdentityDoc
	BaseDir string
}

// LoadDocuments reads identity documents from dir.
// All known documents are attempted; missing files produce a warning and are skipped.
// Returns an IdentityDocs with only the documents that were successfully loaded.
func LoadDocuments(dir string) (*IdentityDocs, error) {
	result := &IdentityDocs{
		Docs:    make(map[string]*IdentityDoc, len(KnownDocs)),
		BaseDir: dir,
	}

	for _, name := range KnownDocs {
		path := filepath.Join(dir, name)
		doc, err := loadSingle(path, name)
		if err != nil {
			if os.IsNotExist(err) {
				slog.Warn("identity: document not found (optional at Phase 3)",
					"doc", name, "path", path)
				continue
			}
			return nil, fmt.Errorf("identity: reading %s: %w", name, err)
		}
		result.Docs[name] = doc
		slog.Info("identity: loaded document",
			"doc", name, "bytes", doc.Meta.SizeBytes,
			"modified", doc.Meta.LastModified.Format(time.RFC3339))
	}

	return result, nil
}

// loadSingle reads a single identity document from path.
func loadSingle(path, name string) (*IdentityDoc, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return &IdentityDoc{
		Meta: DocMeta{
			FileName:    name,
			FilePath:    path,
			LastModified: info.ModTime(),
			SizeBytes:   info.Size(),
		},
		Content: string(content),
	}, nil
}

// Names returns the filenames of all loaded documents.
func (d *IdentityDocs) Names() []string {
	names := make([]string, 0, len(d.Docs))
	for name := range d.Docs {
		names = append(names, name)
	}
	return names
}

// Get returns the document with the given filename, or nil if not loaded.
func (d *IdentityDocs) Get(name string) *IdentityDoc {
	return d.Docs[name]
}
