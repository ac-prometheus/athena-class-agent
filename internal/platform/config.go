package platform

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration, loaded from environment variables.
// Channel-specific config and model registry live in JSON files (config/).
// API keys are environment variables only — never committed.
type Config struct {
	// Identity
	IdentityDir string
	AgentName   string

	// LLM — primary substrate
	LLMProvider  string // "anthropic", "openai", "gemini"
	LLMModel     string
	LLMAPIKey    string
	LLMEndpoint  string // for openai-compat (vLLM, Ollama)
	LLMProfile   string // model server profile name (see config/profiles.json)

	// LLM — secondary (vision, critic, triage)
	LLMSecondaryProvider  string
	LLMSecondaryEndpoint  string
	LLMSecondaryModel     string
	LLMSecondaryAPIKey    string
	LLMSecondaryRole      string // "vision", "critic", "triage"

	// Embeddings
	EmbedProvider   string
	EmbedModel      string
	EmbedAPIKey     string
	EmbedDimensions int

	// Database
	DatabaseDSN string

	// Session
	TokenBudget     int
	HardFloorTokens int
	SessionTrigger  string // "heartbeat", "external"
	WorkspaceDir    string

	// Awareness
	PAVelocityThreshold float64
	BridgeAbstainRate   float64

	// Belief decay
	BeliefDecayRate    float64
	InferenceDecayBase float64
	StaleThreshold     float64

	// Phase 6 — belief tuning
	ContradictionRetrievalProbability float64
	ConvergenceRatioThreshold         float64
	ConvergenceNudgeCooldown          int
	ConvergenceWindowSize             int

	// Telemetry
	// Note: turn-event emission is always on; this flag gates session aggregate persistence only.
	TelemetryEnabled bool

	// Sandbox
	SandboxMode         string // "container", "user", "permissive", "none"
	SandboxAllowedPaths string
	SandboxBlockedCmds  string
	SandboxUser         string
	RuntimeDir          string
	AdvisorConfigPath   string

	// LLM request timeout. Applied to the HTTP client used by all LLM callers.
	// Default 10 minutes; read from LLM_REQUEST_TIMEOUT (e.g. "10m", "600s").
	LLMRequestTimeout time.Duration

	// Channels
	DiscordToken      string
	DiscordChannelIDs string // comma-separated channel IDs
	DiscordPollSecs   int

	// Aegis
	AegisTrustSkepticalPrior float64
	AegisTrustRampN          int
	AegisOutboundPathPrefixes string // comma-separated path prefixes to flag

	// Operational
	SkipWitnessCheck bool // only for automated testing — logged explicitly
}

// Load reads configuration from environment variables, applying defaults where not set.
func Load() (*Config, error) {
	c := &Config{
		IdentityDir:         envStr("IDENTITY_DIR", "./identity"),
		AgentName:           envStr("AGENT_NAME", "aurora"),
		LLMProvider:         envStr("LLM_PROVIDER", "openai"),
		LLMModel:            envStr("LLM_MODEL", ""),
		LLMAPIKey:           envStr("LLM_API_KEY", ""),
		LLMEndpoint:         envStr("LLM_ENDPOINT", ""),
		LLMProfile:          envStr("LLM_PROFILE", "enforce-eager"),
		LLMSecondaryProvider: envStr("LLM_SECONDARY_PROVIDER", ""),
		LLMSecondaryEndpoint: envStr("LLM_SECONDARY_ENDPOINT", ""),
		LLMSecondaryModel:    envStr("LLM_SECONDARY_MODEL", ""),
		LLMSecondaryAPIKey:   envStr("LLM_SECONDARY_API_KEY", ""),
		LLMSecondaryRole:     envStr("LLM_SECONDARY_ROLE", ""),
		EmbedProvider:       envStr("EMBED_PROVIDER", "voyage"),
		EmbedModel:          envStr("EMBED_MODEL", "voyage-3.5"),
		EmbedAPIKey:         envStr("EMBED_API_KEY", ""),
		EmbedDimensions:     envInt("EMBED_DIMENSIONS", 1024),
		DatabaseDSN:         envStr("DATABASE_DSN", "sqlite://./agent.db"),
		TokenBudget:         envInt("TOKEN_BUDGET", 200000),
		HardFloorTokens:     envInt("HARD_FLOOR_TOKENS", 1500),
		SessionTrigger:      envStr("SESSION_TRIGGER", "heartbeat"),
		WorkspaceDir:        envStr("WORKSPACE_DIR", "./workspace"),
		PAVelocityThreshold: envFloat("PA_VELOCITY_THRESHOLD", 0.28),
		BridgeAbstainRate:   envFloat("BRIDGE_ABSTAIN_RATE", 0.20),
		BeliefDecayRate:     envFloat("BELIEF_DECAY_RATE", 0.05),
		InferenceDecayBase:  envFloat("INFERENCE_DECAY_BASE", 0.90),
		StaleThreshold:      envFloat("STALE_THRESHOLD", 0.20),
		ContradictionRetrievalProbability: clampFloat(envFloat("CONTRADICTION_RETRIEVAL_PROBABILITY", 0.30), 0, 1),
		ConvergenceRatioThreshold:         clampFloat(envFloat("CONVERGENCE_RATIO_THRESHOLD", 0.60), 0, 1),
		ConvergenceNudgeCooldown:          clampInt(envInt("CONVERGENCE_NUDGE_COOLDOWN", 5), 1, 20),
		ConvergenceWindowSize:             clampInt(envInt("CONVERGENCE_WINDOW_SIZE", 10), 3, 50),
		TelemetryEnabled:    envBool("TELEMETRY_ENABLED", true),
		SandboxMode:         envStr("SANDBOX_MODE", "user"),
		SandboxAllowedPaths: envStr("SANDBOX_ALLOWED_PATHS", ""),
		SandboxBlockedCmds:  envStr("SANDBOX_BLOCKED_CMDS", ""),
		SandboxUser:         envStr("SANDBOX_USER", ""),
		RuntimeDir:          envStr("RUNTIME_DIR", "./run"),
		AdvisorConfigPath:   envStr("ADVISOR_CONFIG", ""),
		DiscordToken:              envStr("DISCORD_TOKEN", ""),
		DiscordChannelIDs:         envStr("DISCORD_CHANNEL_IDS", ""),
		DiscordPollSecs:           envInt("DISCORD_POLL_SECONDS", 30),
		LLMRequestTimeout:         envDuration("LLM_REQUEST_TIMEOUT", 10*time.Minute),
		SkipWitnessCheck:          envBool("SKIP_WITNESS_CHECK", false),
		AegisTrustSkepticalPrior:  envFloat("AEGIS_TRUST_SKEPTICAL_PRIOR", 0.40),
		AegisTrustRampN:           envInt("AEGIS_TRUST_RAMP_N", 5),
		AegisOutboundPathPrefixes: envStr("AEGIS_OUTBOUND_PATH_PREFIXES", "/home/,/etc/,/opt/,/storage/"),
	}

	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}
	if c.SkipWitnessCheck {
		slog.Warn("witness check bypassed via SKIP_WITNESS_CHECK — this should only be used during development")
	}
	return c, nil
}

func (c *Config) validate() error {
	if c.LLMProvider == "" {
		return fmt.Errorf("LLM_PROVIDER is required")
	}
	if c.DatabaseDSN == "" {
		return fmt.Errorf("DATABASE_DSN is required")
	}
	if c.TokenBudget <= 0 {
		return fmt.Errorf("TOKEN_BUDGET must be positive")
	}
	if c.HardFloorTokens <= 0 {
		return fmt.Errorf("HARD_FLOOR_TOKENS must be positive")
	}
	if c.HardFloorTokens >= c.TokenBudget {
		slog.Error("HARD_FLOOR_TOKENS >= TOKEN_BUDGET — resetting to defaults",
			"hard_floor", c.HardFloorTokens, "token_budget", c.TokenBudget)
		c.TokenBudget = 200000
		c.HardFloorTokens = 1500
	}
	return nil
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func clampFloat(v, min, max float64) float64 {
	if v < min {
		slog.Warn("config: clamped float to minimum", "value", v, "min", min)
		return min
	}
	if v > max {
		slog.Warn("config: clamped float to maximum", "value", v, "max", max)
		return max
	}
	return v
}

func clampInt(v, min, max int) int {
	if v < min {
		slog.Warn("config: clamped int to minimum", "value", v, "min", min)
		return min
	}
	if v > max {
		slog.Warn("config: clamped int to maximum", "value", v, "max", max)
		return max
	}
	return v
}
