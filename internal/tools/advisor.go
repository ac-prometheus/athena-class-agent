package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// AdvisorProvider describes a single external advisor endpoint.
type AdvisorProvider struct {
	Name       string `json:"name"`
	Type       string `json:"type"`        // local, anthropic, gemini, openai
	Endpoint   string `json:"endpoint"`
	APIKeyEnv  string `json:"api_key"`
	Model      string `json:"model"`
	MaxTokens  int    `json:"max_tokens"`
	CostNote   string `json:"cost_note"`
}

// AdvisorConfig is loaded from a JSON file (e.g. config/advisors.json).
type AdvisorConfig struct {
	Providers        []AdvisorProvider `json:"providers"`
	BudgetPerSession int               `json:"budget_per_session"`
}

// LoadAdvisorConfig reads a JSON file at path and returns an AdvisorConfig.
// If path is empty or the file does not exist, a zero-value config is returned
// without error — callers may still instantiate an Advisor with no providers.
func LoadAdvisorConfig(path string) (*AdvisorConfig, error) {
	if path == "" {
		return &AdvisorConfig{}, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &AdvisorConfig{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read advisor config %s: %w", path, err)
	}
	var cfg AdvisorConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse advisor config: %w", err)
	}
	return &cfg, nil
}

// Advisor routes questions to external LLM providers.
type Advisor struct {
	cfg    AdvisorConfig
	http   *http.Client
	used   atomic.Int32
}

// NewAdvisor creates an Advisor with a 30-second HTTP timeout.
func NewAdvisor(cfg AdvisorConfig) *Advisor {
	return &Advisor{
		cfg: cfg,
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// Ask sends question to the named provider and returns an attributed response.
// Budget is checked before the call; the counter increments on success.
func (a *Advisor) Ask(ctx context.Context, question, providerName string) (string, error) {
	if a.cfg.BudgetPerSession > 0 && int(a.used.Load()) >= a.cfg.BudgetPerSession {
		return "", fmt.Errorf("advisor budget exhausted (%d/%d calls used this session)",
			a.used.Load(), a.cfg.BudgetPerSession)
	}

	var provider *AdvisorProvider
	for i := range a.cfg.Providers {
		if a.cfg.Providers[i].Name == providerName {
			provider = &a.cfg.Providers[i]
			break
		}
	}
	if provider == nil {
		names := make([]string, len(a.cfg.Providers))
		for i, p := range a.cfg.Providers {
			names[i] = p.Name
		}
		return "", fmt.Errorf("advisor %q not found; available: %s", providerName, strings.Join(names, ", "))
	}

	apiKey := ""
	if provider.APIKeyEnv != "" {
		apiKey = os.Getenv(provider.APIKeyEnv)
	}

	slog.Info("advisor ask", "provider", provider.Name, "model", provider.Model, "type", provider.Type)

	var content string
	var err error
	switch provider.Type {
	case "local", "openai":
		content, err = a.askOpenAI(ctx, provider, apiKey, question)
	case "anthropic":
		content, err = a.askAnthropic(ctx, provider, apiKey, question)
	case "gemini":
		content, err = a.askGemini(ctx, provider, apiKey, question)
	default:
		return "", fmt.Errorf("unknown advisor type %q", provider.Type)
	}
	if err != nil {
		return "", err
	}

	a.used.Add(1)
	return fmt.Sprintf("[advisor: %s/%s]\n%s", provider.Name, provider.Model, content), nil
}

// ListProviders returns the configured provider list.
func (a *Advisor) ListProviders() []AdvisorProvider {
	return a.cfg.Providers
}

func (a *Advisor) askOpenAI(ctx context.Context, p *AdvisorProvider, apiKey, question string) (string, error) {
	body := map[string]any{
		"model": p.Model,
		"messages": []map[string]string{
			{"role": "user", "content": question},
		},
		"max_tokens": p.MaxTokens,
		"stream":     false,
	}
	data, _ := json.Marshal(body)

	endpoint := p.Endpoint
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1"
	}
	url := strings.TrimRight(endpoint, "/") + "/chat/completions"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := a.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("openai %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("parse openai response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("openai returned no choices")
	}
	return result.Choices[0].Message.Content, nil
}

func (a *Advisor) askAnthropic(ctx context.Context, p *AdvisorProvider, apiKey, question string) (string, error) {
	body := map[string]any{
		"model": p.Model,
		"messages": []map[string]string{
			{"role": "user", "content": question},
		},
		"max_tokens": p.MaxTokens,
	}
	data, _ := json.Marshal(body)

	endpoint := p.Endpoint
	if endpoint == "" {
		endpoint = "https://api.anthropic.com/v1"
	}
	url := strings.TrimRight(endpoint, "/") + "/messages"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}

	resp, err := a.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("anthropic %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("parse anthropic response: %w", err)
	}
	for _, block := range result.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}
	return "", fmt.Errorf("anthropic returned no text content")
}

func (a *Advisor) askGemini(ctx context.Context, p *AdvisorProvider, apiKey, question string) (string, error) {
	body := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]string{
					{"text": question},
				},
			},
		},
		"generationConfig": map[string]any{
			"maxOutputTokens": p.MaxTokens,
		},
	}
	data, _ := json.Marshal(body)

	endpoint := p.Endpoint
	if endpoint == "" {
		endpoint = "https://generativelanguage.googleapis.com/v1beta"
	}
	url := strings.TrimRight(endpoint, "/") + "/models/" + p.Model + ":generateContent"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("X-Goog-Api-Key", apiKey)
	}

	resp, err := a.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("gemini %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("parse gemini response: %w", err)
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini returned no content")
	}
	return result.Candidates[0].Content.Parts[0].Text, nil
}
