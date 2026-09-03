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

// validateCodes loads inline YAML and returns the multiset of diagnostic codes
// from a full validation run. Used where a test needs to reason about several
// codes at once (e.g. the tag vs. pattern case matrix for component property
// names, where field-type-invalid and component-property-invalid-name are both
// in play).
func validateCodes(t *testing.T, yaml string) map[string]int {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.yaml"), yaml)
	c, err := sightmap.DirLoader(dir).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	m := map[string]int{}
	for _, f := range sightmap.Validate(c) {
		m[f.Code]++
	}
	return m
}

func codesEqual(got, want map[string]int) bool {
	if len(got) != len(want) {
		return false
	}
	for k, n := range want {
		if got[k] != n {
			return false
		}
	}
	return true
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
        source: rsp.body
        field: status
      - name: legacy
        source: rsp.body
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

// Message level/message are a vocabulary word and a regex, so an unquoted
// number is always a mistake — and `message: 404` is the natural way to write a
// pattern for an HTTP-status console error. ajv rejects all of these.
func TestFieldTypeInvalid_MessageNonStringScalars(t *testing.T) {
	for _, body := range []string{"message: 404", "level: 500", "message: true", "message:"} {
		got := findingsWithCode(t, "field-type-invalid", "version: 1\nmessages:\n  - name: M\n    "+body+"\n")
		if len(got) != 1 {
			t.Errorf("%q: want 1 field-type-invalid, got %d: %v", body, len(got), got)
		}
	}
}

func TestFieldTypeInvalid_MessageQuotedIsClean(t *testing.T) {
	got := findingsWithCode(t, "field-type-invalid", `
version: 1
messages:
  - name: M
    message: "404"
    level: EXCEPTION
`)
	if len(got) != 0 {
		t.Fatalf("quoted scalars must be clean, got %v", got)
	}
}

// ajv rejects a message with no name or an empty one; Go must agree.
func TestValidate_MessageEmptyNameMatchesAjv(t *testing.T) {
	for _, body := range []string{"level: ERROR", `name: ""`} {
		got := findingsWithCode(t, "missing-name", "version: 1\nmessages:\n  - "+body+"\n")
		if len(got) != 1 {
			t.Errorf("%q: want 1 missing-name, got %d: %v", body, len(got), got)
		}
	}
}

func TestUnknownField_MessagesNoFalsePositives(t *testing.T) {
	w := unknownWarnings(t, `
version: 1
messages:
  - name: CartVersionMismatch
    level: ERROR
    message: cart version mismatch
    description: Cart mutated elsewhere
    source: src/cart/sync.ts
`)
	if len(w) != 0 {
		t.Fatalf("valid messages must not warn: %v", w)
	}
}

func TestUnknownField_MessageTypo(t *testing.T) {
	w := unknownWarnings(t, `
version: 1
messages:
  - name: M
    lvl: ERROR
`)
	if len(w) != 1 {
		t.Fatalf("want 1 unknown-field for the typo, got %d: %v", len(w), w)
	}
}

// Component properties are the third property-bearing entity (after request
// and message properties). Their `name` and `extract` fields are schema-typed
// strings, so an unquoted bool/int/null loads cleanly into the Go string field
// while ajv rejects it as "must be string" — the two conformance checkers have
// to agree. Mirrors TestFieldTypeInvalid_RequestPropertyNonStringScalars and
// TestFieldTypeInvalid_MessageNonStringScalars.
func TestFieldTypeInvalid_ComponentPropertyNonStringScalars(t *testing.T) {
	// Each entry is a full two-line property body. The name cases carry a valid
	// `extract: text` so the only field-type-invalid comes from the bad name;
	// the extract cases carry a valid `name: label` so it comes from the bad
	// extract. (extract's content is already enforced by checkExtractMode, so
	// a content-invalid extract also emits component-property-extract-invalid,
	// which this test deliberately filters out.)
	cases := []struct {
		name string
		prop string
	}{
		{"integer name", "name: 5\n        extract: text"},
		{"bool name", "name: true\n        extract: text"},
		{"null name", "name:\n        extract: text"},
		{"float name", "name: 1.5\n        extract: text"},
		{"integer extract", "name: label\n        extract: 5"},
		{"null extract", "name: label\n        extract:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := findingsWithCode(t, "field-type-invalid", `
version: 1
components:
  - name: Header
    selector: '#header'
    properties:
      - `+tc.prop+`
`)
			if len(got) != 1 {
				t.Fatalf("want exactly 1 field-type-invalid, got %d: %v", len(got), got)
			}
			if !got[0].IsError() {
				t.Error("a type mismatch must be an error, not a warning")
			}
		})
	}
}

// A quoted scalar is !!str, so checkStringScalars leaves it alone even when the
// content would be invalid (e.g. "5"); that content is the pattern check's job
// (see TestComponentProperty_NameTagAndPatternMatchAjv).
func TestFieldTypeInvalid_ComponentPropertyQuotedIsClean(t *testing.T) {
	got := findingsWithCode(t, "field-type-invalid", `
version: 1
components:
  - name: Header
    selector: '#header'
    properties:
      - name: "5"
        extract: "text"
`)
	if len(got) != 0 {
		t.Fatalf("quoted scalars must be clean of field-type-invalid, got %v", got)
	}
}

// TestComponentProperty_NameTagAndPatternMatchAjv is the case matrix from the
// bug report. yaml.v3 decodes any scalar into a Go string field by the raw
// lexeme, so the YAML tag and the decoded content vary independently. Each gap
// uniquely covers one axis; both fixes are required, neither subsumes the
// other.
//
//	name: 5    (!!int)  → lexeme "5" fails pattern → tag check AND pattern check
//	name: true (!!bool) → lexeme "true" matches     → tag check ONLY  (Gap 1 load-bearing)
//	name: "5"  (!!str)  → lexeme "5" fails pattern  → pattern check ONLY (Gap 2 load-bearing)
//	name:      (!!null) → lexeme "" fails pattern   → tag check AND pattern check
//
// A property named "true" is reachable downstream via PATH.prop resolution
// (Header.true), so Case C is wired, not just exit-code divergence — which is
// why Gap 1 is load-bearing rather than a cosmetic duplicate of the pattern
// check.
func TestComponentProperty_NameTagAndPatternMatchAjv(t *testing.T) {
	cases := []struct {
		name      string
		yaml      string
		wantCodes map[string]int
	}{
		{
			name:      "int name: tag and pattern both fire (overlap)",
			yaml:      "name: 5\n        extract: text",
			wantCodes: map[string]int{"field-type-invalid": 1, "component-property-invalid-name": 1},
		},
		{
			name:      "bool name: tag-only (Gap 1 uniquely load-bearing)",
			yaml:      "name: true\n        extract: text",
			wantCodes: map[string]int{"field-type-invalid": 1},
		},
		{
			name:      "quoted-str name: pattern-only (Gap 2 uniquely load-bearing)",
			yaml:      "name: \"5\"\n        extract: text",
			wantCodes: map[string]int{"component-property-invalid-name": 1},
		},
		{
			name:      "null name: tag and pattern both fire (overlap)",
			yaml:      "name:\n        extract: text",
			wantCodes: map[string]int{"field-type-invalid": 1, "component-property-invalid-name": 1},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateCodes(t, `
version: 1
components:
  - name: Header
    selector: '#header'
    properties:
      - `+tc.yaml+`
`)
			if !codesEqual(got, tc.wantCodes) {
				t.Fatalf("got %v, want %v", got, tc.wantCodes)
			}
		})
	}
}
