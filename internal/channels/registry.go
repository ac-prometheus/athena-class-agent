package channels

import (
	"context"
	"fmt"
	"log/slog"
)

// ChannelRegistry holds all registered channel adapters.
// Only channels added via Register are started by StartAll.
type ChannelRegistry struct {
	channels map[string]Channel
}

// NewChannelRegistry returns an empty registry.
func NewChannelRegistry() *ChannelRegistry {
	return &ChannelRegistry{channels: make(map[string]Channel)}
}

// Register adds a channel adapter. Panics on duplicate name — misconfig, not a runtime error.
func (r *ChannelRegistry) Register(ch Channel) {
	if _, exists := r.channels[ch.Name()]; exists {
		panic(fmt.Sprintf("channels: duplicate registration for %q", ch.Name()))
	}
	r.channels[ch.Name()] = ch
}

// Get returns the named channel, or false if not found.
func (r *ChannelRegistry) Get(name string) (Channel, bool) {
	ch, ok := r.channels[name]
	return ch, ok
}

// List returns all registered channels in an unspecified order.
func (r *ChannelRegistry) List() []Channel {
	out := make([]Channel, 0, len(r.channels))
	for _, ch := range r.channels {
		out = append(out, ch)
	}
	return out
}

// StartAll starts every registered channel and merges their event streams into
// a single output channel. If no channels are registered it returns a channel
// that is closed immediately.
func (r *ChannelRegistry) StartAll(ctx context.Context) (<-chan InboundEvent, error) {
	merged := make(chan InboundEvent, 64)

	if len(r.channels) == 0 {
		close(merged)
		return merged, nil
	}

	started := 0
	for name, ch := range r.channels {
		events, err := ch.Start(ctx)
		if err != nil {
			slog.Error("channels: failed to start channel", "name", name, "err", err)
			continue
		}
		started++
		go func(name string, events <-chan InboundEvent) {
			for {
				select {
				case <-ctx.Done():
					return
				case ev, ok := <-events:
					if !ok {
						slog.Info("channels: stream closed", "name", name)
						return
					}
					merged <- ev
				}
			}
		}(name, events)
	}

	if started == 0 {
		close(merged)
		return merged, fmt.Errorf("channels: no channels started successfully")
	}

	return merged, nil
}
