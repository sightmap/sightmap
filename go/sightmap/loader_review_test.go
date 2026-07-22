package sightmap

import (
	"os"
	"path/filepath"
	"testing"
)

// A review/ punch-list file is a YAML sequence, not a corpus mapping. The corpus
// loader must skip the review/ directory so such files don't break compilation
// (regression for the homepage__base.punch.yaml "cannot unmarshal !!seq" error).
func TestLoadDir_IgnoresReviewPunchFiles(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "components.yaml"),
		[]byte("components:\n  - name: Foo\n    selector: '.foo'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	reviewDir := filepath.Join(dir, "review")
	if err := os.MkdirAll(reviewDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reviewDir, "home__base.punch.yaml"),
		[]byte("- id: a1b2\n  category: note\n  status: open\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	corpus, err := loadDir(dir)
	if err != nil {
		t.Fatalf("loadDir should ignore review/ punch files, got: %v", err)
	}
	if len(corpus.GlobalComponents) != 1 || corpus.GlobalComponents[0].Name != "Foo" {
		t.Fatalf("expected 1 global component Foo, got %+v", corpus.GlobalComponents)
	}
}
