package sightmap_test

import (
	"path/filepath"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

func unknownWarnings(t *testing.T, yaml string) []sightmap.ValidationError {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.yaml"), yaml)
	c, err := sightmap.DirLoader(dir).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var out []sightmap.ValidationError
	for _, f := range sightmap.Validate(c) {
		if f.Code == "unknown-field" {
			out = append(out, f)
		}
	}
	return out
}

func TestUnknownField_Typo(t *testing.T) {
	w := unknownWarnings(t, `
version: 1
memroy:
  - oops
views:
  - name: Home
    route: /
`)
	if len(w) != 1 {
		t.Fatalf("expected 1 unknown-field warning for 'memroy', got %v", w)
	}
	if w[0].IsError() {
		t.Errorf("unknown field should be a warning, not an error")
	}
}

// TestUnknownField_FutureFieldSurvives: a half-baked field warns but does not
// fail the corpus (so authors can stash experimental fields mid-development).
func TestUnknownField_FutureFieldSurvives(t *testing.T) {
	w := unknownWarnings(t, `
version: 1
macros:
  - name: signIn
views:
  - name: Home
    route: /
`)
	if len(w) != 1 || w[0].IsError() {
		t.Fatalf("expected exactly one non-fatal warning for 'macros', got %v", w)
	}
}

func TestUnknownField_NestedTypo(t *testing.T) {
	w := unknownWarnings(t, `
version: 1
components:
  - name: Btn
    selector: .btn
    selecto: .oops
`)
	if len(w) != 1 {
		t.Fatalf("expected 1 nested unknown-field warning for 'selecto', got %v", w)
	}
}

// TestUnknownField_NoFalsePositives: every blessed + reserved field must be
// recognized — including stability/access/snapshots/url/properties and $ref.
func TestUnknownField_NoFalsePositives(t *testing.T) {
	w := unknownWarnings(t, `
version: 1
url: https://example.com
memory:
  - file note
snapshots:
  - name: base
    notes: n
    url: https://example.com/
components:
  - name: Header
    selector: '#header'
    stability: uncertain
requests:
  - name: List
    route: /api/list
    method: GET
views:
  - name: Home
    route: /
    url: https://example.com/
    stability: stub
    access:
      status: blocked
      reason: admin only
    memory:
      - view note
    components:
      - $ref: Header
      - name: Card
        selector: .card
        properties:
          - name: price
            extract: text
            transform: first_dollar
        children:
          - name: Inner
            selector: .inner
`)
	if len(w) != 0 {
		t.Errorf("expected no unknown-field warnings for an all-blessed corpus, got %v", w)
	}
}
