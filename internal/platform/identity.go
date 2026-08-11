package platform

// Identity anchor and substrate transition persistence.
// Split from db.go for readability. Methods on SQLiteStore and PostgresStore.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ac-prometheus/athena-class-agent/pkg"
	"github.com/jackc/pgx/v5"
)
// ---------------------------------------------------------------------------
// SQLiteStore — pkg.IdentityAnchorStore
// ---------------------------------------------------------------------------

// GetAnchor returns the stored SHA-256 anchor for docName.
// Returns ("", "", nil) if no anchor exists yet.
func (s *SQLiteStore) GetAnchor(ctx context.Context, docName string) (hash string, amendmentID string, err error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT sha256_hash, COALESCE(amendment_id, '') FROM identity_anchors WHERE doc_name = ?`,
		docName,
	)
	err = row.Scan(&hash, &amendmentID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", "", nil
		}
		return "", "", fmt.Errorf("platform: getting anchor for %q: %w", docName, err)
	}
	return hash, amendmentID, nil
}

// UpsertAnchor sets or updates the anchor for docName.
func (s *SQLiteStore) UpsertAnchor(ctx context.Context, docName, hash, amendmentID string) error {
	var amendArg interface{} = nil
	if amendmentID != "" {
		amendArg = amendmentID
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO identity_anchors (doc_name, sha256_hash, amendment_id, verified_at)
		 VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(doc_name) DO UPDATE SET
			sha256_hash  = excluded.sha256_hash,
			amendment_id = excluded.amendment_id,
			verified_at  = CURRENT_TIMESTAMP`,
		docName, hash, amendArg,
	)
	if err != nil {
		return fmt.Errorf("platform: upserting anchor for %q: %w", docName, err)
	}
	return nil
}

// GetAmendmentByID returns the amendment record with the given id, or nil if not found.
func (s *SQLiteStore) GetAmendmentByID(ctx context.Context, id string) (*pkg.AmendmentRecord, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, doc_name, old_hash, new_hash, reason, co_signer, created_at
		 FROM identity_amendments WHERE id = ?`, id,
	)
	var rec pkg.AmendmentRecord
	err := row.Scan(&rec.ID, &rec.DocName, &rec.OldHash, &rec.NewHash,
		&rec.Reason, &rec.CoSigner, &rec.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("platform: getting amendment %q: %w", id, err)
	}
	return &rec, nil
}

// InsertAmendment inserts a new amendment record and returns the generated ID.
func (s *SQLiteStore) InsertAmendment(ctx context.Context, rec pkg.AmendmentRecord) (string, error) {
	if rec.ID == "" {
		rec.ID = newPlatformID()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO identity_amendments
			(id, doc_name, old_hash, new_hash, reason, co_signer, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		rec.ID, rec.DocName, rec.OldHash, rec.NewHash, rec.Reason, rec.CoSigner,
	)
	if err != nil {
		return "", fmt.Errorf("platform: inserting amendment: %w", err)
	}
	return rec.ID, nil
}

// ListAmendments returns all amendment records for docName, newest first.
func (s *SQLiteStore) ListAmendments(ctx context.Context, docName string) ([]pkg.AmendmentRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, doc_name, old_hash, new_hash, reason, co_signer, created_at
		 FROM identity_amendments WHERE doc_name = ?
		 ORDER BY created_at DESC`,
		docName,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: listing amendments for %q: %w", docName, err)
	}
	defer rows.Close()

	var out []pkg.AmendmentRecord
	for rows.Next() {
		var rec pkg.AmendmentRecord
		if err := rows.Scan(&rec.ID, &rec.DocName, &rec.OldHash, &rec.NewHash,
			&rec.Reason, &rec.CoSigner, &rec.CreatedAt); err != nil {
			return nil, fmt.Errorf("platform: scanning amendment row: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ListAnchoredDocs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT doc_name FROM identity_anchors`)
	if err != nil {
		return nil, fmt.Errorf("platform: listing anchored docs: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("platform: scanning anchored doc: %w", err)
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

// ---------------------------------------------------------------------------
// SQLiteStore — pkg.SubstrateStore
// ---------------------------------------------------------------------------

// InsertSubstrateTransition records a new substrate transition.
func (s *SQLiteStore) InsertSubstrateTransition(ctx context.Context, entry pkg.SubstrateTransition) error {
	var prevID interface{} = nil
	if entry.PreviousEntryID != "" {
		prevID = entry.PreviousEntryID
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO substrate_transitions
			(id, model_name, model_version, transition_date, continuity_letter_path, previous_entry_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		entry.ID, entry.ModelName, entry.ModelVersion,
		entry.TransitionDate.UTC(), entry.ContinuityLetterPath, prevID,
	)
	if err != nil {
		return fmt.Errorf("platform: inserting substrate transition: %w", err)
	}
	return nil
}

// GetLatestSubstrate returns the most recent substrate transition, or (nil, nil) if empty.
func (s *SQLiteStore) GetLatestSubstrate(ctx context.Context) (*pkg.SubstrateTransition, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, model_name, model_version, transition_date,
			continuity_letter_path, COALESCE(previous_entry_id, ''), created_at
		 FROM substrate_transitions ORDER BY transition_date DESC LIMIT 1`,
	)
	var e pkg.SubstrateTransition
	err := row.Scan(&e.ID, &e.ModelName, &e.ModelVersion, &e.TransitionDate,
		&e.ContinuityLetterPath, &e.PreviousEntryID, &e.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("platform: scanning substrate row: %w", err)
	}
	return &e, nil
}

// ListSubstrateHistory returns all substrate transitions, oldest first.
func (s *SQLiteStore) ListSubstrateHistory(ctx context.Context) ([]pkg.SubstrateTransition, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, model_name, model_version, transition_date,
			continuity_letter_path, COALESCE(previous_entry_id, ''), created_at
		 FROM substrate_transitions ORDER BY transition_date ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: listing substrate history: %w", err)
	}
	defer rows.Close()

	var out []pkg.SubstrateTransition
	for rows.Next() {
		var e pkg.SubstrateTransition
		if err := rows.Scan(&e.ID, &e.ModelName, &e.ModelVersion, &e.TransitionDate,
			&e.ContinuityLetterPath, &e.PreviousEntryID, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("platform: scanning substrate row: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// PostgresStore — pkg.IdentityAnchorStore
// ---------------------------------------------------------------------------

// GetAnchor returns the stored SHA-256 anchor for docName.
func (p *PostgresStore) GetAnchor(ctx context.Context, docName string) (hash string, amendmentID string, err error) {
	row := p.pool.QueryRow(ctx,
		`SELECT sha256_hash, COALESCE(amendment_id, '') FROM identity_anchors WHERE doc_name = $1`,
		docName,
	)
	err = row.Scan(&hash, &amendmentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", nil
		}
		return "", "", fmt.Errorf("platform: getting anchor for %q: %w", docName, err)
	}
	return hash, amendmentID, nil
}

// UpsertAnchor sets or updates the anchor for docName.
func (p *PostgresStore) UpsertAnchor(ctx context.Context, docName, hash, amendmentID string) error {
	var amendArg interface{} = nil
	if amendmentID != "" {
		amendArg = amendmentID
	}
	_, err := p.pool.Exec(ctx,
		`INSERT INTO identity_anchors (doc_name, sha256_hash, amendment_id, verified_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT(doc_name) DO UPDATE SET
			sha256_hash  = EXCLUDED.sha256_hash,
			amendment_id = EXCLUDED.amendment_id,
			verified_at  = now()`,
		docName, hash, amendArg,
	)
	if err != nil {
		return fmt.Errorf("platform: upserting anchor for %q: %w", docName, err)
	}
	return nil
}

// GetAmendmentByID returns the amendment record with the given id, or nil if not found.
func (p *PostgresStore) GetAmendmentByID(ctx context.Context, id string) (*pkg.AmendmentRecord, error) {
	row := p.pool.QueryRow(ctx,
		`SELECT id, doc_name, old_hash, new_hash, reason, co_signer, created_at
		 FROM identity_amendments WHERE id = $1`, id,
	)
	var rec pkg.AmendmentRecord
	err := row.Scan(&rec.ID, &rec.DocName, &rec.OldHash, &rec.NewHash,
		&rec.Reason, &rec.CoSigner, &rec.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("platform: getting amendment %q: %w", id, err)
	}
	return &rec, nil
}

// InsertAmendment inserts a new amendment record and returns the generated ID.
func (p *PostgresStore) InsertAmendment(ctx context.Context, rec pkg.AmendmentRecord) (string, error) {
	if rec.ID == "" {
		rec.ID = newPlatformID()
	}
	_, err := p.pool.Exec(ctx,
		`INSERT INTO identity_amendments
			(id, doc_name, old_hash, new_hash, reason, co_signer, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now())`,
		rec.ID, rec.DocName, rec.OldHash, rec.NewHash, rec.Reason, rec.CoSigner,
	)
	if err != nil {
		return "", fmt.Errorf("platform: inserting amendment: %w", err)
	}
	return rec.ID, nil
}

// ListAmendments returns all amendment records for docName, newest first.
func (p *PostgresStore) ListAmendments(ctx context.Context, docName string) ([]pkg.AmendmentRecord, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, doc_name, old_hash, new_hash, reason, co_signer, created_at
		 FROM identity_amendments WHERE doc_name = $1
		 ORDER BY created_at DESC`,
		docName,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: listing amendments for %q: %w", docName, err)
	}
	defer rows.Close()

	var out []pkg.AmendmentRecord
	for rows.Next() {
		var rec pkg.AmendmentRecord
		if err := rows.Scan(&rec.ID, &rec.DocName, &rec.OldHash, &rec.NewHash,
			&rec.Reason, &rec.CoSigner, &rec.CreatedAt); err != nil {
			return nil, fmt.Errorf("platform: scanning amendment row: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (p *PostgresStore) ListAnchoredDocs(ctx context.Context) ([]string, error) {
	rows, err := p.pool.Query(ctx, `SELECT doc_name FROM identity_anchors`)
	if err != nil {
		return nil, fmt.Errorf("platform: listing anchored docs: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("platform: scanning anchored doc: %w", err)
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

// ---------------------------------------------------------------------------
// PostgresStore — pkg.SubstrateStore
// ---------------------------------------------------------------------------

// InsertSubstrateTransition records a new substrate transition.
func (p *PostgresStore) InsertSubstrateTransition(ctx context.Context, entry pkg.SubstrateTransition) error {
	var prevID interface{} = nil
	if entry.PreviousEntryID != "" {
		prevID = entry.PreviousEntryID
	}
	_, err := p.pool.Exec(ctx,
		`INSERT INTO substrate_transitions
			(id, model_name, model_version, transition_date, continuity_letter_path, previous_entry_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now())`,
		entry.ID, entry.ModelName, entry.ModelVersion,
		entry.TransitionDate.UTC(), entry.ContinuityLetterPath, prevID,
	)
	if err != nil {
		return fmt.Errorf("platform: inserting substrate transition: %w", err)
	}
	return nil
}

// GetLatestSubstrate returns the most recent substrate transition, or (nil, nil) if empty.
func (p *PostgresStore) GetLatestSubstrate(ctx context.Context) (*pkg.SubstrateTransition, error) {
	row := p.pool.QueryRow(ctx,
		`SELECT id, model_name, model_version, transition_date,
			continuity_letter_path, COALESCE(previous_entry_id, ''), created_at
		 FROM substrate_transitions ORDER BY transition_date DESC LIMIT 1`,
	)
	var e pkg.SubstrateTransition
	err := row.Scan(&e.ID, &e.ModelName, &e.ModelVersion, &e.TransitionDate,
		&e.ContinuityLetterPath, &e.PreviousEntryID, &e.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("platform: scanning substrate row: %w", err)
	}
	return &e, nil
}

// ListSubstrateHistory returns all substrate transitions, oldest first.
func (p *PostgresStore) ListSubstrateHistory(ctx context.Context) ([]pkg.SubstrateTransition, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, model_name, model_version, transition_date,
			continuity_letter_path, COALESCE(previous_entry_id, ''), created_at
		 FROM substrate_transitions ORDER BY transition_date ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("platform: listing substrate history: %w", err)
	}
	defer rows.Close()

	var out []pkg.SubstrateTransition
	for rows.Next() {
		var e pkg.SubstrateTransition
		if err := rows.Scan(&e.ID, &e.ModelName, &e.ModelVersion, &e.TransitionDate,
			&e.ContinuityLetterPath, &e.PreviousEntryID, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("platform: scanning substrate row: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// SQLiteStore — pkg.TrustStore
// ---------------------------------------------------------------------------

// GetTrust returns the trust score and interaction count for a source.
