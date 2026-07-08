// Package awareness provides environmental grounding data and peripheral awareness
// for Phase 6 of Tier 1 context assembly.
package awareness

import (
	"time"
)

// Grounding is an identity anchor.
// GroundingData holds environmental context surfaced in Phase 6 of T1 assembly.
// Time and timezone are always populated. Location comes from config (may be empty).
// Weather is a stub at Phase 3 — it returns "not configured" unless a weather
// API key and location are provided in config.
type GroundingData struct {
	// Time is the current time at session start, in the configured timezone.
	Time time.Time
	// Timezone is the IANA timezone name (e.g. "America/Los_Angeles").
	// Falls back to "UTC" if not configured.
	Timezone string
	// Location is a human-readable location string from config (e.g. "Portland, OR").
	// Empty string if not configured.
	Location string
	// Weather is a short weather description.
	// Returns "not configured" unless a weather API key is set.
	Weather string
}

// GroundingConfig holds the operator-provided environmental settings.
type GroundingConfig struct {
	// Timezone is the IANA timezone name. Defaults to "UTC" if empty.
	Timezone string
	// Location is a human-readable location string. Empty means "not configured".
	Location string
	// WeatherAPIKey is the API key for the weather backend. Empty disables weather.
	WeatherAPIKey string
	// WeatherLocation is the location string for weather queries (may differ from Location).
	WeatherLocation string
}

// Gather assembles the current grounding data using the provided config.
// Weather is stubbed at Phase 3 — it always returns "not configured" unless
// WeatherAPIKey is set (even then, this stub returns "not configured"; a real
// implementation wires in an HTTP call to the weather backend).
func Gather(cfg GroundingConfig) *GroundingData {
	tz := cfg.Timezone
	if tz == "" {
		tz = "UTC"
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		// Invalid timezone falls back to UTC.
		loc = time.UTC
		tz = "UTC"
	}

	now := time.Now().In(loc)

	weather := "not configured"
	if cfg.WeatherAPIKey != "" {
		// Phase 3 stub: weather API integration deferred to a later phase.
		// When wired: call the weather backend here and populate weather.
		weather = "not configured"
	}

	return &GroundingData{
		Time:     now,
		Timezone: tz,
		Location: cfg.Location,
		Weather:  weather,
	}
}

// Format returns a human-readable grounding block suitable for Phase 6 of T1 assembly.
func (g *GroundingData) Format() string {
	loc := g.Location
	if loc == "" {
		loc = "unknown"
	}
	return "Time: " + g.Time.Format("2006-01-02 15:04:05 MST") +
		"\nTimezone: " + g.Timezone +
		"\nLocation: " + loc +
		"\nWeather: " + g.Weather
}
