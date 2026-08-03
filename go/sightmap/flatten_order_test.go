package sightmap

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The flattened corpus is emitted in a stable, deterministic order: corpus files
// in lexical path order, components in declaration order, and each parent
// immediately before its flattened children (pre-order). That ordering underpins
// a reproducible wire form, so lock it against accidental reordering.
func TestLoadDir_DeterministicPreOrder(t *testing.T) {
	dir := t.TempDir()
	// Lexical filenames: a-comps sorts before b-comps.
	if err := os.WriteFile(filepath.Join(dir, "a-comps.yaml"), []byte(`
version: 1
components:
  - name: Alpha
    selector: .a
    children:
      - name: AlphaOne
        selector: .a1
      - name: AlphaTwo
        selector: .a2
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b-comps.yaml"), []byte(`
version: 1
components:
  - name: Beta
    selector: .b
`), 0o644); err != nil {
		t.Fatal(err)
	}

	corpus, err := loadDir(dir)
	if err != nil {
		t.Fatalf("loadDir: %v", err)
	}

	var got []string
	for _, c := range corpus.GlobalComponents {
		got = append(got, c.Name)
	}
	// a-comps before b-comps (file order); Alpha before its children (pre-order);
	// AlphaOne before AlphaTwo (declaration order).
	want := []string{"Alpha", "AlphaOne", "AlphaTwo", "Beta"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("flatten order = %v, want %v", got, want)
	}
}
