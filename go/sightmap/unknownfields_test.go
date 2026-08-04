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

// findingsWithCode is unknownWarnings' generalization: filter a validation run
// by any diagnostic code, not just unknown-field.
func findingsWithCode(t *testing.T, code, yaml string) []sightmap.ValidationError {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.yaml"), yaml)
	c, err := sightmap.DirLoader(dir).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var out []sightmap.ValidationError
	for _, f := range sightmap.Validate(c) {
		if f.Code == code {
			out = append(out, f)
		}
	}
	return out
}

// yaml.v3 decodes any scalar into a Go string field by taking the raw lexeme,
// so an unquoted number used to load cleanly here while ajv rejected it. The
// two conformance checkers have to agree.
func TestFieldTypeInvalid_RequestPropertyNonStringScalars(t *testing.T) {
	cases := []struct {
		name string
		yaml string
	}{
		{"integer field", "field: 200"},
		{"integer pattern", "pattern: 404"},
		{"bool field", "field: true"},
		{"null field", "field:"},
		{"float field", "field: 1.5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := findingsWithCode(t, "field-type-invalid", `
version: 1
requests:
  - name: R
    route: /api/x
    properties:
      - name: p
        `+tc.yaml+`
`)
			if len(got) != 1 {
				t.Fatalf("want 1 field-type-invalid, got %d: %v", len(got), got)
			}
			if !got[0].IsError() {
				t.Error("a type mismatch must be an error, not a warning")
			}
		})
	}
}

func TestFieldTypeInvalid_QuotedScalarsAreClean(t *testing.T) {
	got := findingsWithCode(t, "field-type-invalid", `
version: 1
requests:
  - name: R
    route: /api/x
    properties:
      - name: p
        field: "200"
      - name: q
        pattern: "404"
`)
	if len(got) != 0 {
		t.Fatalf("quoted scalars must be clean, got %v", got)
	}
}

// The requestFields / requestPropertyFields allowlists need a corpus that is
// entirely valid, or a typo in either set would go unnoticed.
func TestUnknownField_RequestPropertiesNoFalsePositives(t *testing.T) {
	w := unknownWarnings(t, `
version: 1
requests:
  - name: CheckoutPayment
    route: /api/checkout/pay
    method: POST
    description: Pay
    source: src/api/pay.ts
    headers: [x-request-id]
    memory:
      - 429s past 10/min
    tags: [checkout]
    request:
      fields:
        - name: amount
          type: number
    response:
      fields:
        - name: status
          type: string
    properties:
      - name: outcome
        field: rsp.body.status
        transform: slug
      - name: legacy
        pattern: 'declined'
`)
	if len(w) != 0 {
		t.Fatalf("valid request properties must not warn: %v", w)
	}
}

func TestUnknownField_RequestPropertyTypo(t *testing.T) {
	w := unknownWarnings(t, `
version: 1
requests:
  - name: R
    route: /api/x
    properties:
      - name: p
        feild: rsp.body.status
`)
	if len(w) != 1 {
		t.Fatalf("want 1 unknown-field for the typo, got %d: %v", len(w), w)
	}
}
