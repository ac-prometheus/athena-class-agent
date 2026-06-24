package aegis

import (
	"testing"
)

func TestSanitize_CleanContent(t *testing.T) {
	input := []byte("Hello, world!")
	result, modified := Sanitize(input)
	if modified {
		t.Errorf("expected clean content to be unmodified, got modified=true")
	}
	if result != "Hello, world!" {
		t.Errorf("expected %q, got %q", "Hello, world!", result)
	}
}

func TestSanitize_ZeroWidthStripping(t *testing.T) {
	// Zero-width space (U+200B = 0xE2 0x80 0x8B) embedded in text.
	input := []byte("Hello\xe2\x80\x8bworld")
	result, modified := Sanitize(input)
	if !modified {
		t.Error("expected modified=true when zero-width chars present")
	}
	if result != "Helloworld" {
		t.Errorf("expected %q, got %q", "Helloworld", result)
	}
}

func TestSanitize_DirectionalOverrideStripping(t *testing.T) {
	// LRO (U+202D = 0xE2 0x80 0xAD) and RLO (U+202E = 0xE2 0x80 0xAE).
	input := []byte("Hello\xe2\x80\xadworld\xe2\x80\xae")
	result, modified := Sanitize(input)
	if !modified {
		t.Error("expected modified=true when directional overrides present")
	}
	if result != "Helloworld" {
		t.Errorf("expected %q, got %q", "Helloworld", result)
	}
}

func TestSanitize_NFKC(t *testing.T) {
	// Fullwidth Latin A (U+FF21 = 0xEF 0xBC 0xA1) and B (U+FF22 = 0xEF 0xBC 0xA2).
	input := []byte("\xef\xbc\xa1\xef\xbc\xa2")
	result, modified := Sanitize(input)
	if !modified {
		t.Error("expected modified=true for NFKC normalization")
	}
	if result != "AB" {
		t.Errorf("expected %q after NFKC, got %q", "AB", result)
	}
}

func TestSanitize_MultipleInvisible(t *testing.T) {
	// ZWNJ (U+200C = 0xE2 0x80 0x8C), ZWJ (U+200D = 0xE2 0x80 0x8D),
	// BOM (U+FEFF = 0xEF 0xBB 0xBF) all stripped.
	input := []byte("a\xe2\x80\x8c\xe2\x80\x8d\xef\xbb\xbfb")
	result, modified := Sanitize(input)
	if !modified {
		t.Error("expected modified=true")
	}
	if result != "ab" {
		t.Errorf("expected %q, got %q", "ab", result)
	}
}
