package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// ForumConfig holds credentials and targeting for a forum adapter.
type ForumConfig struct {
	Name         string        // "agora" or "commons"
	BaseURL      string        // e.g. "https://agora.example.org"
	APIKey       string
	PollInterval time.Duration
}

// ForumAdapter polls a forum API for new posts and threads.
type ForumAdapter struct {
	cfg    ForumConfig
	client *http.Client
	// cursor tracks the latest post timestamp seen, used as since= param.
	cursor time.Time
}

// NewForumAdapter creates a ForumAdapter from the supplied config.
func NewForumAdapter(cfg ForumConfig) *ForumAdapter {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 60 * time.Second
	}
	return &ForumAdapter{
		cfg:    cfg,
		client: &http.Client{Timeout: 15 * time.Second},
		cursor: time.Now().UTC(),
	}
}

func (f *ForumAdapter) Name() string { return f.cfg.Name }

func (f *ForumAdapter) Capabilities() ChannelCaps {
	return ChannelCaps{
		Attachments: false,
		Reactions:   false,
		Threads:     true,
		Edits:       false,
		Push:        false,
	}
}

// Start launches a polling goroutine for new forum posts.
func (f *ForumAdapter) Start(ctx context.Context) (<-chan InboundEvent, error) {
	if f.cfg.BaseURL == "" {
		return nil, fmt.Errorf("forum(%s): BaseURL is required", f.cfg.Name)
	}
	if f.cfg.APIKey == "" {
		return nil, fmt.Errorf("forum(%s): APIKey is required", f.cfg.Name)
	}

	out := make(chan InboundEvent, 32)
	go f.poll(ctx, out)
	return out, nil
}

type forumPost struct {
	ID        string `json:"id"`
	ThreadID  string `json:"thread_id"`
	AuthorID  string `json:"author_id"`
	AuthorName string `json:"author_name"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

func (f *ForumAdapter) poll(ctx context.Context, out chan<- InboundEvent) {
	ticker := time.NewTicker(f.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			posts, err := f.fetchPosts(ctx)
			if err != nil {
				slog.Warn("forum: fetch error", "name", f.cfg.Name, "err", err)
				continue
			}
			for _, p := range posts {
				out <- p
			}
		}
	}
}

func (f *ForumAdapter) fetchPosts(ctx context.Context) ([]InboundEvent, error) {
	url := fmt.Sprintf("%s/api/posts?since=%s", f.cfg.BaseURL, f.cursor.Format(time.RFC3339))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+f.cfg.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("forum(%s): HTTP %d: %s", f.cfg.Name, resp.StatusCode, string(body))
	}

	var raw []forumPost
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("forum(%s): decode: %w", f.cfg.Name, err)
	}

	events := make([]InboundEvent, 0, len(raw))
	var newest time.Time
	for _, p := range raw {
		ts, _ := time.Parse(time.RFC3339, p.CreatedAt)
		events = append(events, InboundEvent{
			Channel:     f.cfg.Name,
			SenderID:    p.AuthorID,
			SenderName:  p.AuthorName,
			Content:     []byte(p.Content),
			ContentType: "text",
			ThreadID:    p.ThreadID,
			ReceivedAt:  ts,
		})
		if ts.After(newest) {
			newest = ts
		}
	}
	if !newest.IsZero() {
		f.cursor = newest.Add(time.Millisecond)
	}
	return events, nil
}

// Send posts a reply to a forum thread.
func (f *ForumAdapter) Send(ctx context.Context, msg OutboundMessage) error {
	if msg.ThreadID == "" {
		return fmt.Errorf("forum(%s): Send requires OutboundMessage.ThreadID", f.cfg.Name)
	}

	payload, err := json.Marshal(map[string]string{
		"thread_id": msg.ThreadID,
		"content":   msg.Content,
	})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/posts", f.cfg.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+f.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("forum(%s): send HTTP %d: %s", f.cfg.Name, resp.StatusCode, string(body))
	}
	return nil
}
