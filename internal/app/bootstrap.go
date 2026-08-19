package app

import (
	"os"
	"strings"
)

// DriverNameFromDSN infers the database driver from the DSN string.
func DriverNameFromDSN(dsn string) string {
	if dsn == "" {
		return "sqlite3"
	}
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		return "postgres"
	}
	return "sqlite3"
}

// ProfileFromEnv reads the AGENT_PROFILE environment variable and returns
// the corresponding RuntimeProfile. Empty defaults to ProfileDevelopment.
func ProfileFromEnv() (RuntimeProfile, error) {
	return ParseProfile(os.Getenv("AGENT_PROFILE"))
}
