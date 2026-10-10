package llm

import "testing"

func TestParseModelList(t *testing.T) {
	// The shared {"data":[{"id":...}]} shape (Anthropic + OpenAI-compatible),
	// with an empty id and a duplicate that must be dropped.
	body := []byte(`{"data":[{"id":"gpt-4o"},{"id":""},{"id":"o3-mini"},{"id":"gpt-4o"}]}`)
	got := parseModelList(body)
	want := []string{"gpt-4o", "o3-mini"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("index %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseModelListRejectsGarbage(t *testing.T) {
	if got := parseModelList([]byte(`not json`)); got != nil {
		t.Fatalf("expected nil on garbage, got %v", got)
	}
	if got := parseModelList([]byte(`{"data":"nope"}`)); len(got) != 0 {
		t.Fatalf("expected empty on non-array data, got %v", got)
	}
}
