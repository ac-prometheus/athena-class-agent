package aegis

import (
	"log/slog"
	"regexp"
	"strings"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

var (
	uuidRe      = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	sessionIDRe = regexp.MustCompile(`sess_[0-9a-zA-Z]{16,}|session[-_][0-9a-zA-Z]{16,}`)
)

var apiKeyPrefixes = []string{
	"sk-",
	"Bearer ",
	"token=",
	"api_key=",
	"ANTHROPIC_API_KEY",
	"OPENAI_API_KEY",
	"API_KEY=",
	"SECRET_KEY=",
}

var dsnPrefixes = []string{
	"postgres://",
	"postgresql://",
	"sqlite://",
	"sqlite3://",
	"mysql://",
}

var defaultPathPrefixes = []string{
	"/home/",
	"/etc/",
	"/opt/",
	"/storage/",
}

// ScanOutbound checks content for sensitive data that should not leave the agent.
// Returns a report — never blocks, alert only per autonomy invariants.
func ScanOutbound(content string) *pkg.OutboundReport {
	return ScanOutboundWithPaths(content, defaultPathPrefixes)
}

// ScanOutboundWithPaths allows configuring the path prefix list.
func ScanOutboundWithPaths(content string, pathPrefixes []string) *pkg.OutboundReport {
	var findings []string

	for _, prefix := range apiKeyPrefixes {
		if strings.Contains(content, prefix) {
			findings = append(findings, "possible API key or credential: contains "+prefix)
		}
	}

	if uuidRe.MatchString(strings.ToLower(content)) {
		findings = append(findings, "UUID pattern detected")
	}

	for _, prefix := range pathPrefixes {
		if strings.Contains(content, prefix) {
			findings = append(findings, "file path prefix detected: "+prefix)
		}
	}

	if sessionIDRe.MatchString(content) {
		findings = append(findings, "session ID pattern detected")
	}

	for _, prefix := range dsnPrefixes {
		if strings.Contains(content, prefix) {
			findings = append(findings, "DSN/connection string detected: "+prefix)
		}
	}

	if len(findings) > 0 {
		slog.Warn("aegis: outbound scan findings", "count", len(findings))
	}

	return &pkg.OutboundReport{
		Clean:    len(findings) == 0,
		Findings: findings,
	}
}
