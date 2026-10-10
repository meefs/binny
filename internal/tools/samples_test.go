package tools

import (
	"strings"
	"testing"

	"openbin.ai/binny/internal/api"
)

func sampleFixture() []api.Sample {
	return []api.Sample{
		{ID: "aaaa-1", Label: "payload", OriginalFilename: "stage2.bin", Status: "READY", Sha256: "deadbeef01"},
		{ID: "bbbb-2", Label: "", OriginalFilename: "loader.dll", Status: "READY", Sha256: "cafef00d02"},
		{ID: "cccc-3", Label: "pending one", OriginalFilename: "x.elf", Status: "INGEST_PENDING"},
	}
}

func TestResolveSample(t *testing.T) {
	ss := sampleFixture()
	cases := []struct {
		q      string
		wantID string // "" = primary (nil)
		err    bool
	}{
		{"", "", false},
		{"primary", "", false},
		{"0", "", false},
		{"1", "aaaa-1", false},
		{"3", "cccc-3", false},
		{"4", "", true},
		{"payload", "aaaa-1", false},
		{"PAYLOAD", "aaaa-1", false},
		{"loader.dll", "bbbb-2", false},
		{"bbbb-2", "bbbb-2", false},
		{"cafef0", "bbbb-2", false}, // sha prefix
		{"loader", "bbbb-2", false}, // unique substring
		{"zzz", "", true},
	}
	for _, c := range cases {
		got, err := ResolveSample(ss, c.q)
		if c.err {
			if err == nil {
				t.Errorf("%q: expected error, got %v", c.q, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: unexpected error %v", c.q, err)
			continue
		}
		gotID := ""
		if got != nil {
			gotID = got.ID
		}
		if gotID != c.wantID {
			t.Errorf("%q: want %q got %q", c.q, c.wantID, gotID)
		}
	}
}

func TestResolveSampleAmbiguous(t *testing.T) {
	ss := []api.Sample{
		{ID: "1", Label: "stage-a", Status: "READY"},
		{ID: "2", Label: "stage-b", Status: "READY"},
	}
	_, err := ResolveSample(ss, "stage")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
}

func TestSwitchSampleRefusesNotReady(t *testing.T) {
	tb := NewToolbox(nil, &api.Project{ID: "p1", Kind: api.KindBIN, Status: "READY"})
	s := &api.Sample{ID: "s1", Label: "pending", Status: "INGEST_PENDING"}
	if _, err := tb.SwitchSample(s); err == nil {
		t.Fatal("expected refusal for a non-READY sample")
	}
	if tb.ActiveSample() != nil {
		t.Fatal("failed switch must leave the primary active")
	}
}

func TestCacheKeyPerSample(t *testing.T) {
	tb := NewToolbox(nil, &api.Project{ID: "p1", Kind: api.KindBIN})
	if tb.cacheKey() != "p1" {
		t.Fatalf("primary key: %q", tb.cacheKey())
	}
	tb.sample = &api.Sample{ID: "s9"}
	if tb.cacheKey() != "p1:s9" {
		t.Fatalf("sample key: %q", tb.cacheKey())
	}
}
