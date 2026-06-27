package awareness

import (
	"math"
	"math/rand/v2"
	"sort"
	"strings"

	"github.com/ac-prometheus/athena-class-agent/internal/memory"
)

// PeripheralAwareness tracks semantic drift across turns via an EWMA centroid.
// When the conversation drifts far from its centroid (high velocity), a nudge
// is injected suggesting a targeted memory search — the agent can then decide
// whether to follow it.
//
// Design constraints:
//   - At most 4 nudges per session (nudgeCount cap)
//   - At least 8 turns between nudges (lastNudgeTurn cooldown)
//   - Velocity threshold has ±jitter to avoid mechanistic triggering
type PeripheralAwareness struct {
	centroid           []float32
	centroidN          int
	lambda             float64 // EWMA decay: default 0.85
	nudgeCount         int
	lastNudgeTurn      int
	velocityThreshold  float64
	jitter             float64 // default 0.03
	convergenceWindow  *memory.ConvergenceWindow
	lastNudgeSession   int
}

// NudgeInjection is the output when a velocity spike warrants a memory hint.
type NudgeInjection struct {
	Turn     int
	Velocity float64
	Verb     string   // CLI verb (e.g. "memory")
	Args     []string // CLI args (e.g. ["search", "topic"])
	Topic    string
}

// Command returns a display-safe CLI suggestion. Never pass this to sh -c.
func (n *NudgeInjection) Command() string {
	if len(n.Args) == 0 {
		return n.Verb
	}
	return n.Verb + " " + strings.Join(n.Args, " ")
}

// NewPeripheralAwareness creates a PeripheralAwareness with the given threshold.
// threshold should come from config (PA_VELOCITY_THRESHOLD, default 0.28).
func NewPeripheralAwareness(threshold float64) *PeripheralAwareness {
	return &PeripheralAwareness{
		lambda:            0.85,
		velocityThreshold: threshold,
		jitter:            0.03,
		lastNudgeTurn:     -8, // allow first nudge on turn 0 if velocity is high
		convergenceWindow: memory.NewConvergenceWindow(10),
		lastNudgeSession:  -100, // allow first session nudge immediately
	}
}

// UpdateCentroid incorporates turnEmbedding into the running EWMA centroid.
// Must be called after CheckNudge so velocity is computed against the pre-turn centroid.
func (pa *PeripheralAwareness) UpdateCentroid(turnEmbedding []float32) {
	if len(turnEmbedding) == 0 {
		return
	}
	if pa.centroid == nil {
		pa.centroid = make([]float32, len(turnEmbedding))
		copy(pa.centroid, turnEmbedding)
		pa.centroidN = 1
		return
	}
	if len(pa.centroid) != len(turnEmbedding) {
		return
	}
	// EWMA: centroid_new = λ * centroid_old + (1-λ) * turn
	for i := range pa.centroid {
		pa.centroid[i] = float32(pa.lambda*float64(pa.centroid[i]) +
			(1-pa.lambda)*float64(turnEmbedding[i]))
	}
	pa.centroidN++
}

// SemanticVelocity returns the cosine distance between the current centroid
// and the new embedding. Returns 0 if the centroid is not yet established.
func (pa *PeripheralAwareness) SemanticVelocity(turnEmbedding []float32) float64 {
	if pa.centroid == nil || len(pa.centroid) != len(turnEmbedding) {
		return 0
	}
	return cosineDistance(pa.centroid, turnEmbedding)
}

// CheckNudge evaluates whether a nudge should fire this turn.
// Returns nil if no nudge fires (caps, cooldown, or velocity below threshold).
// Call UpdateCentroid after this to keep velocity computation accurate.
func (pa *PeripheralAwareness) CheckNudge(turn int, turnEmbedding []float32, topics []string) *NudgeInjection {
	if pa.nudgeCount >= 4 {
		return nil
	}
	if turn-pa.lastNudgeTurn < 8 {
		return nil
	}

	velocity := pa.SemanticVelocity(turnEmbedding)
	// Apply jitter so the threshold isn't a hard cliff.
	jitterDelta := (rand.Float64()*2 - 1) * pa.jitter
	effectiveThreshold := pa.velocityThreshold + jitterDelta

	if velocity <= effectiveThreshold {
		return nil
	}

	topic := chooseTopic(topics)
	// Fall back to content-derived topics when tool-call names are uninformative.
	if topic == "current focus" && len(topics) > 0 {
		if derived := extractTopics(strings.Join(topics, " "), 1); len(derived) > 0 {
			topic = derived[0]
		}
	}
	nudge := &NudgeInjection{
		Turn:     turn,
		Velocity: velocity,
		Topic:    topic,
		Verb: "memory",
		Args: []string{"search", topic},
	}

	pa.nudgeCount++
	pa.lastNudgeTurn = turn
	return nudge
}

// CheckConvergenceNudge records metrics for this session and returns a nudge
// if the rolling convergence ratio exceeds the threshold with an upward trend.
// cooldown and ratioThreshold come from config (CONVERGENCE_NUDGE_COOLDOWN,
// CONVERGENCE_RATIO_THRESHOLD).
func (pa *PeripheralAwareness) CheckConvergenceNudge(session int, metrics *memory.ConvergenceMetrics, ratioThreshold float64, cooldown int) *NudgeInjection {
	pa.convergenceWindow.Add(*metrics)
	if !pa.convergenceWindow.ShouldNudge(ratioThreshold, cooldown, pa.lastNudgeSession, session) {
		return nil
	}
	pa.lastNudgeSession = session
	ratio := pa.convergenceWindow.RollingRatio()
	n := len(pa.convergenceWindow.History)
	return &NudgeInjection{
		Turn:     -1, // session-level nudge, not turn-level
		Topic:    "grounded experience",
		Verb: "memory",
		Args: []string{"search", "direct experience session log"},
		Velocity: ratio,
	}
}

// chooseTopic returns the first non-empty topic, or a generic fallback.
// When topics are empty or all blank, extractTopics is called on content if provided.
func chooseTopic(topics []string) string {
	for _, t := range topics {
		if t != "" {
			return t
		}
	}
	return "current focus"
}

// extractTopics returns up to maxTopics high-frequency non-stopword words from content.
// Used to derive a relevant topic from turn content when no explicit topics are available.
func extractTopics(content string, maxTopics int) []string {
	stopwords := map[string]bool{
		"a": true, "an": true, "the": true, "and": true, "or": true, "but": true,
		"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
		"with": true, "by": true, "from": true, "as": true, "is": true, "was": true,
		"are": true, "were": true, "be": true, "been": true, "being": true, "have": true,
		"has": true, "had": true, "do": true, "does": true, "did": true, "will": true,
		"would": true, "could": true, "should": true, "may": true, "might": true,
		"must": true, "can": true, "it": true, "its": true, "this": true, "that": true,
		"i": true, "you": true, "he": true, "she": true, "we": true, "they": true,
		"my": true, "your": true, "his": true, "her": true, "our": true, "their": true,
		"not": true, "no": true, "so": true, "if": true, "then": true, "than": true,
		"what": true, "which": true, "who": true, "when": true, "where": true, "how": true,
	}

	freq := make(map[string]int)
	for _, word := range strings.Fields(content) {
		w := strings.ToLower(strings.Trim(word, ".,!?;:\"'()[]{}"))
		if len(w) < 3 || stopwords[w] {
			continue
		}
		freq[w]++
	}

	type entry struct {
		word  string
		count int
	}
	entries := make([]entry, 0, len(freq))
	for w, c := range freq {
		entries = append(entries, entry{w, c})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count != entries[j].count {
			return entries[i].count > entries[j].count
		}
		return entries[i].word < entries[j].word
	})

	result := make([]string, 0, maxTopics)
	for _, e := range entries {
		if len(result) >= maxTopics {
			break
		}
		result = append(result, e.word)
	}
	return result
}

// cosineDistance returns 1 - cosine_similarity(a, b).
// Result is in [0, 2]; 0 = identical direction, 2 = opposite.
func cosineDistance(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		ai, bi := float64(a[i]), float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	similarity := dot / (math.Sqrt(normA) * math.Sqrt(normB))
	return 1 - similarity
}
