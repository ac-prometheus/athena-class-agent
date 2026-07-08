package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const discordAPIBase = "https://discord.com/api/v10"

// DiscordConfig holds credentials and targeting for the Discord adapter.
// All fields are read from environment variables by the caller.
type DiscordConfig struct {
	Token        string
	ChannelIDs   []string
	PollInterval time.Duration // how often to poll each channel
}

// DiscordAdapter polls one or more Discord channels for new messages.
//
// Production should replace the polling loop with a WebSocket gateway connection
// (discord.com/api/v10/gateway) for lower latency and to avoid missing messages
// during long poll intervals. The REST polling approach here is sufficient for
// the Phase 5 scaffold and development environments.
type DiscordAdapter struct {
	cfg    DiscordConfig
	client *http.Client
	mu     sync.Mutex
	lastID map[string]string
}

// NewDiscordAdapter creates an adapter from the supplied config.
func NewDiscordAdapter(cfg DiscordConfig) *DiscordAdapter {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 30 * time.Second
	}
	return &DiscordAdapter{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
		lastID: make(map[string]string),
	}
}

func (d *DiscordAdapter) Name() string { return "discord" }

func (d *DiscordAdapter) Capabilities() ChannelCaps {
	return ChannelCaps{
		Attachments: true,
		Reactions:   true,
		Threads:     true,
		Edits:       false,
		Push:        false, // polling; upgrade to WebSocket for production
	}
}

// Start launches a goroutine per channel that polls for new messages.
func (d *DiscordAdapter) Start(ctx context.Context) (<-chan InboundEvent, error) {
	if d.cfg.Token == "" {
		return nil, fmt.Errorf("discord: DISCORD_TOKEN is required")
	}
	if len(d.cfg.ChannelIDs) == 0 {
		return nil, fmt.Errorf("discord: no channel IDs configured")
	}

	out := make(chan InboundEvent, 32)

	for _, chID := range d.cfg.ChannelIDs {
		go d.pollChannel(ctx, chID, out)
	}

	return out, nil
}

func (d *DiscordAdapter) pollChannel(ctx context.Context, channelID string, out chan<- InboundEvent) {
	ticker := time.NewTicker(d.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.mu.Lock()
			afterID := d.lastID[channelID]
			d.mu.Unlock()

			msgs, retryAfter, err := d.fetchMessages(ctx, channelID, afterID)
			if retryAfter > 0 {
				slog.Warn("discord: rate limited", "channel", channelID, "retry_after_s", retryAfter)
				select {
				case <-time.After(time.Duration(retryAfter) * time.Second):
				case <-ctx.Done():
					return
				}
				continue
			}
			if err != nil {
				slog.Warn("discord: fetch error", "channel", channelID, "err", err)
				continue
			}
			for _, msg := range msgs {
				select {
				case out <- msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

type discordMessage struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
	Author    struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"author"`
}

// fetchMessages returns events, a retry-after duration (>0 on rate limit), and error.
func (d *DiscordAdapter) fetchMessages(ctx context.Context, channelID, afterID string) ([]InboundEvent, int, error) {
	url := fmt.Sprintf("%s/channels/%s/messages?limit=50", discordAPIBase, channelID)
	if afterID != "" {
		url += "&after=" + afterID
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bot "+d.cfg.Token)

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := 5
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if n, err := strconv.Atoi(ra); err == nil {
				retryAfter = n
			}
		}
		return nil, retryAfter, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("discord: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var raw []discordMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, 0, fmt.Errorf("discord: decode: %w", err)
	}

	events := make([]InboundEvent, 0, len(raw))
	var newestID string
	for _, m := range raw {
		ts, _ := time.Parse(time.RFC3339, m.Timestamp)
		ev := InboundEvent{
			Channel:     "discord",
			SenderID:    m.Author.ID,
			SenderName:  m.Author.Username,
			Content:     []byte(m.Content),
			ContentType: "text",
			ReceivedAt:  ts,
		}
		events = append(events, ev)
		newestID = m.ID
	}
	if newestID != "" {
		d.mu.Lock()
		d.lastID[channelID] = newestID
		d.mu.Unlock()
	}

	return events, 0, nil
}

// Send posts a message to the specified Discord channel.
func (d *DiscordAdapter) Send(ctx context.Context, msg OutboundMessage) error {
	if msg.Channel == "" {
		return fmt.Errorf("discord: Send requires OutboundMessage.Channel (channel ID)")
	}

	payload, err := json.Marshal(map[string]string{"content": msg.Content})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/channels/%s/messages", discordAPIBase, msg.Channel)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+d.cfg.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord: send HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
