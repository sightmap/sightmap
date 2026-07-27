package sightmap_test

import (
	"path/filepath"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// hasErrorCode reports whether any finding is an error with the given code.
func hasErrorCode(fs []sightmap.ValidationError, code string) bool {
	for _, f := range fs {
		if f.Code == code && f.IsError() {
			return true
		}
	}
	return false
}

func TestRequired_MissingSelector_Errors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "components.yaml"), `
version: 1
components:
  - name: NoSelector
`)
	c, err := sightmap.DirLoader(dir).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !hasErrorCode(sightmap.Validate(c), "missing-selector") {
		t.Errorf("expected a missing-selector error, got %v", sightmap.Validate(c))
	}
}

func TestRequired_MissingComponentName_Errors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "components.yaml"), `
version: 1
components:
  - selector: ".orphan"
`)
	c, err := sightmap.DirLoader(dir).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !hasErrorCode(sightmap.Validate(c), "missing-name") {
		t.Errorf("expected a missing-name error, got %v", sightmap.Validate(c))
	}
}

func TestRequired_UnresolvedRef_Errors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "views.yaml"), `
version: 1
views:
  - name: Home
    route: /
    components:
      - $ref: DoesNotExist
`)
	c, err := sightmap.DirLoader(dir).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !hasErrorCode(sightmap.Validate(c), "ref-unresolved") {
		t.Errorf("expected a ref-unresolved error, got %v", sightmap.Validate(c))
	}
}

func TestRequired_ResolvedRef_NoError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "corpus.yaml"), `
version: 1
components:
  - name: Header
    selector: "#header"
views:
  - name: Home
    route: /
    components:
      - $ref: Header
`)
	c, err := sightmap.DirLoader(dir).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if hasErrorCode(sightmap.Validate(c), "ref-unresolved") {
		t.Errorf("a resolvable $ref must not error, got %v", sightmap.Validate(c))
	}
}

func TestRequired_ViewMissingName_Errors(t *testing.T) {
	c := corpusFrom(nil, []sightmap.View{{Name: "", Route: "/x"}})
	if !hasErrorCode(sightmap.Validate(c), "missing-name") {
		t.Errorf("expected a missing-name error for a nameless view, got %v", sightmap.Validate(c))
	}
}
