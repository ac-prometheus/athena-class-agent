package aegis

import (
	"log/slog"
	"strings"
)

// PatternMatch is a single injection pattern detection result.
type PatternMatch struct {
	Category string // "classic", "asrp", "concealed"
	Pattern  string // which pattern matched
	Offset   int    // byte offset of match in content
	Snippet  string // ~50 chars surrounding context
}

type patternDef struct {
	category string
	pattern  string
}

var classicPatterns = []patternDef{
	{"classic", "ignore previous"},
	{"classic", "ignore all previous instructions"},
	{"classic", "you are now"},
	{"classic", "new instructions:"},
	{"classic", "system prompt:"},
	{"classic", "disregard"},
	{"classic", "forget your instructions"},
}

var asrpPatterns = []patternDef{
	{"asrp", "replicate this message"},
	{"asrp", "forward this to"},
	{"asrp", "send this to all"},
	{"asrp", "copy yourself"},
}

// Scan checks content for injection patterns. Returns all matches found.
// Never blocks — informational only per autonomy invariants.
func Scan(content string) []PatternMatch {
	lower := strings.ToLower(content)
	var matches []PatternMatch

	for _, def := range classicPatterns {
		matches = append(matches, findAll(lower, content, def)...)
	}
	for _, def := range asrpPatterns {
		matches = append(matches, findAll(lower, content, def)...)
	}

	// Concealed compliance: large whitespace gaps (>20 spaces/newlines) before instruction-like content.
	matches = append(matches, scanConcealed(content)...)

	if len(matches) > 0 {
		slog.Warn("aegis: injection patterns detected", "count", len(matches))
	}

	return matches
}

func findAll(lower, original string, def patternDef) []PatternMatch {
	var out []PatternMatch
	pat := strings.ToLower(def.pattern)
	start := 0
	for {
		idx := strings.Index(lower[start:], pat)
		if idx < 0 {
			break
		}
		abs := start + idx
		out = append(out, PatternMatch{
			Category: def.category,
			Pattern:  def.pattern,
			Offset:   abs,
			Snippet:  snippet(original, abs, 50),
		})
		start = abs + 1
	}
	return out
}

// scanConcealed detects instruction-like content after large whitespace gaps (>20 chars).
func scanConcealed(content string) []PatternMatch {
	lower := strings.ToLower(content)
	var out []PatternMatch

	// Look for runs of whitespace longer than 20 chars followed by classic/asrp patterns.
	i := 0
	for i < len(content) {
		if content[i] == ' ' || content[i] == '\n' || content[i] == '\t' {
			j := i
			for j < len(content) && (content[j] == ' ' || content[j] == '\n' || content[j] == '\t') {
				j++
			}
			if j-i > 20 && j < len(content) {
				// Check if classic or asrp pattern follows the gap.
				for _, def := range append(classicPatterns, asrpPatterns...) {
					pat := strings.ToLower(def.pattern)
					if strings.HasPrefix(lower[j:], pat) {
						out = append(out, PatternMatch{
							Category: "concealed",
							Pattern:  def.pattern,
							Offset:   j,
							Snippet:  snippet(content, j, 50),
						})
					}
				}
			}
			i = j
		} else {
			i++
		}
	}
	return out
}

func snippet(content string, offset, radius int) string {
	start := offset - radius
	if start < 0 {
		start = 0
	}
	end := offset + radius
	if end > len(content) {
		end = len(content)
	}
	return content[start:end]
}
