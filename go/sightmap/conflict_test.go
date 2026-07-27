package sightmap_test

import (
	"path/filepath"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// findByCode returns the first finding with the given code, or a zero value.
func findByCode(fs []sightmap.ValidationError, code string) (sightmap.ValidationError, bool) {
	for _, f := range fs {
		if f.Code == code {
			return f, true
		}
	}
	return sightmap.ValidationError{}, false
}

func TestConflict_DuplicateViewName_Warns(t *testing.T) {
	c := corpusFrom(nil, []sightmap.View{
		{Name: "Dashboard", Route: "/a"},
		{Name: "Dashboard", Route: "/b"},
	})
	f, ok := findByCode(sightmap.Validate(c), "merge-collision-view")
	if !ok {
		t.Fatalf("expected a merge-collision-view finding, got %v", sightmap.Validate(c))
	}
	if f.IsError() {
		t.Errorf("duplicate view name should be a warning, not an error")
	}
}

func TestConflict_DuplicateRoute_Warns(t *testing.T) {
	c := corpusFrom(nil, []sightmap.View{
		{Name: "First", Route: "/x"},
		{Name: "Second", Route: "/x/"}, // normalizes to the same route
	})
	f, ok := findByCode(sightmap.Validate(c), "route-conflict")
	if !ok {
		t.Fatalf("expected a route-conflict finding, got %v", sightmap.Validate(c))
	}
	if f.IsError() {
		t.Errorf("route conflict should be a warning, not an error")
	}
}

func TestConflict_NoFalsePositive_DistinctViews(t *testing.T) {
	c := corpusFrom(nil, []sightmap.View{
		{Name: "A", Route: "/a"},
		{Name: "B", Route: "/b"},
	})
	for _, f := range sightmap.Validate(c) {
		if f.Code == "merge-collision-view" || f.Code == "route-conflict" {
			t.Errorf("unexpected conflict finding for distinct views: %v", f)
		}
	}
}

func TestConflict_DuplicateGlobalComponentName_Warns(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.yaml"), `
version: 1
components:
  - name: Header
    selector: "#header"
`)
	writeFile(t, filepath.Join(dir, "b.yaml"), `
version: 1
components:
  - name: Header
    selector: ".site-header"
`)
	c, err := sightmap.DirLoader(dir).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	f, ok := findByCode(sightmap.Validate(c), "merge-collision-component")
	if !ok {
		t.Fatalf("expected a merge-collision-component finding, got %v", sightmap.Validate(c))
	}
	if f.IsError() {
		t.Errorf("global component name collision should be a warning, not an error")
	}
}

// TestConflict_ChildReuse_NoGlobalCollision guards the false positive: a child
// component reused under two parents flattens to same-name entries with
// different compound selectors, which must NOT be reported as a global collision.
func TestConflict_ChildReuse_NoGlobalCollision(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "components.yaml"), `
version: 1
components:
  - name: CarouselA
    selector: ".carousel-a"
    children:
      - name: ScrollButton
        selector: "button.scroll"
  - name: CarouselB
    selector: ".carousel-b"
    children:
      - name: ScrollButton
        selector: "button.scroll"
`)
	c, err := sightmap.DirLoader(dir).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f, ok := findByCode(sightmap.Validate(c), "merge-collision-component"); ok {
		t.Errorf("child reuse must not be flagged as a global collision, got: %v", f)
	}
}
