package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Local presets are ready without a key, and a config written before they
// shipped gets them appended on load without touching the user's entries.
func TestLocalProviders(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	for _, id := range LocalProviderIDs() {
		p := DefaultConfig().Find(id)
		if p == nil || !p.Local || !p.HasKey() || !p.Keyless() || p.Kind != KindOpenAI {
			t.Fatalf("%s: bad preset %+v", id, p)
		}
	}
	if !DefaultConfig().Find("ollama").HasKey() || DefaultConfig().Find("openai").HasKey() {
		t.Fatal("HasKey: ollama should be ready keyless, openai should not")
	}
	if DefaultConfig().HasConfiguredKey() {
		t.Fatal("a local preset must not count as a configured key (first-run setup would be skipped)")
	}

	// An old config: one provider the user edited, no local presets.
	old := Config{DefaultProvider: "openai", DefaultModel: "my-model", Providers: []Provider{
		{ID: "openai", Kind: KindOpenAI, BaseURL: "https://router.example.com/v1", Models: []string{"my-model"}},
	}}
	raw, _ := json.Marshal(old)
	path, err := BinnyConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	c, firstRun, err := Load()
	if err != nil || firstRun {
		t.Fatalf("load: err=%v firstRun=%v", err, firstRun)
	}
	if c.Providers[0].ID != "openai" || c.Providers[0].BaseURL != "https://router.example.com/v1" {
		t.Fatalf("user entry changed: %+v", c.Providers[0])
	}
	if got := c.Find("ollama"); got == nil || !got.Local || got.BaseURL != "http://localhost:11434/v1" {
		t.Fatalf("ollama not appended: %+v", got)
	}
	if got := c.Find("lmstudio"); got == nil || !got.Local {
		t.Fatalf("lmstudio not appended: %+v", got)
	}
	// Appended after the user's entries, and only once.
	if c.Providers[0].ID != "openai" || len(c.Providers) != 3 {
		t.Fatalf("unexpected registry: %d providers, first=%s", len(c.Providers), c.Providers[0].ID)
	}
	migrate(c)
	if len(c.Providers) != 3 {
		t.Fatalf("migrate re-appended: %d providers", len(c.Providers))
	}

	// A local server moved to another host keeps working keyless.
	if err := c.SetBaseURL("ollama", "http://10.0.0.5:11434/v1"); err != nil {
		t.Fatal(err)
	}
	if p := c.Find("ollama"); !p.HasKey() || p.BaseURL != "http://10.0.0.5:11434/v1" {
		t.Fatalf("after endpoint change: %+v", p)
	}
}
