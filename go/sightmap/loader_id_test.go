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
// placements apart even though they share its id.
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
	want := [][]string{{"r2v8yd5e", "kq2m7rta", "t9w4hx2c"}, {"f4j8nz1q", "kq2m7rta", "t9w4hx2c"}}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("Title IDPaths = %v, want %v", paths, want)
	}
}

// An ancestor without an id contributes "" to IDPath, and a component without
// an id has none.
func TestIDPathWithUnidentifiedAncestor(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.yaml"), `
version: 1
components:
  - name: Shell
    selector: main
    children:
      - id: c4mxq2nb
        name: Card
        selector: .card
`)
	c, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range c.GlobalComponents {
		switch d.Name {
		case "Shell":
			if d.IDPath != nil {
				t.Errorf("Shell IDPath = %v, want nil", d.IDPath)
			}
		case "Card":
			if !reflect.DeepEqual(d.IDPath, []string{"", "c4mxq2nb"}) {
				t.Errorf("Card IDPath = %v, want [\"\" c4mxq2nb]", d.IDPath)
			}
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

func TestComponentFormerly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.yaml"), `
version: 1
components:
  - id: m5c9fw4j
    name: Hero
    selector: .hero
    formerly: [p0z8ug3v, b2hdx7ke]
  - id: q7rak3d9
    name: SelfReference
    selector: .self
    formerly: [q7rak3d9]
  - id: w8e2mz5t
    name: Repeated
    selector: .rep
    formerly: [p0z8ug3v, p0z8ug3v]
  - name: NoID
    selector: .noid
    formerly: [p0z8ug3v]
`)
	c, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"component-formerly-invalid", "component-formerly-invalid", "component-formerly-invalid"}
	if got := codes(sightmap.Validate(c)); !reflect.DeepEqual(got, want) {
		t.Errorf("codes = %v, want %v", got, want)
	}
	for _, d := range c.GlobalComponents {
		if d.Name == "Hero" && !reflect.DeepEqual(d.Formerly, []string{"p0z8ug3v", "b2hdx7ke"}) {
			t.Errorf("Hero Formerly = %v", d.Formerly)
		}
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

// id and formerly are known component keys; a $ref entry still accepts no
// other key.
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
    formerly: [p0z8ug3v]
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
