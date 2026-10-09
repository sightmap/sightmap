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

// SEP-0020: ids load onto every declaration, and IDPath tells a definition's
// placements apart even though they share its id: inside a $ref expansion it
// is prefixed by the component holding the $ref.
func TestComponentIDsAndIDPaths(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "components.yaml"), `
version: 1
definitions:
  - id: kq2m7rta
    name: ProductCard
    selector: '[data-component="ProductCard"]'
    children:
      - id: t9w4hx2c
        name: Title
        selector: h3
`)
	writeFile(t, filepath.Join(dir, "views.yaml"), `
version: 1
views:
  - name: Search
    route: /s/**
    components:
      - id: r2v8yd5e
        name: Results
        selector: ul.results
        children:
          - $ref: ProductCard
      - id: f4j8nz1q
        name: Recent
        selector: ul.recent
        children:
          - $ref: ProductCard
`)
	c, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if errs := sightmap.Validate(c); len(errs) != 0 {
		t.Fatalf("unexpected diagnostics: %v", errs)
	}
	var paths [][]string
	for _, d := range c.Views[0].Components {
		if d.Name == "Title" {
			if d.ID != "t9w4hx2c" {
				t.Errorf("Title ID = %q, want t9w4hx2c", d.ID)
			}
			paths = append(paths, d.IDPath)
		}
	}
	want := [][]string{{"r2v8yd5e", "t9w4hx2c"}, {"f4j8nz1q", "t9w4hx2c"}}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("Title IDPaths = %v, want %v", paths, want)
	}
}

// Outside any $ref expansion IDPath is just the component's own id, so moving
// a component (or renaming or reselecting an ancestor) doesn't change it. A
// $ref held by a component without an id gives its expansion an incomplete
// path, marked by "".
func TestIDPathStableUnderMoves(t *testing.T) {
	load := func(yaml string) *sightmap.Corpus {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "app.yaml"), yaml)
		c, err := sightmap.Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	pathOf := func(c *sightmap.Corpus, name string) []string {
		for _, d := range c.GlobalComponents {
			if d.Name == name {
				return d.IDPath
			}
		}
		t.Fatalf("no component %q", name)
		return nil
	}
	before := load(`
version: 1
components:
  - id: f3n8kq2m
    name: CheckoutForm
    selector: form.checkout
    children:
      - id: kq2m7rta
        name: CardDetail
        selector: .card-detail
`)
	after := load(`
version: 1
components:
  - id: f3n8kq2m
    name: CheckoutForm
    selector: form.checkout
  - id: p6sjw3ra
    name: PaymentSheet
    selector: '[role="dialog"]'
    children:
      - id: kq2m7rta
        name: CardDetail
        selector: .card-detail
`)
	if b, a := pathOf(before, "CardDetail"), pathOf(after, "CardDetail"); !reflect.DeepEqual(b, a) || !reflect.DeepEqual(a, []string{"kq2m7rta"}) {
		t.Errorf("CardDetail IDPath before %v, after move %v; want [kq2m7rta] both", b, a)
	}

	unheld := load(`
version: 1
definitions:
  - id: kq2m7rta
    name: ProductCard
    selector: .card
components:
  - name: Shell
    selector: main
    children:
      - $ref: ProductCard
`)
	for _, d := range unheld.GlobalComponents {
		if d.Name == "ProductCard" && !reflect.DeepEqual(d.IDPath, []string{"", "kq2m7rta"}) {
			t.Errorf("ProductCard under an unidentified holder: IDPath %v, want [\"\" kq2m7rta]", d.IDPath)
		}
		if d.Name == "Shell" && d.IDPath != nil {
			t.Errorf("Shell IDPath = %v, want nil", d.IDPath)
		}
	}
}

// Uniqueness is checked across declarations in every file. Referencing a
// definition several times is not a duplicate.
func TestComponentIDDuplicateAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.yaml"), `
version: 1
definitions:
  - id: kq2m7rta
    name: ProductCard
    selector: .card
components:
  - id: checkout01
    name: CheckoutForm
    selector: form.checkout
`)
	writeFile(t, filepath.Join(dir, "b.yaml"), `
version: 1
views:
  - name: Search
    route: /s/**
    components:
      - id: r2v8yd5e
        name: Results
        selector: ul.results
        children:
          - $ref: ProductCard
      - id: f4j8nz1q
        name: Recent
        selector: ul.recent
        children:
          - $ref: ProductCard
      - id: checkout01
        name: LegacyCheckout
        selector: form.legacy
`)
	c, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	errs := sightmap.Validate(c)
	if got := codes(errs); !reflect.DeepEqual(got, []string{"component-id-duplicate"}) {
		t.Fatalf("codes = %v, want [component-id-duplicate]", got)
	}
	if e := errs[0]; e.File != "b.yaml" || e.Component != "LegacyCheckout" || !e.IsError() ||
		!strings.Contains(e.Message, `"CheckoutForm" in a.yaml`) {
		t.Errorf("duplicate diagnostic = %+v", e)
	}
}

func TestComponentIDInvalidShape(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.yaml"), `
version: 1
components:
  - id: ab
    name: TooShort
    selector: .a
  - id: -leading-dash
    name: LeadingDash
    selector: .b
  - id: has space
    name: Space
    selector: .c
  - id: ok_ID-42
    name: Fine
    selector: .d
`)
	c, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"component-id-invalid", "component-id-invalid", "component-id-invalid"}
	if got := codes(sightmap.Validate(c)); !reflect.DeepEqual(got, want) {
		t.Errorf("codes = %v, want %v", got, want)
	}
}

// Once a corpus declares any id, lint flags each declaration without one,
// once, however many times a $ref places it. A corpus without ids is silent.
func TestLintComponentIDMissing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.yaml"), `
version: 1
definitions:
  - name: ProductCard
    selector: '[data-component="ProductCard"]'
views:
  - name: Search
    route: /s/**
    components:
      - id: r2v8yd5e
        name: Results
        selector: ul.results
        children:
          - $ref: ProductCard
      - name: Filters
        selector: .filters
`)
	c, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	var missing []string
	for _, w := range sightmap.Lint(c) {
		if w.Rule == "component-id-missing" {
			missing = append(missing, w.Component)
		}
	}
	sort.Strings(missing)
	if !reflect.DeepEqual(missing, []string{"Filters", "ProductCard"}) {
		t.Errorf("component-id-missing on %v, want [Filters ProductCard]", missing)
	}

	plain := t.TempDir()
	writeFile(t, filepath.Join(plain, "app.yaml"), `
version: 1
components:
  - name: SiteHeader
    selector: header
`)
	c2, err := sightmap.Load(plain)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range sightmap.Lint(c2) {
		if w.Rule == "component-id-missing" {
			t.Errorf("unexpected %v in a corpus without ids", w)
		}
	}
}

// id is a known component key; a $ref entry still accepts no other key.
func TestComponentIDKnownFields(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.yaml"), `
version: 1
definitions:
  - id: kq2m7rta
    name: ProductCard
    selector: .card
components:
  - id: m5c9fw4j
    name: Hero
    selector: .hero
    children:
      - $ref: ProductCard
        id: x1y2z3w4
`)
	c, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	var unknown []string
	for _, e := range sightmap.Validate(c) {
		if strings.Contains(e.Code, "unknown") {
			unknown = append(unknown, e.Message)
		}
	}
	if len(unknown) != 1 || !strings.Contains(unknown[0], "id") {
		t.Errorf("unknown-field findings = %v, want exactly the id on the $ref entry", unknown)
	}
}

// A definition placed by $refs inside other globals is one declaration: an
// id-less one is reported once, not once per expansion.
func TestLintComponentIDMissingOncePerDeclaration(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.yaml"), `
version: 1
definitions:
  - name: Logo
    selector: .logo
components:
  - id: h7gq3n0p
    name: Header
    selector: header
    children:
      - $ref: Logo
  - id: f9tz2m4k
    name: Footer
    selector: footer
    children:
      - $ref: Logo
`)
	c, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	var missing []string
	for _, w := range sightmap.Lint(c) {
		if w.Rule == "component-id-missing" {
			missing = append(missing, w.Component+" "+w.Selector)
		}
	}
	if !reflect.DeepEqual(missing, []string{"Logo .logo"}) {
		t.Errorf("component-id-missing = %v, want only the Logo declaration", missing)
	}
}

// An id must be a YAML string, as the schema requires: an unquoted number or
// a null is invalid rather than silently coerced or dropped.
func TestComponentIDMustBeString(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.yaml"), `
version: 1
components:
  - id: 12345678
    name: Number
    selector: .n
  - id: ~
    name: Empty
    selector: .z
  - id: '87654321'
    name: Quoted
    selector: .q
`)
	c, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := codes(sightmap.Validate(c)); !reflect.DeepEqual(got, []string{"component-id-invalid", "component-id-invalid"}) {
		t.Errorf("codes = %v, want two component-id-invalid (the number and the null)", got)
	}
}
