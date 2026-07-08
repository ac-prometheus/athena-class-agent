package daemon

import (
	"log/slog"
	"regexp"
	"time"

	"github.com/ac-prometheus/athena-class-agent/internal/channels"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// WakeCondition describes a single stimulus that may trigger a session.
type WakeCondition struct {
	Channel  string
	Pattern  string // regex or keyword matched against event content
	Priority int    // higher = more likely to override idle state
}

// WakeScheduler decides whether an inbound event warrants waking the agent
// and tracks any explicitly scheduled future wakes.
type WakeScheduler struct {
	conditions []WakeCondition
	compiled   []*regexp.Regexp
	scheduled  []time.Time
	minTrust   float64 // events below this trust score never wake (default 0.30)
}

// NewWakeScheduler compiles each condition's pattern and returns a scheduler.
func NewWakeScheduler(conditions []WakeCondition) *WakeScheduler {
	ws := &WakeScheduler{conditions: conditions, minTrust: 0.30}
	ws.compiled = make([]*regexp.Regexp, len(conditions))
	for i, c := range conditions {
		re, err := regexp.Compile(c.Pattern)
		if err != nil {
			slog.Warn("wake: invalid pattern", "pattern", c.Pattern, "err", err)
			ws.compiled[i] = nil
		} else {
			ws.compiled[i] = re
		}
	}
	return ws
}

// ShouldWake returns true if the event passes Aegis checks AND matches a wake
// condition. The adapter's WakeWorthy flag is a hint, not a gate — Aegis has
// the final word. Events that failed injection scanning or have trust below
// the minimum threshold are always rejected regardless of WakeWorthy.
func (ws *WakeScheduler) ShouldWake(event channels.InboundEvent, annotation *pkg.AegisAnnotation) bool {
	if annotation != nil {
		if !annotation.ScanPassed {
			slog.Info("wake: rejected — injection flags", "channel", event.Channel, "flags", annotation.Flags)
			return false
		}
		if annotation.TrustScore < ws.minTrust {
			slog.Info("wake: rejected — low trust", "channel", event.Channel, "trust", annotation.TrustScore)
			return false
		}
	}

	if event.WakeWorthy {
		return true
	}
	for i, c := range ws.conditions {
		if c.Channel != "" && c.Channel != event.Channel {
			continue
		}
		re := ws.compiled[i]
		if re == nil {
			continue
		}
		if re.Match(event.Content) && c.Priority > 0 {
			return true
		}
	}
	return false
}

// ScheduleWake adds an explicit future wake time.
func (ws *WakeScheduler) ScheduleWake(at time.Time) {
	ws.scheduled = append(ws.scheduled, at)
	slog.Info("wake: scheduled", "at", at)
}

// NextWake returns the earliest scheduled wake that is still in the future,
// or nil if nothing is scheduled.
func (ws *WakeScheduler) NextWake() *time.Time {
	now := time.Now()
	var earliest *time.Time
	for _, t := range ws.scheduled {
		t := t
		if t.After(now) {
			if earliest == nil || t.Before(*earliest) {
				earliest = &t
			}
		}
	}
	return earliest
}

// DeclineWake records a logged decision not to wake despite a stimulus.
func (ws *WakeScheduler) DeclineWake(reason string) {
	slog.Info("wake: declined", "reason", reason)
}
