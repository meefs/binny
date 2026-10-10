package store

import (
	"bytes"
	"os"
	"testing"
)

// withTempRoot points the store at a throwaway XDG_DATA_HOME.
func withTempRoot(t *testing.T) {
	t.Helper()
	old := os.Getenv("XDG_DATA_HOME")
	if err := os.Setenv("XDG_DATA_HOME", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("XDG_DATA_HOME", old) })
}

func TestSaveLoadRoundtrip(t *testing.T) {
	withTempRoot(t)
	sha := "abc123def4567890"
	blob := []byte(`{"functions":[],"metadata":{"language":"ARM:LE:32:v7"}}`)
	meta := Meta{Sha256: sha, Name: "fw.bin", SamplePath: "/tmp/fw.bin", SizeBytes: 42, Processor: "ARM:LE:32:v7"}

	if Has(sha) {
		t.Fatal("Has before Save")
	}
	if err := Save(meta, blob); err != nil {
		t.Fatal(err)
	}
	if !Has(sha) {
		t.Fatal("Has after Save = false")
	}
	got, err := Blob(sha)
	if err != nil || !bytes.Equal(got, blob) {
		t.Fatalf("Blob roundtrip failed: %v", err)
	}
	m, err := Load(sha)
	if err != nil || m.Name != "fw.bin" || m.Processor != "ARM:LE:32:v7" || m.CreatedAt == "" {
		t.Fatalf("Load = %+v, %v", m, err)
	}
	list := List()
	if len(list) != 1 || list[0].Sha256 != sha {
		t.Fatalf("List = %+v", list)
	}
}

func TestRemoteCache(t *testing.T) {
	withTempRoot(t)
	id := "0b7f3c6e-project"
	if RemoteBlob(id) != nil {
		t.Fatal("cache hit before save")
	}
	SaveRemote(id, []byte(`{"x":1}`))
	if got := RemoteBlob(id); string(got) != `{"x":1}` {
		t.Fatalf("RemoteBlob = %q", got)
	}
	// Remote entries must not leak into the local List.
	if l := List(); len(l) != 0 {
		t.Fatalf("List sees remote cache: %+v", l)
	}
	DropRemote(id)
	if RemoteBlob(id) != nil {
		t.Fatal("cache hit after drop")
	}
}

func TestLocalID(t *testing.T) {
	if LocalID("abcdef0123456789ff") != "local:abcdef012345" {
		t.Fatalf("LocalID = %q", LocalID("abcdef0123456789ff"))
	}
}
