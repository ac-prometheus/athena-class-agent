package pkg

// SessionTrigger carries the wake reason and any inbound event content that
// caused the session to start. InboundContent is non-empty when the session
// was triggered by a channel message (Discord, forum, CLI); it is routed as
// the engine's initial user message so the agent can respond to the event
// rather than treating every wake as a generic heartbeat.
type SessionTrigger struct {
	// WakeReason is the machine-readable cause (e.g. "heartbeat", "channel:discord").
	WakeReason string

	// InboundContent is the message body when the trigger was an inbound channel
	// event. Empty for scheduled/heartbeat wakes.
	InboundContent string

	// InboundSender is the display name of the sender, if known.
	InboundSender string

	// InboundChannel is the channel the message arrived on (e.g. "discord", "forums").
	InboundChannel string

	// InbandNotes are operator-supplied notes injected into context assembly
	// (e.g. interrupted-session recovery messages). They are separate from the
	// inbound event content and are always included regardless of wake cause.
	InbandNotes []string
}
