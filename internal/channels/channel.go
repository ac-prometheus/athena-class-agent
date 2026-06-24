package channels

import (
	"context"
	"time"
)

// Channel is the contract every inbound/outbound adapter must satisfy.
// Adapters push events — callers never poll them directly.
type Channel interface {
	Name() string
	Start(ctx context.Context) (<-chan InboundEvent, error)
	Send(ctx context.Context, msg OutboundMessage) error
	Capabilities() ChannelCaps
}

// InboundEvent carries a single message received from a channel.
// Content is the raw bytes — Aegis needs the originals before any normalisation.
type InboundEvent struct {
	Channel     string
	SenderID    string
	SenderName  string
	Content     []byte
	ContentType string // "text", "attachment", "reaction"
	ThreadID    string
	ReceivedAt  time.Time
	WakeWorthy  bool
}

// OutboundMessage describes a message to be sent through a channel.
type OutboundMessage struct {
	Channel  string
	Content  string
	ThreadID string
	ReplyTo  string
	Files    []string
}

// ChannelCaps describes what a channel adapter supports.
type ChannelCaps struct {
	Attachments bool
	Reactions   bool
	Threads     bool
	Edits       bool
	Push        bool // true = WebSocket/gateway push; false = polling
}
