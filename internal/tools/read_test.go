package tools

import (
	"bytes"
	"testing"
)

func TestDecodeHexPreview(t *testing.T) {
	// The worker emits space-separated hex pairs, with a trailing "..." when the
	// symbol is longer than the captured window.
	got := decodeHexPreview("31 39 32 2e 31 36 38 ...")
	want := []byte("192.168")
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := decodeHexPreview(""); got != nil {
		t.Fatalf("empty preview should decode to nil, got %v", got)
	}
}

func TestSameAddr(t *testing.T) {
	matches := [][2]string{
		{"00400a86", "0x00400a86"}, // stored bare vs model's 0x form (the real bug)
		{"0x400A86", "00400a86"},   // prefix + case + leading-zero drift
		{"400a86", "0x400a86"},
		{"00000000", "0x0"},
		{" 0x401000 ", "401000"}, // surrounding whitespace
	}
	for _, m := range matches {
		if !sameAddr(m[0], m[1]) {
			t.Errorf("sameAddr(%q, %q) = false, want true", m[0], m[1])
		}
	}
	nonMatches := [][2]string{
		{"00400a86", "00400a87"},
		{"0x401000", "0x402000"},
	}
	for _, m := range nonMatches {
		if sameAddr(m[0], m[1]) {
			t.Errorf("sameAddr(%q, %q) = true, want false", m[0], m[1])
		}
	}
}
