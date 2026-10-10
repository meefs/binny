package resume

import (
	"testing"

	"openbin.ai/binny/internal/agent"
)

func TestSaveLoadDelete(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	const pid = "proj-123"
	if Exists(pid) {
		t.Fatal("no session should exist yet")
	}

	in := State{
		ProjectID:   pid,
		ProjectName: "sample",
		Mode:        "MALWARE",
		Autonomy:    "SUPERVISED",
		ProviderID:  "openai",
		Model:       "gpt-4o",
		Engine: agent.EngineState{
			Stage:       1,
			Rounds:      5,
			SpentTokens: 42000,
			Investigation: &agent.Investigation{
				MalwareTypes: []string{"ransomware"},
			},
		},
		Transcript:    []Line{{Role: 2, Text: "hello"}},
		EngineStarted: true,
	}
	if err := Save(in); err != nil {
		t.Fatalf("save: %v", err)
	}
	if !Exists(pid) {
		t.Fatal("session should exist after save")
	}

	got, ok, err := Load(pid)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.Version != stateVersion {
		t.Errorf("version = %d, want %d", got.Version, stateVersion)
	}
	if got.SavedAt == "" {
		t.Error("savedAt not stamped")
	}
	if got.ProjectName != "sample" || got.Engine.Rounds != 5 || got.Engine.SpentTokens != 42000 {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if got.Engine.Investigation == nil || len(got.Engine.Investigation.MalwareTypes) != 1 {
		t.Errorf("investigation not preserved: %+v", got.Engine.Investigation)
	}
	if len(got.Transcript) != 1 || got.Transcript[0].Text != "hello" {
		t.Errorf("transcript not preserved: %+v", got.Transcript)
	}

	if err := Delete(pid); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if Exists(pid) {
		t.Fatal("session should be gone after delete")
	}
}
