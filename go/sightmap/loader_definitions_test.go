package sightmap_test

import (
	"path/filepath"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// SEP-0019: file-root definitions are $ref targets that are never matched on
// their own. A view that references one gets it expanded in place; a view
// that does not never sees it.
func TestDefinitionsAreReferencedNotMatched(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "components.yaml"), `
version: 1
components:
  - name: SiteHeader
    selector: header
definitions:
  - name: ProductCard
    selector: '[data-component="ProductCard"]'
    children:
      - name: Title
        selector: h3
`)
	writeFile(t, filepath.Join(dir, "views.yaml"), `
version: 1
views:
  - name: Search
    route: /s/**
    components:
      - name: Results
        selector: ul.results
        children:
          - $ref: ProductCard
  - name: Home
    route: /
    components:
      - name: Hero
        selector: .hero
`)
	c, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if errs := sightmap.Validate(c); len(errs) != 0 {
		t.Fatalf("unexpected diagnostics: %v", errs)
	}
	names := func(url string) map[string][]string {
		out := map[string][]string{}
		for _, d := range c.ComponentsForURL(url) {
			out[d.Name] = append(out[d.Name], d.Selectors...)
		}
		return out
	}
	search := names("https://x.test/s/drills")
	if got := search["ProductCard"]; len(got) != 1 || got[0] != `ul.results [data-component="ProductCard"]` {
		t.Errorf("Search ProductCard selectors = %v, want the scoped expansion only", got)
	}
	if got := search["Title"]; len(got) != 1 || got[0] != `ul.results [data-component="ProductCard"] h3` {
		t.Errorf("Search Title selectors = %v", got)
	}
	if _, ok := search["SiteHeader"]; !ok {
		t.Error("globals still apply alongside definitions")
	}
	home := names("https://x.test/")
	if _, ok := home["ProductCard"]; ok {
		t.Error("an unreferenced definition must not be matched on Home")
	}
	if _, ok := home["Title"]; ok {
		t.Error("an unreferenced definition's children must not be matched on Home")
	}
	for _, g := range c.GlobalComponents {
		if g.Name == "ProductCard" || g.Name == "Title" {
			t.Errorf("definition %s leaked into GlobalComponents", g.Name)
		}
	}
	if len(c.Definitions) != 2 {
		t.Errorf("Definitions = %d flattened entries, want 2", len(c.Definitions))
	}
}

func TestDefinitionCollisions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.yaml"), `
version: 1
components:
  - name: Button
    selector: button.global
definitions:
  - name: Button
    selector: button.def
  - name: Card
    selector: .card-a
`)
	writeFile(t, filepath.Join(dir, "b.yaml"), `
version: 1
definitions:
  - name: Card
    selector: .card-b
views:
  - name: V
    route: /
    components:
      - $ref: Card
      - $ref: Button
`)
	c, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, e := range sightmap.Validate(c) {
		codes[e.Code] = true
	}
	for _, want := range []string{"definition-shadowed-by-global", "merge-collision-definition"} {
		if !codes[want] {
			t.Errorf("missing %s; got %v", want, codes)
		}
	}
	got := map[string]string{}
	for _, d := range c.Views[0].Components {
		got[d.Name] = d.Selectors[0]
	}
	if got["Card"] != ".card-a" {
		t.Errorf("Card = %q, want first-by-path .card-a", got["Card"])
	}
	if got["Button"] != "button.global" {
		t.Errorf("Button = %q, want the global (globals win a clash)", got["Button"])
	}
}

func TestDefinitionsKnownField(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "components.yaml"), `
version: 1
definitions:
  - name: Card
    selector: .card
    bogus: 1
`)
	c, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	var unknown []string
	for _, e := range sightmap.Validate(c) {
		if e.Code == "unknown-field" {
			unknown = append(unknown, e.Message)
		}
	}
	if len(unknown) != 1 {
		t.Errorf("want exactly the nested bogus key flagged (definitions itself is known), got %v", unknown)
	}
}
