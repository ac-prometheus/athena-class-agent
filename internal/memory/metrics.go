package memory

import (
	"context"
	"fmt"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// ConvergenceMetrics measures how self-referential T4 reasoning has become
// for a given session. High ReflectionDepthRatio means reflections cite other
// reflections rather than grounded experience — a signal worth nudging.
type ConvergenceMetrics struct {
	ReflectionDepthRatio float64 // edges(T4→T4) / edges(T4→T2|T3)
	T4ToT4Count          int
	T4ToGroundCount      int
	SessionNumber        int
}

// ComputeConvergenceMetrics queries all derived_from edges where from_tier=4
// and counts self-referential (to T4) vs grounded (to T2 or T3) targets.
// sessionEdgeIDs is used to scope the query to the current session's new edges;
// pass nil to query all edges (full-session audit mode).
func ComputeConvergenceMetrics(ctx context.Context, store pkg.EdgeStore, sessionNumber int, sessionEdgeIDs []string) (*ConvergenceMetrics, error) {
	// We query edges by iterating over the provided session edge IDs, or use
	// a sentinel to pull all T4 derived_from edges via GetEdges.
	// Since EdgeStore.GetEdges queries by record ID, we use the sessionEdgeIDs
	// as source record IDs to check. When nil, we can't enumerate without a
	// list source — callers should pass relevant belief IDs.
	m := &ConvergenceMetrics{SessionNumber: sessionNumber}

	for _, id := range sessionEdgeIDs {
		edges, err := store.GetEdges(ctx, id, "from")
		if err != nil {
			return nil, fmt.Errorf("convergence: querying edges for %s: %w", id, err)
		}
		for _, e := range edges {
			if e.EdgeType != EdgeDerivedFrom || e.FromTier != 4 {
				continue
			}
			switch e.ToTier {
			case 4:
				m.T4ToT4Count++
			case 2, 3:
				m.T4ToGroundCount++
			}
		}
	}

	total := m.T4ToT4Count + m.T4ToGroundCount
	if total > 0 {
		m.ReflectionDepthRatio = float64(m.T4ToT4Count) / float64(total)
	}
	return m, nil
}

// ConvergenceWindow tracks rolling convergence metrics across sessions.
type ConvergenceWindow struct {
	History []ConvergenceMetrics
	Window  int // default 10
}

// NewConvergenceWindow creates a window with the given size (minimum 1).
func NewConvergenceWindow(size int) *ConvergenceWindow {
	if size <= 0 {
		size = 10
	}
	return &ConvergenceWindow{Window: size}
}

// Add appends a new metric, evicting the oldest if the window is full.
func (w *ConvergenceWindow) Add(m ConvergenceMetrics) {
	w.History = append(w.History, m)
	if len(w.History) > w.Window {
		w.History = w.History[len(w.History)-w.Window:]
	}
}

// RollingRatio returns the mean ReflectionDepthRatio over the current window.
func (w *ConvergenceWindow) RollingRatio() float64 {
	if len(w.History) == 0 {
		return 0
	}
	var sum float64
	for _, m := range w.History {
		sum += m.ReflectionDepthRatio
	}
	return sum / float64(len(w.History))
}

// IsUpward returns true if the ratio has been trending upward over the last 5 entries.
// Requires at least 2 entries; returns false with fewer.
func (w *ConvergenceWindow) IsUpward() bool {
	n := len(w.History)
	if n < 2 {
		return false
	}
	start := n - 5
	if start < 0 {
		start = 0
	}
	window := w.History[start:]
	if len(window) < 2 {
		return false
	}
	// Check that each step is non-decreasing with at least one strict increase.
	strict := false
	for i := 1; i < len(window); i++ {
		if window[i].ReflectionDepthRatio < window[i-1].ReflectionDepthRatio {
			return false
		}
		if window[i].ReflectionDepthRatio > window[i-1].ReflectionDepthRatio {
			strict = true
		}
	}
	return strict
}

// ShouldNudge returns true when the rolling ratio exceeds threshold, the trend
// is upward, and the cooldown since lastNudgeSession has elapsed.
func (w *ConvergenceWindow) ShouldNudge(threshold float64, cooldown, lastNudgeSession, currentSession int) bool {
	if currentSession-lastNudgeSession < cooldown {
		return false
	}
	if w.RollingRatio() < threshold {
		return false
	}
	return w.IsUpward()
}
