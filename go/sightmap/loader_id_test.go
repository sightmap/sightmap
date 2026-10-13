package sightmap_test

import (
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

func codes(errs []sightmap.ValidationError) []string {
	var out []string
	for _, e := range errs {
		out = append(out, e.Code)
	}
	sort.Strings(out)
	return out
}

func loadYAML(t *testing.T, files map[string]string) *sightmap.Corpus {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		writeFile(t, filepath.Join(dir, name), body)
	}
	c, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// SEP-0020: every named entity carries an id, and the loader exposes each one.
func TestEntityIDsLoad(t *testing.T) {
	c := loadYAML(t, map[string]string{"app.yaml": `
version: 1
environments:
  - id: env4kq2m7r
    name: prod
    origins:
      app: https://example.com
definitions:
  - id: def8kq2m7r
    name: ProductCard
    selector: .card
    properties:
      - id: prp2v8yd5e
        name: price
        extract: { from: dom.text }
views:
  - id: viw3n8kq2m
    name: Search
    route: /s
    properties:
      - id: prp7q4wz1d
        name: query
        extract: { from: url.query, path: q }
    components:
      - id: cmp6sjw3ra
        name: Results
        selector: ul.results
        children:
          - id: ref9tz2m4k
            $ref: ProductCard
    requests:
      - id: req5c9fw4j
        name: search
        route: /api/search
        response:
          fields:
            - id: fld1h7gq3n
              name: total
requests:
  - id: req2b3n6pk
    name: cart
    route: /api/cart
    properties:
      - id: prp4x8mn2c
        name: count
        extract: { from: rsp.body, path: count }
messages:
  - id: msg8r2k5vt
    name: Boom
    level: exception
    properties:
      - id: prp6d1zq9h
        name: fn
        extract: { from: stack, path: top.function }
signals:
  - id: sig3w7jx4p
    name: ResultsShown
    ref: Results
snapshots:
  - id: snp5m2hb8c
    name: base
`})
	if errs := sightmap.Validate(c); len(errs) != 0 {
		t.Fatalf("unexpected diagnostics: %v", errs)
	}
	v := c.Views[0]
	got := map[string]string{
		"environment":  c.Environments[0].ID,
		"definition":   c.Definitions[0].ID,
		"def property": c.Definitions[0].Properties[0].ID,
		"view":         v.ID,
		"url property": v.Properties[0].ID,
		"request":      v.Requests[0].ID,
		"field":        v.Requests[0].Response.Fields[0].ID,
		"global req":   c.Requests[0].ID,
		"req property": c.Requests[0].Properties[0].ID,
		"message":      c.Messages[0].ID,
		"msg property": c.Messages[0].Properties[0].ID,
		"signal":       c.Signals[0].ID,
	}
	want := map[string]string{
		"environment":  "env4kq2m7r",
		"definition":   "def8kq2m7r",
		"def property": "prp2v8yd5e",
		"view":         "viw3n8kq2m",
		"url property": "prp7q4wz1d",
		"request":      "req5c9fw4j",
		"field":        "fld1h7gq3n",
		"global req":   "req2b3n6pk",
		"req property": "prp4x8mn2c",
		"message":      "msg8r2k5vt",
		"msg property": "prp6d1zq9h",
		"signal":       "sig3w7jx4p",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ids = %v, want %v", got, want)
	}
}

// A $ref entry has its own id. The root of each expansion carries the $ref
// entry's id as RefID and the definition's id as ID, so two placements of one
// definition are told apart by RefID.
func TestRefIDOnExpansionRoot(t *testing.T) {
	c := loadYAML(t, map[string]string{"app.yaml": `
version: 1
definitions:
  - id: def8kq2m7r
    name: ProductCard
    selector: .card
    children:
      - id: cmpt9w4hx2
        name: Title
        selector: h3
views:
  - id: viw3n8kq2m
    name: Search
    route: /s/**
    components:
      - id: refr2v8yd5
        $ref: ProductCard
  - id: viw7f4j8nz
    name: Home
    route: /
    components:
      - id: refb3n6pk1
        $ref: ProductCard
`})
	if errs := sightmap.Validate(c); len(errs) != 0 {
		t.Fatalf("unexpected diagnostics: %v", errs)
	}
	type placement struct{ view, name, id, refID string }
	var got []placement
	for _, v := range c.Views {
		for _, d := range v.Components {
			got = append(got, placement{v.Name, d.Name, d.ID, d.RefID})
		}
	}
	want := []placement{
		{"Search", "ProductCard", "def8kq2m7r", "refr2v8yd5"},
		{"Search", "Title", "cmpt9w4hx2", ""},
		{"Home", "ProductCard", "def8kq2m7r", "refb3n6pk1"},
		{"Home", "Title", "cmpt9w4hx2", ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("placements = %v, want %v", got, want)
	}
}

// Uniqueness spans every entity kind and every file. Referencing a definition
// several times is not a duplicate.
func TestIDDuplicateAcrossKindsAndFiles(t *testing.T) {
	c := loadYAML(t, map[string]string{
		"a.yaml": `
version: 1
definitions:
  - id: def8kq2m7r
    name: ProductCard
    selector: .card
components:
  - id: checkout01
    name: CheckoutForm
    selector: form.checkout
`,
		"b.yaml": `
version: 1
views:
  - id: checkout01
    name: Checkout
    route: /checkout
    components:
      - id: cmp6sjw3ra
        name: Results
        selector: ul.results
        children:
          - id: refr2v8yd5
            $ref: ProductCard
      - id: cmp4f8nz1q
        name: Recent
        selector: ul.recent
        children:
          - id: refb3n6pk1
            $ref: ProductCard
`,
	})
	errs := sightmap.Validate(c)
	if got := codes(errs); !reflect.DeepEqual(got, []string{"id-duplicate"}) {
		t.Fatalf("codes = %v, want [id-duplicate]", got)
	}
	if e := errs[0]; e.File != "b.yaml" || e.Component != "Checkout" || !e.IsError() ||
		!strings.Contains(e.Message, `component "CheckoutForm" in a.yaml`) {
		t.Errorf("duplicate diagnostic = %+v", e)
	}
}

// An id must be a YAML string of the right shape: an unquoted number or a null
// is invalid rather than silently coerced or dropped.
func TestIDInvalid(t *testing.T) {
	c := loadYAML(t, map[string]string{"app.yaml": `
version: 1
components:
  - id: short1
    name: TooShort
    selector: .a
  - id: -leading-dash
    name: LeadingDash
    selector: .b
  - id: has a space
    name: Space
    selector: .c
  - id: 12345678
    name: Number
    selector: .n
  - id: ~
    name: Nothing
    selector: .z
  - id: '87654321'
    name: Quoted
    selector: .q
  - id: ok_ID-42
    name: Fine
    selector: .d
`})
	want := []string{"id-invalid", "id-invalid", "id-invalid", "id-invalid", "id-invalid"}
	if got := codes(sightmap.Validate(c)); !reflect.DeepEqual(got, want) {
		t.Errorf("codes = %v, want %v", got, want)
	}
}

// Once a sightmap declares any id, every entity without one is an error. A
// sightmap without ids validates as before.
func TestIDMissingOnceAdopted(t *testing.T) {
	c := loadYAML(t, map[string]string{
		"a.yaml": `
version: 1
definitions:
  - id: def8kq2m7r
    name: ProductCard
    selector: .card
`,
		"b.yaml": `
version: 1
views:
  - name: Search
    route: /s/**
    components:
      - $ref: ProductCard
      - id: cmp6sjw3ra
        name: Filters
        selector: .filters
        properties:
          - name: count
            extract: { from: dom.text }
`,
	})
	var missing []string
	for _, e := range sightmap.Validate(c) {
		if e.Code == "id-missing" {
			if !e.IsError() {
				t.Errorf("id-missing is not an error: %+v", e)
			}
			missing = append(missing, e.Component)
		}
	}
	sort.Strings(missing)
	if want := []string{"ProductCard", "Search", "count"}; !reflect.DeepEqual(missing, want) {
		t.Errorf("id-missing on %v, want %v (the view, the $ref entry and the property)", missing, want)
	}

	plain := loadYAML(t, map[string]string{"app.yaml": `
version: 1
components:
  - name: SiteHeader
    selector: header
`})
	if errs := sightmap.Validate(plain); len(errs) != 0 {
		t.Errorf("unexpected diagnostics in a sightmap without ids: %v", errs)
	}
}

// id is a known key on every entity, and the only key a $ref entry accepts
// besides $ref.
func TestIDKnownFields(t *testing.T) {
	c := loadYAML(t, map[string]string{"app.yaml": `
version: 1
definitions:
  - id: def8kq2m7r
    name: ProductCard
    selector: .card
components:
  - id: cmp5c9fw4j
    name: Hero
    selector: .hero
    children:
      - id: refx1y2z3w
        $ref: ProductCard
        name: Extra
`})
	var unknown []string
	for _, e := range sightmap.Validate(c) {
		if e.Code == "unknown-field" {
			unknown = append(unknown, e.Message)
		}
	}
	if len(unknown) != 1 || !strings.Contains(unknown[0], `"name"`) {
		t.Errorf("unknown-field findings = %v, want exactly the name on the $ref entry", unknown)
	}
}
