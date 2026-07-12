package engine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// OpenAICompatClient implements pkg.LLMClient for any OpenAI-compatible endpoint
// (vLLM, Ollama, OpenAI API). Handles streaming SSE, thinking-token stripping,
// and system-message position quirks.
type OpenAICompatClient struct {
	endpoint   string
	apiKey     string
	model      string
	httpClient *http.Client

	// Quirks — set from model registry entries.
	// systemMsgFirstOnly: Qwen requires the system message at position 0 only.
	systemMsgFirstOnly bool
	// thinkingMode: inject enable_thinking in extra_body; strip <think>...</think> from responses.
	thinkingMode bool
}

// OpenAICompatConfig configures the client.
type OpenAICompatConfig struct {
	Endpoint           string
	APIKey             string
	Model              string
	SystemMsgFirstOnly bool          // Qwen compatibility
	ThinkingMode       bool          // local models with thinking tokens
	RequestTimeout     time.Duration // HTTP client timeout; 0 uses default (10 minutes)
}

// NewOpenAICompatClient creates a ready-to-use client.
func NewOpenAICompatClient(cfg OpenAICompatConfig) *OpenAICompatClient {
	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	return &OpenAICompatClient{
		endpoint:           strings.TrimRight(cfg.Endpoint, "/"),
		apiKey:             cfg.APIKey,
		model:              cfg.Model,
		httpClient:         &http.Client{Timeout: timeout},
		systemMsgFirstOnly: cfg.SystemMsgFirstOnly,
		thinkingMode:       cfg.ThinkingMode,
	}
}

// Complete sends a completion request and returns the full response.
// Streaming is used internally; the caller receives the assembled result.
func (c *OpenAICompatClient) Complete(ctx context.Context, req pkg.CompletionRequest) (*pkg.CompletionResponse, error) {
	messages, err := c.buildMessages(req)
	if err != nil {
		return nil, fmt.Errorf("building messages: %w", err)
	}

	body := map[string]any{
		"model":    c.model,
		"messages": messages,
		"stream":   true,
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if c.thinkingMode {
		body["extra_body"] = map[string]any{"enable_thinking": true}
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshalling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	start := time.Now()
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("executing HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("LLM endpoint returned %d: %s", resp.StatusCode, body)
	}

	return c.parseStream(resp.Body, start)
}

// buildMessages assembles the messages array, enforcing system-message position quirks.
func (c *OpenAICompatClient) buildMessages(req pkg.CompletionRequest) ([]map[string]any, error) {
	var msgs []map[string]any

	if req.System != "" {
		sysmsg := map[string]any{"role": "system", "content": req.System}
		if c.systemMsgFirstOnly {
			// Qwen: system message must be first; no further system messages allowed.
			msgs = append(msgs, sysmsg)
			for _, m := range req.Messages {
				if m.Role == "system" {
					slog.Warn("dropping non-first system message for system_msg_first_only model", "content_prefix", truncate(m.Content, 60))
					continue
				}
				msgs = append(msgs, map[string]any{"role": m.Role, "content": m.Content})
			}
			return msgs, nil
		}
		msgs = append(msgs, sysmsg)
	}

	for _, m := range req.Messages {
		msgs = append(msgs, map[string]any{"role": m.Role, "content": m.Content})
	}
	return msgs, nil
}

// sseChunk is the minimal shape of an OpenAI-compatible SSE data chunk.
type sseChunk struct {
	Choices []struct {
		Delta struct {
			Content          string          `json:"content"`
			ReasoningContent string          `json:"reasoning_content"`
			ToolCalls        []toolCallDelta `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// toolCallDelta is one partial tool call chunk from the SSE stream.
// The OpenAI streaming format sends tool calls incrementally: the first chunk
// for a given index carries the ID and function name, subsequent chunks append
// to function.arguments character by character.
type toolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// parseStream reads an SSE stream and assembles a CompletionResponse.
// Handles split lines, empty choices, null content, and [DONE] sentinels.
func (c *OpenAICompatClient) parseStream(r io.Reader, start time.Time) (*pkg.CompletionResponse, error) {
	var (
		contentBuf    strings.Builder
		toolCallAccum = make(map[int]*pkg.ToolCall)
		promptToks    int
		compToks      int
		ttft          time.Duration
		gotFirst      bool
	)

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" || line == ": keep-alive" {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			break
		}

		var chunk sseChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			slog.Debug("skipping malformed SSE chunk", "payload", truncate(payload, 120), "err", err)
			continue
		}

		if chunk.Usage != nil {
			promptToks = chunk.Usage.PromptTokens
			compToks = chunk.Usage.CompletionTokens
		}

		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]

		deltaContent := choice.Delta.Content
		if deltaContent == "" {
			deltaContent = choice.Delta.ReasoningContent
		}
		hasContent := deltaContent != ""
		hasToolCalls := len(choice.Delta.ToolCalls) > 0

		if !gotFirst && (hasContent || hasToolCalls) {
			ttft = time.Since(start)
			gotFirst = true
		}

		if hasContent {
			contentBuf.WriteString(deltaContent)
		}

		for _, tc := range choice.Delta.ToolCalls {
			acc, ok := toolCallAccum[tc.Index]
			if !ok {
				acc = &pkg.ToolCall{}
				toolCallAccum[tc.Index] = acc
			}
			if tc.ID != "" {
				acc.ID = tc.ID
			}
			if tc.Function.Name != "" {
				acc.Name = tc.Function.Name
			}
			acc.Arguments += tc.Function.Arguments
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading SSE stream: %w", err)
	}

	total := time.Since(start)
	raw := contentBuf.String()
	content, thinking := stripThinkingTokens(raw)

	var tokS float64
	if total.Seconds() > 0 && compToks > 0 {
		tokS = float64(compToks) / total.Seconds()
	}

	thinkingToks := len(thinking) / 4

	var toolCalls []pkg.ToolCall
	if len(toolCallAccum) > 0 {
		indices := make([]int, 0, len(toolCallAccum))
		for idx := range toolCallAccum {
			indices = append(indices, idx)
		}
		sort.Ints(indices)
		for _, idx := range indices {
			toolCalls = append(toolCalls, *toolCallAccum[idx])
		}
	}

	return &pkg.CompletionResponse{
		Content:          content,
		ThinkingTrace:    thinking,
		ToolCalls:        toolCalls,
		PromptTokens:     promptToks,
		CompletionTokens: compToks,
		ThinkingTokens:   thinkingToks,
		TTFT:             ttft,
		TotalLatency:     total,
		RawTokS:          tokS,
	}, nil
}

// stripThinkingTokens removes <think>...</think> or <thought>...</thought> blocks
// from content. Returns (cleaned content, thinking trace).
func stripThinkingTokens(s string) (content, thinking string) {
	for _, pair := range [][2]string{
		{"<think>", "</think>"},
		{"<thought>", "</thought>"},
	} {
		open, close := pair[0], pair[1]
		for {
			start := strings.Index(s, open)
			if start == -1 {
				break
			}
			end := strings.Index(s[start+len(open):], close)
			if end == -1 {
				break
			}
			end += start + len(open) + len(close)
			thinking += s[start+len(open) : end-len(close)]
			s = s[:start] + s[end:]
		}
	}
	return strings.TrimSpace(s), strings.TrimSpace(thinking)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
