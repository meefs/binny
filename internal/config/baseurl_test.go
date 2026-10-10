package config

import (
	"strings"
	"testing"
)

func TestSetBaseURL(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	c := DefaultConfig()

	if err := c.SetBaseURL("anthropic", "https://anthropic.example.com/"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got := c.Find("anthropic").BaseURL; got != "https://anthropic.example.com" {
		t.Fatalf("trailing slash should be trimmed, got %q", got)
	}

	// Persisted: a fresh Load sees the override and the default is still known.
	loaded, firstRun, err := Load()
	if err != nil || firstRun {
		t.Fatalf("load: err=%v firstRun=%v", err, firstRun)
	}
	if got := loaded.Find("anthropic").BaseURL; got != "https://anthropic.example.com" {
		t.Fatalf("override not persisted, got %q", got)
	}
	if DefaultBaseURL("anthropic") != "https://api.anthropic.com" {
		t.Fatalf("shipped default changed: %q", DefaultBaseURL("anthropic"))
	}

	// Compat roots keep their path: /v1 is part of the root for OpenAI-style APIs.
	if err := c.SetBaseURL("openai", "http://localhost:8080/v1"); err != nil {
		t.Fatalf("localhost http should be allowed for self-host: %v", err)
	}
	if got := c.Find("openai").BaseURL; got != "http://localhost:8080/v1" {
		t.Fatalf("path dropped, got %q", got)
	}

	bad := map[string]string{
		"no scheme":    "anthropic.example.com",
		"query":        "https://x.example.com/?k=v",
		"bad scheme":   "ftp://x.example.com",
		"empty":        "",
		"unknown prov": "whatever",
	}
	for name, in := range bad {
		id := "anthropic"
		if name == "unknown prov" {
			id = "nope"
		}
		if err := c.SetBaseURL(id, in); err == nil {
			t.Errorf("%s: expected error for %q", name, in)
		}
	}
	if err := c.SetBaseURL("bedrock", "https://x.example.com"); err == nil || !strings.Contains(err.Error(), "AWS") {
		t.Errorf("bedrock should be rejected with an AWS hint, got %v", err)
	}
}
