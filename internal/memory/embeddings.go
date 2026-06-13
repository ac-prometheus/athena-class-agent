package memory

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// ---------------------------------------------------------------------------
// VoyageProvider — HTTP client to Voyage AI embedding API.
// ---------------------------------------------------------------------------

// VoyageProvider implements pkg.EmbeddingProvider using Voyage AI's REST API.
// Model "voyage-3.5" produces 1024-dimensional vectors (default).
type VoyageProvider struct {
	apiKey     string
	model      string
	dimensions int
	client     *http.Client
	endpoint   string
}

// NewVoyageProvider constructs a VoyageProvider.
// model: "voyage-3.5" (1024-dim) or "voyage-3-large" (1024-dim).
func NewVoyageProvider(apiKey, model string) *VoyageProvider {
	dims := 1024
	return &VoyageProvider{
		apiKey:     apiKey,
		model:      model,
		dimensions: dims,
		client:     &http.Client{Timeout: 30 * time.Second},
		endpoint:   "https://api.voyageai.com/v1/embeddings",
	}
}

type voyageRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type voyageResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Embed sends a single text to Voyage AI and returns its embedding vector.
func (v *VoyageProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(voyageRequest{Input: []string{text}, Model: v.model})
	if err != nil {
		return nil, fmt.Errorf("voyage: marshalling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("voyage: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+v.apiKey)

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("voyage: HTTP request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("voyage: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("voyage: HTTP %d: %s", resp.StatusCode, raw)
	}

	var vr voyageResponse
	if err := json.Unmarshal(raw, &vr); err != nil {
		return nil, fmt.Errorf("voyage: decoding response: %w", err)
	}
	if vr.Error != nil {
		return nil, fmt.Errorf("voyage: API error: %s", vr.Error.Message)
	}
	if len(vr.Data) == 0 {
		return nil, fmt.Errorf("voyage: empty data in response")
	}
	return vr.Data[0].Embedding, nil
}

// Dimensions returns the embedding dimensionality for this provider/model.
func (v *VoyageProvider) Dimensions() int { return v.dimensions }

// ---------------------------------------------------------------------------
// NomicProvider — HTTP client to Ollama (local nomic-embed-text).
// ---------------------------------------------------------------------------

// NomicProvider implements pkg.EmbeddingProvider using a local Ollama instance
// running the nomic-embed-text model (768-dimensional vectors).
type NomicProvider struct {
	endpoint   string
	model      string
	dimensions int
	client     *http.Client
}

// NewNomicProvider constructs a NomicProvider.
// endpoint: Ollama base URL, e.g. "http://localhost:11434".
// model: "nomic-embed-text" (768-dim) or "mxbai-embed-large" (1024-dim).
func NewNomicProvider(endpoint, model string) *NomicProvider {
	dims := 768
	if model == "mxbai-embed-large" {
		dims = 1024
	}
	return &NomicProvider{
		endpoint:   endpoint,
		model:      model,
		dimensions: dims,
		client:     &http.Client{Timeout: 60 * time.Second},
	}
}

type nomicRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type nomicResponse struct {
	Embedding []float32 `json:"embedding"`
	Error     string    `json:"error,omitempty"`
}

// Embed sends a single text to Ollama and returns the nomic embedding vector.
func (n *NomicProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(nomicRequest{Model: n.model, Prompt: text})
	if err != nil {
		return nil, fmt.Errorf("nomic: marshalling request: %w", err)
	}

	url := n.endpoint + "/api/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("nomic: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("nomic: HTTP request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("nomic: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nomic: HTTP %d: %s", resp.StatusCode, raw)
	}

	var nr nomicResponse
	if err := json.Unmarshal(raw, &nr); err != nil {
		return nil, fmt.Errorf("nomic: decoding response: %w", err)
	}
	if nr.Error != "" {
		return nil, fmt.Errorf("nomic: API error: %s", nr.Error)
	}
	return nr.Embedding, nil
}

// Dimensions returns the embedding dimensionality.
func (n *NomicProvider) Dimensions() int { return n.dimensions }

// ---------------------------------------------------------------------------
// Background embedding goroutine
// ---------------------------------------------------------------------------

// EmbedWorkerConfig holds configuration for the background embedding worker.
type EmbedWorkerConfig struct {
	BatchSize    int           // number of pending rows to process per tick (default: 20)
	PollInterval time.Duration // how often to check for pending rows (default: 30s)
}

// DefaultEmbedWorkerConfig returns sensible defaults.
func DefaultEmbedWorkerConfig() EmbedWorkerConfig {
	return EmbedWorkerConfig{
		BatchSize:    20,
		PollInterval: 30 * time.Second,
	}
}

// EmbedWorker runs as a background goroutine, scanning for T2 entries with
// embedding_pending=true, computing embeddings via the provider, and updating
// the row. T2 rows themselves have no embedding column; this worker triggers
// the compression pipeline that produces T3 embeddings.
//
// For T3/T4/T5 rows, the worker updates the embedding column directly.
//
// Call Run in a goroutine; cancel ctx to stop.
func EmbedWorker(ctx context.Context, db *sql.DB, provider pkg.EmbeddingProvider, cfg EmbedWorkerConfig) {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 20
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 30 * time.Second
	}

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := processPendingT3(ctx, db, provider, cfg.BatchSize); err != nil {
				slog.Error("embed worker: T3 batch failed", "err", err)
			}
			if err := processPendingT4(ctx, db, provider, cfg.BatchSize); err != nil {
				slog.Error("embed worker: T4 batch failed", "err", err)
			}
			if err := processPendingT5(ctx, db, provider, cfg.BatchSize); err != nil {
				slog.Error("embed worker: T5 batch failed", "err", err)
			}
		}
	}
}

// processPendingT3 embeds narrative_summaries rows where embedding IS NULL.
func processPendingT3(ctx context.Context, db *sql.DB, provider pkg.EmbeddingProvider, batch int) error {
	rows, err := db.QueryContext(ctx,
		`SELECT id, content FROM narrative_summaries
		 WHERE embedding IS NULL
		 ORDER BY created_at ASC LIMIT ?`, batch,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	return embedRows(ctx, db, provider, rows, "narrative_summaries")
}

// processPendingT4 embeds reflections rows where embedding IS NULL.
func processPendingT4(ctx context.Context, db *sql.DB, provider pkg.EmbeddingProvider, batch int) error {
	rows, err := db.QueryContext(ctx,
		`SELECT id, content FROM reflections
		 WHERE embedding IS NULL
		 ORDER BY created_at ASC LIMIT ?`, batch,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	return embedRows(ctx, db, provider, rows, "reflections")
}

// processPendingT5 embeds kg_entities rows where embedding IS NULL.
func processPendingT5(ctx context.Context, db *sql.DB, provider pkg.EmbeddingProvider, batch int) error {
	rows, err := db.QueryContext(ctx,
		`SELECT id, summary FROM kg_entities
		 WHERE embedding IS NULL
		 ORDER BY t_created ASC LIMIT ?`, batch,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	return embedRows(ctx, db, provider, rows, "kg_entities")
}

// embedRows iterates a (id, content) result set and backfills embeddings.
// The embedding is stored as a JSON-encoded float32 array (TEXT column on SQLite,
// vector column on Postgres — the column type is handled by the migration runner).
func embedRows(ctx context.Context, db *sql.DB, provider pkg.EmbeddingProvider, rows *sql.Rows, table string) error {
	type pending struct {
		id      string
		content string
	}
	var items []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.content); err != nil {
			return err
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, item := range items {
		vec, err := provider.Embed(ctx, item.content)
		if err != nil {
			slog.Error("embed worker: embedding failed — skipping row",
				"table", table, "id", item.id, "err", err)
			continue
		}

		// Store as JSON float array — compatible with both TEXT (SQLite) and
		// vector columns (Postgres via pgvector's text input format).
		vecJSON, err := json.Marshal(vec)
		if err != nil {
			slog.Error("embed worker: marshalling vector", "id", item.id, "err", err)
			continue
		}

		if _, err := db.ExecContext(ctx,
			`UPDATE `+table+` SET embedding = ? WHERE id = ?`,
			string(vecJSON), item.id,
		); err != nil {
			slog.Error("embed worker: updating embedding", "table", table, "id", item.id, "err", err)
		}
	}

	return nil
}

// Compile-time interface checks.
var _ pkg.EmbeddingProvider = (*VoyageProvider)(nil)
var _ pkg.EmbeddingProvider = (*NomicProvider)(nil)
