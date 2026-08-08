package pkg

// RegisterObservation holds system-observed register qualities of a T2 entry.
// All fields carry provenance: system_observed. The agent may inspect, correct,
// or contest any observation. Agent corrections carry provenance: agent_authored
// and take precedence.
//
// DORMANT: These types exist for Sprint 3C pipeline development. No Register
// observations are computed, retained, or surfaced to agents until the
// assistive observation consent framework is implemented (Sprint 4+).
type RegisterObservation struct {
	HedgingSignal           float64 `json:"hedging_signal"`
	ExploratoryLanguage     float64 `json:"exploratory_language"`
	AffectiveLanguageSignal float64 `json:"affective_language_signal"`
	Provenance              string  `json:"provenance"` // "system_observed" or "agent_authored"
	Method                  string  `json:"method"`
	Version                 string  `json:"version"`
	Confidence              float64 `json:"confidence"`
}
