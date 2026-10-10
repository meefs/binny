package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectFilePath(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "sample.bin")
	if err := os.WriteFile(f, []byte("MZ..."), 0o600); err != nil {
		t.Fatal(err)
	}

	// A path embedded in a sentence is found.
	if got := detectFilePath("the raw sample is at " + f + " thanks"); got != f {
		t.Errorf("embedded path: got %q, want %q", got, f)
	}
	// Quoted path.
	if got := detectFilePath(`here: "` + f + `"`); got != f {
		t.Errorf("quoted path: got %q, want %q", got, f)
	}
	// A directory is not a sample.
	if got := detectFilePath(dir); got != "" {
		t.Errorf("directory should not match, got %q", got)
	}
	// Plain prose with no real file matches nothing.
	if got := detectFilePath("what is the question exactly"); got != "" {
		t.Errorf("prose should not match, got %q", got)
	}
	// A slashless word is never stat'd/matched even if it exists as a name.
	if got := detectFilePath("sample.bin"); got != "" {
		t.Errorf("bare word should not match, got %q", got)
	}
}

func TestLooksPathish(t *testing.T) {
	for _, s := range []string{"~/x", "/etc/hosts", "a/b", "./x"} {
		if !looksPathish(s) {
			t.Errorf("%q should look pathish", s)
		}
	}
	for _, s := range []string{"hello", "sample.bin", "what?"} {
		if looksPathish(s) {
			t.Errorf("%q should NOT look pathish", s)
		}
	}
}
