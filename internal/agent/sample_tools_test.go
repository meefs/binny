package agent

import (
	"strings"
	"testing"

	"openbin.ai/binny/internal/api"
)

func TestFormatSamplesMarksActive(t *testing.T) {
	p := &api.Project{Name: "dropper.exe", Arch: "x86", ExecutableFormat: "PE"}
	ss := []api.Sample{
		{ID: "a", Label: "payload", OriginalFilename: "stage2.bin", Status: "READY", Arch: "x86-64", ExecutableFormat: "PE", Sha256: "0123456789abcdef"},
		{ID: "b", OriginalFilename: "cfg.elf", Status: "INGEST_PENDING"},
	}
	out := FormatSamples(p, ss, &ss[0])
	if !strings.Contains(out, "  primary  dropper.exe") {
		t.Errorf("primary should be unmarked when an extra is active:\n%s", out)
	}
	if !strings.Contains(out, "▸ #1") || !strings.Contains(out, "payload (stage2.bin)") {
		t.Errorf("active extra should be marked with its label+filename:\n%s", out)
	}
	if !strings.Contains(out, "[INGEST_PENDING]") {
		t.Errorf("non-ready sample should show its status:\n%s", out)
	}
	out = FormatSamples(p, nil, nil)
	if !strings.Contains(out, "▸ primary") || !strings.Contains(out, "no extra samples") {
		t.Errorf("single-binary project:\n%s", out)
	}
}

func TestSwitchSummary(t *testing.T) {
	p := &api.Project{Name: "dropper.exe"}
	if got := SwitchSummary(p, nil, nil); !strings.Contains(got, "PRIMARY") || !strings.Contains(got, "dropper.exe") {
		t.Errorf("primary summary: %s", got)
	}
	a := &api.Analysis{}
	a.Metadata.FunctionCount = 42
	if got := SwitchSummary(p, &api.Sample{Label: "payload"}, a); !strings.Contains(got, "42 functions") || !strings.Contains(got, "\"payload\"") {
		t.Errorf("sample summary: %s", got)
	}
}
