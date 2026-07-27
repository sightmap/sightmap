package sightmap_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/sightmap/sightmap/go/sightmap"
)

// TestCircularRefDoesNotHang guards the DoS fix: a mutually-recursive $ref chain
// must not make Load spin forever, and Validate must report it as ref-circular.
func TestCircularRefDoesNotHang(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "components.yaml"), `
version: 1
components:
  - name: A
    selector: ".a"
    children:
      - $ref: B
  - name: B
    selector: ".b"
    children:
      - $ref: A
`)

	type result struct {
		c   *sightmap.Corpus
		err error
	}
	done := make(chan result, 1)
	go func() {
		c, err := sightmap.DirLoader(dir).Load()
		done <- result{c, err}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("Load returned error: %v", r.err)
		}
		errs := sightmap.Validate(r.c)
		found := false
		for _, e := range errs {
			if e.Code == "ref-circular" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected a ref-circular validation error, got %v", errs)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Load hung on a circular $ref chain")
	}
}

// TestSelfRefDoesNotHang covers the single-component self reference.
func TestSelfRefDoesNotHang(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "components.yaml"), `
version: 1
components:
  - name: Loop
    selector: ".loop"
    children:
      - $ref: Loop
`)

	done := make(chan *sightmap.Corpus, 1)
	go func() {
		c, _ := sightmap.DirLoader(dir).Load()
		done <- c
	}()
	select {
	case c := <-done:
		errs := sightmap.Validate(c)
		found := false
		for _, e := range errs {
			if e.Code == "ref-circular" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected ref-circular for self reference, got %v", errs)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Load hung on a self-referential $ref")
	}
}

// TestSplitSelectorsBracketsAndQuotes verifies that a comma inside an attribute
// selector or a quoted string is not treated as a selector-list separator.
func TestSplitSelectorsBracketsAndQuotes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "views.yaml"), `
version: 1
views:
  - name: V
    route: /v
    components:
      - name: AttrComma
        selector: '[data-x="a,b"]'
      - name: NthChild
        selector: 'li:nth-child(2n, 3n)'
      - name: RealList
        selector:
          - '.first'
          - '.second'
`)

	c, err := sightmap.DirLoader(dir).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	v := c.ViewByName("V")
	if v == nil {
		t.Fatal("view V not found")
	}
	byName := indexByName(v.Components)

	if got := byName["AttrComma"].Selectors; len(got) != 1 || got[0] != `[data-x="a,b"]` {
		t.Errorf(`AttrComma selectors = %v, want ["[data-x=\"a,b\"]"] (not split at the comma)`, got)
	}
	if got := byName["NthChild"].Selectors; len(got) != 1 || got[0] != "li:nth-child(2n, 3n)" {
		t.Errorf("NthChild selectors = %v, want one un-split selector", got)
	}
	// A genuine comma-separated list (authored as a YAML sequence) still yields
	// two alternatives.
	if got := byName["RealList"].Selectors; len(got) != 2 {
		t.Errorf("RealList selectors = %v, want 2 alternatives", got)
	}
}
