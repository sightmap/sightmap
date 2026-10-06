package viewset

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnapHeaderReaders(t *testing.T) {
	dir := t.TempDir()
	snap := filepath.Join(dir, "x.snap")
	body := "[View: SearchView]\nroute: /s/:query\nurl: https://example.com/s/grommets?NCNI-5\n\n--- component tree ---\n\nurl: https://not.the.header/\n"
	if err := os.WriteFile(snap, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ViewNameOf(snap); got != "SearchView" {
		t.Errorf("ViewNameOf = %q", got)
	}
	if got := RouteOf(snap); got != "/s/:query" {
		t.Errorf("RouteOf = %q", got)
	}
	if got := URLOf(snap); got != "https://example.com/s/grommets?NCNI-5" {
		t.Errorf("URLOf = %q", got)
	}
	old := filepath.Join(dir, "old.snap")
	if err := os.WriteFile(old, []byte("[View: V]\nroute: /\n\n--- component tree ---\n\nurl: https://not.the.header/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := URLOf(old); got != "" {
		t.Errorf("URLOf on a capture without a url line = %q, want empty", got)
	}
}
