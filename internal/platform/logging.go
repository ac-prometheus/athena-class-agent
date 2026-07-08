package platform

import (
	"log/slog"
	"os"
)

// NewLogger creates a structured JSON logger writing to stdout.
// Use slog.SetDefault to make this the package-level logger.
func NewLogger(level slog.Level) *slog.Logger {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(h)
}

// LogLevel parses a level string ("debug", "info", "warn", "error").
// Returns slog.LevelInfo on unrecognised input.
func LogLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
