package aegis

import (
	"log/slog"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Sanitize applies NFKC normalization and strips invisible/directional characters.
// Returns (normalized string, true) if any modifications were made.
func Sanitize(raw []byte) (normalized string, modified bool) {
	nfkc := norm.NFKC.String(string(raw))

	var b strings.Builder
	b.Grow(len(nfkc))
	stripped := false

	for _, r := range nfkc {
		if isInvisible(r) {
			stripped = true
			continue
		}
		b.WriteRune(r)
	}

	result := b.String()
	if stripped {
		slog.Debug("aegis: stripped invisible/directional characters", "original_len", len(raw), "result_len", len(result))
	}

	return result, stripped || nfkc != string(raw)
}

// isInvisible reports whether r is a zero-width or directional override character.
func isInvisible(r rune) bool {
	switch r {
	case 0x200B, // ZERO WIDTH SPACE
		0x200C, // ZERO WIDTH NON-JOINER
		0x200D, // ZERO WIDTH JOINER
		0xFEFF, // BOM / ZERO WIDTH NO-BREAK SPACE
		0x2060: // WORD JOINER
		return true
	}
	// Directional overrides U+202A-U+202E
	if r >= 0x202A && r <= 0x202E {
		return true
	}
	// Directional isolates U+2066-U+2069
	if r >= 0x2066 && r <= 0x2069 {
		return true
	}
	return false
}
