package viewset

import (
	"os"
	"path/filepath"
	"testing"
)

func TestViewSnapshotSet(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "snapshots", "home")
	if err := os.MkdirAll(filepath.Join(home, "stale-subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(rel string) {
		p := filepath.Join(home, rel)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Given out of order, with .tree.json siblings that must NOT become entries.
	write("20260607T193000Z.snap")
	write("20260607T193000Z.snap.tree.json")
	write("20260607T090000Z.snap")
	write("20260607T090000Z.snap.tree.json")
	write("20260607T140000Z.snap")

	set := Set(dir, "home")
	if len(set) != 3 {
		t.Fatalf("got %d entries, want 3 (.snap only, subdir excluded): %+v", len(set), set)
	}
	// Must be ordered oldest→newest by stamp.
	wantStamps := []string{"20260607T090000Z", "20260607T140000Z", "20260607T193000Z"}
	for i, e := range set {
		if e.Stamp != wantStamps[i] {
			t.Errorf("entry %d stamp = %q, want %q", i, e.Stamp, wantStamps[i])
		}
	}
}

func TestViewSnapshotSetMissing(t *testing.T) {
	if set := Set(t.TempDir(), "nope"); len(set) != 0 {
		t.Errorf("missing view set = %+v, want empty", set)
	}
}
