package channels

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// CLIAdapter is the local channel: reads from stdin, writes to stdout.
// It is the simplest adapter and the default when no other channels are configured.
type CLIAdapter struct{}

// NewCLIAdapter returns a CLIAdapter.
func NewCLIAdapter() *CLIAdapter { return &CLIAdapter{} }

func (c *CLIAdapter) Name() string { return "cli" }

func (c *CLIAdapter) Capabilities() ChannelCaps {
	return ChannelCaps{
		Attachments: false,
		Reactions:   false,
		Threads:     false,
		Edits:       false,
		Push:        true, // stdin is blocking-push from the OS perspective
	}
}

// Start reads lines from stdin and emits them as InboundEvents.
// Closes the output channel when stdin reaches EOF or ctx is cancelled.
func (c *CLIAdapter) Start(ctx context.Context) (<-chan InboundEvent, error) {
	out := make(chan InboundEvent, 8)

	go func() {
		defer close(out)
		scanner := bufio.NewScanner(os.Stdin)
		for {
			// Check cancellation before each blocking Read.
			select {
			case <-ctx.Done():
				return
			default:
			}

			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					slog.Warn("cli: stdin scan error", "err", err)
				}
				return
			}

			line := scanner.Bytes()
			cp := make([]byte, len(line))
			copy(cp, line)

			select {
			case <-ctx.Done():
				return
			case out <- InboundEvent{
				Channel:     "cli",
				SenderID:    "operator",
				SenderName:  "operator",
				Content:     cp,
				ContentType: "text",
				ReceivedAt:  time.Now(),
				WakeWorthy:  true,
			}:
			}
		}
	}()

	return out, nil
}

// Send writes a message to stdout.
func (c *CLIAdapter) Send(_ context.Context, msg OutboundMessage) error {
	_, err := fmt.Println(msg.Content)
	return err
}
