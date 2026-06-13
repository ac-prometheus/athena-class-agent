package memory

import (
	"crypto/rand"
	"fmt"
)

// newID generates a random UUID v4 string using crypto/rand.
// Used wherever the caller hasn't supplied an ID and we need to generate one.
// Format: 8-4-4-4-12 hex digits separated by hyphens.
func newID() string {
	var b [16]byte
	_, err := rand.Read(b[:])
	if err != nil {
		// crypto/rand failure is a catastrophic system condition, not a logic error.
		panic(fmt.Sprintf("memory: crypto/rand unavailable: %v", err))
	}
	// Set UUID v4 version and variant bits per RFC 4122.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
