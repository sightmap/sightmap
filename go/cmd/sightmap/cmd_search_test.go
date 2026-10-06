package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// search covers file-root definitions (SEP-0019): their fields are authored
// only there, since views hold just $refs to them.
func TestSearchFindsDefinitions(t *testing.T) {
	dir := t.TempDir()
	corpus := `version: 1
definitions:
  - name: ProductCard
    selector: '[data-component="ProductCard"]'
    memory: [lifted price lives on the card]
`
	if err := os.WriteFile(filepath.Join(dir, "components.yaml"), []byte(corpus), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"ProductCard", "lifted price"} {
		out := captureSearchStdout(t, func() error { return runSearch([]string{"--sightmap-dir", dir, q}) })
		if !strings.Contains(out, "[definitions] › ProductCard") {
			t.Errorf("search %q: definition not found; got:\n%s", q, out)
		}
	}
}

func captureSearchStdout(t *testing.T, fn func() error) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	runErr := fn()
	os.Stdout = orig
	w.Close()
	data, _ := io.ReadAll(r)
	if runErr != nil {
		t.Fatal(runErr)
	}
	return string(data)
}
