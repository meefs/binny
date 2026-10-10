package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
)

func newTestModel() *Model {
	ti := textinput.New()
	return &Model{input: ti, histIdx: -1}
}

func TestSlashMatches(t *testing.T) {
	m := newTestModel()

	m.input.SetValue("/mo")
	got := m.slashMatches()
	// /model, /models, /mode all start with /mo
	if len(got) != 3 {
		t.Fatalf("/mo → expected 3 matches, got %d (%v)", len(got), names(got))
	}

	m.input.SetValue("/model")
	if got := m.slashMatches(); len(got) != 2 { // /model, /models
		t.Fatalf("/model → expected 2 matches, got %d (%v)", len(got), names(got))
	}

	// A space means the command word is done → menu closes.
	m.input.SetValue("/model gpt-4o")
	if got := m.slashMatches(); got != nil {
		t.Fatalf("args typed → expected no menu, got %v", names(got))
	}

	// Non-slash text never opens the menu.
	m.input.SetValue("hello")
	if got := m.slashMatches(); got != nil {
		t.Fatalf("plain text → expected no menu, got %v", names(got))
	}

	// Dismissed hides it even with a valid prefix.
	m.input.SetValue("/mo")
	m.menuDismissed = true
	if got := m.slashMatches(); got != nil {
		t.Fatalf("dismissed → expected no menu, got %v", names(got))
	}
}

func TestHistoryNavigation(t *testing.T) {
	m := newTestModel()
	m.history = []string{"first", "second", "third"}

	// Typing a partial line, then ↑ stashes it and walks back from newest.
	m.input.SetValue("draf")
	m.historyPrev()
	if m.input.Value() != "third" {
		t.Fatalf("first ↑ → want 'third', got %q", m.input.Value())
	}
	m.historyPrev()
	if m.input.Value() != "second" {
		t.Fatalf("second ↑ → want 'second', got %q", m.input.Value())
	}
	// ↓ back forward, past the newest restores the stashed in-progress line.
	m.historyNext()
	if m.input.Value() != "third" {
		t.Fatalf("↓ → want 'third', got %q", m.input.Value())
	}
	m.historyNext()
	if m.input.Value() != "draf" {
		t.Fatalf("↓ past newest → want stashed 'draf', got %q", m.input.Value())
	}
	if m.histIdx != -1 {
		t.Fatalf("nav should have reset, histIdx=%d", m.histIdx)
	}
}

func TestPushHistoryDedupesConsecutive(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // don't touch the real history file
	m := newTestModel()
	m.history = []string{"a"}
	m.pushHistory("a") // consecutive dupe — ignored
	if len(m.history) != 1 {
		t.Fatalf("consecutive dupe should not grow history: %v", m.history)
	}
	m.pushHistory("b")
	if len(m.history) != 2 || m.history[1] != "b" {
		t.Fatalf("distinct push should append: %v", m.history)
	}
}

// TestSlashMenuScrollsToSelection guards the regression where arrowing past the
// visible window (maxShow=8) lost the highlight and hid the entries below it:
// the rendered menu must always contain the selected command and show a "more
// above/below" indicator when the list overflows the window.
func TestSlashMenuScrollsToSelection(t *testing.T) {
	m := newTestModel()
	m.input.SetValue("/") // matches every command — comfortably more than maxShow
	all := m.slashMatches()
	if len(all) <= 8 {
		t.Fatalf("test needs >8 commands to exercise scrolling, got %d", len(all))
	}

	// Select the last command (well past the 8-item window).
	m.menuIdx = len(all) - 1
	view := m.viewSlashMenu()
	last := all[len(all)-1].name
	if !strings.Contains(view, last) {
		t.Errorf("selected last item %q not visible in menu:\n%s", last, view)
	}
	// The selection must actually be highlighted (▸), not just present.
	if !strings.Contains(view, "▸ "+last) {
		t.Errorf("selected item %q is not highlighted:\n%s", last, view)
	}
	// Items scrolled off the top get an "↑ N more" indicator.
	if !strings.Contains(view, "↑ ") {
		t.Errorf("expected an ↑ more indicator when scrolled to the bottom:\n%s", view)
	}

	// At the top, the first item is highlighted and a "↓ N more" indicator shows.
	m.menuIdx = 0
	view = m.viewSlashMenu()
	if !strings.Contains(view, "▸ "+all[0].name) {
		t.Errorf("first item not highlighted at top:\n%s", view)
	}
	if !strings.Contains(view, "↓ ") {
		t.Errorf("expected a ↓ more indicator at the top of an overflowing list:\n%s", view)
	}
}

func names(cs []slashCmd) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.name
	}
	return out
}
