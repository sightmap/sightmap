package sightmap_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

const readCorpus = `
version: 1
components:
  - name: TripBar
    selector: ".trip-bar, .trip-bar-alt"
    properties:
      - { name: start, extract: { from: dom.attr, path: data-start } }
      - { name: city, extract: { from: component, path: TripCity.text } }
      - { name: code, extract: { from: component, path: Meta.Code.value } }
      - { name: badge, extract: { from: component, path: TripCity.label, pattern: '^(\w+)' } }
      - { name: booked, extract: { from: component.exists, path: TripCity.Badge } }
      - { name: tags, extract: { from: component, path: 'Tag[].text', join: ', ' } }
      - { name: deep, extract: { from: component, path: Code.value } }
      - { name: missing, extract: { from: component, path: Nope.text } }
      - { name: ambiguous, extract: { from: component, path: Dup.text } }
      - { name: noprop, extract: { from: component, path: TripCity.nope } }
      - { name: invalid, extract: { from: component, path: 'Tag[].text' } }
      - { name: loop, extract: { from: component, path: Loop.a } }
    children:
      - name: TripCity
        selector: .trip-bar-city
        properties:
          - { name: text, extract: { from: dom.text } }
          - { name: label, extract: { from: component, path: Badge.text, pattern: '\S+ (\S+)' } }
        children:
          - name: Badge
            selector: .badge
            properties:
              - { name: text, extract: { from: dom.text } }
      - name: Meta
        selector: .meta
        children:
          - name: Code
            selector: "span.code, b.code"
            properties:
              - { name: value, extract: { from: dom.attr, path: data-code } }
          - name: Dup
            selector: .dup-b
            properties:
              - { name: text, extract: { from: dom.text } }
      - name: Tag
        selector: .tag
        properties:
          - { name: text, extract: { from: dom.text } }
      - name: Dup
        selector: .dup-a
        properties:
          - { name: text, extract: { from: dom.text } }
      - name: Loop
        selector: .loop
        properties:
          - { name: a, extract: { from: component, path: Inner.b } }
        children:
          - name: Inner
            selector: .inner
            properties:
              - { name: b, extract: { from: component, path: Leaf.c } }
            children:
              - name: Leaf
                selector: .leaf
                properties:
                  - { name: c, extract: { from: dom.text } }
`

func loadReadCorpus(t *testing.T) (*sightmap.Corpus, sightmap.ComponentDef) {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "components.yaml"), readCorpus)
	c, err := sightmap.DirLoader(dir).Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for _, d := range c.GlobalComponents {
		if d.Name == "TripBar" {
			return c, d
		}
	}
	t.Fatal("TripBar not loaded")
	return nil, sightmap.ComponentDef{}
}

func propExtract(t *testing.T, d sightmap.ComponentDef, name string) sightmap.Extract {
	t.Helper()
	for _, p := range d.Properties {
		if p.Name == name {
			return p.Extract
		}
	}
	t.Fatalf("%s declares no property %q", d.Name, name)
	return sightmap.Extract{}
}

func TestResolveComponentRead(t *testing.T) {
	c, bar := loadReadCorpus(t)
	cases := []struct {
		prop      string
		target    string
		selectors []string
		source    sightmap.Extract
		patterns  []string
		multi     bool
		join      string
	}{
		{prop: "start", target: "TripBar", source: sightmap.Extract{From: sightmap.FromDOMAttr, Path: "data-start"}},
		{prop: "city", target: "TripCity", selectors: []string{".trip-bar-city"}, source: sightmap.Extract{From: sightmap.FromDOMText}},
		// A nested path crosses every definition between, alternatives multiplied.
		{prop: "code", target: "Code", selectors: []string{".meta span.code", ".meta b.code"}, source: sightmap.Extract{From: sightmap.FromDOMAttr, Path: "data-code"}},
		// A chained read follows to the leaf; patterns apply innermost first.
		{prop: "badge", target: "Badge", selectors: []string{".trip-bar-city .badge"}, source: sightmap.Extract{From: sightmap.FromDOMText}, patterns: []string{`\S+ (\S+)`, `^(\w+)`}},
		{prop: "booked", target: "Badge", selectors: []string{".trip-bar-city .badge"}, source: sightmap.Extract{From: sightmap.FromComponentExists}},
		{prop: "tags", target: "Tag", selectors: []string{".tag"}, source: sightmap.Extract{From: sightmap.FromDOMText}, multi: true, join: ", "},
		// A segment names a descendant at any depth.
		{prop: "deep", target: "Code", selectors: []string{".meta span.code", ".meta b.code"}, source: sightmap.Extract{From: sightmap.FromDOMAttr, Path: "data-code"}},
		// A two-level chain.
		{prop: "loop", target: "Leaf", selectors: []string{".loop .inner .leaf"}, source: sightmap.Extract{From: sightmap.FromDOMText}},
	}
	for _, tc := range cases {
		t.Run(tc.prop, func(t *testing.T) {
			r, err := c.ResolveComponentRead(bar, propExtract(t, bar, tc.prop))
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			got := struct {
				Target    string
				Selectors []string
				Source    sightmap.Extract
				Patterns  []string
				Multi     bool
				Join      string
			}{r.Target.Name, r.Selectors, r.Source, r.Patterns, r.Multi, r.Join}
			want := struct {
				Target    string
				Selectors []string
				Source    sightmap.Extract
				Patterns  []string
				Multi     bool
				Join      string
			}{tc.target, tc.selectors, tc.source, tc.patterns, tc.multi, tc.join}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("got  %+v\nwant %+v", got, want)
			}
		})
	}
}

func TestResolveComponentRead_Errors(t *testing.T) {
	c, bar := loadReadCorpus(t)
	for prop, code := range map[string]string{
		"missing":   "component-path-unresolved",
		"ambiguous": "component-path-ambiguous",
		"noprop":    "component-property-unresolved",
		"invalid":   "component-property-extract-invalid",
	} {
		t.Run(prop, func(t *testing.T) {
			_, err := c.ResolveComponentRead(bar, propExtract(t, bar, prop))
			var ve sightmap.ValidationError
			if !errors.As(err, &ve) || ve.Code != code {
				t.Fatalf("got %v, want code %s", err, code)
			}
			if ve.Component != "TripBar" {
				t.Errorf("error names component %q, want TripBar", ve.Component)
			}
		})
	}

	_, err := c.ResolveComponentRead(sightmap.ComponentDef{Name: "Stranger", Selectors: []string{".x"}}, sightmap.Extract{From: sightmap.FromDOMText})
	var ve sightmap.ValidationError
	if !errors.As(err, &ve) || ve.Code != "component-read-owner-unknown" {
		t.Errorf("owner outside the corpus: got %v", err)
	}
}

// A corpus built without the loader (an older exporter's wire) carries no
// LocalSelectors; relative selectors are recovered from the flattened ones.
func TestResolveComponentRead_WithoutLocalSelectors(t *testing.T) {
	c, _ := loadReadCorpus(t)
	for i := range c.GlobalComponents {
		c.GlobalComponents[i].LocalSelectors = nil
	}
	var bar sightmap.ComponentDef
	for _, d := range c.GlobalComponents {
		if d.Name == "TripBar" {
			bar = d
		}
	}
	r, err := c.ResolveComponentRead(bar, propExtract(t, bar, "code"))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if want := []string{".meta span.code", ".meta b.code"}; !reflect.DeepEqual(r.Selectors, want) {
		t.Errorf("selectors %q, want %q", r.Selectors, want)
	}
}

// A multi-valued read with a plain step can't be expressed as one selector list:
// the plain step takes the first match per branch. All-Name[] paths resolve.
func TestResolveComponentRead_MixedMultiPath(t *testing.T) {
	ex := func(from, path string) sightmap.Extract { return sightmap.Extract{From: from, Path: path} }
	text := []sightmap.ComponentPropertyDef{{Name: "text", Extract: ex(sightmap.FromDOMText, "")}}
	joined := func(path string) sightmap.Extract {
		return sightmap.Extract{From: sightmap.FromComponent, Path: path, Join: "|"}
	}
	c := &sightmap.Corpus{GlobalComponents: []sightmap.ComponentDef{
		{Name: "Card", Selectors: []string{".card"}, LocalSelectors: []string{".card"}, Properties: []sightmap.ComponentPropertyDef{
			{Name: "row_first", Extract: joined("Row[].Item.text")},
			{Name: "first_row", Extract: joined("Row.Item[].text")},
			{Name: "chained", Extract: joined("Row[].first")},
			{Name: "all", Extract: joined("Row[].Item[].text")},
			{Name: "exists", Extract: joined("Row[].has_item")},
		}},
		{Name: "Row", Selectors: []string{".card .row"}, LocalSelectors: []string{".row"}, ParentChain: []string{"Card"}, Properties: []sightmap.ComponentPropertyDef{
			{Name: "first", Extract: ex(sightmap.FromComponent, "Item.text")},
			{Name: "has_item", Extract: ex(sightmap.FromComponentExists, "Item")},
		}},
		{Name: "Item", Selectors: []string{".card .row .item"}, LocalSelectors: []string{".item"}, ParentChain: []string{"Card", "Row"}, Properties: text},
	}}
	card := c.GlobalComponents[0]
	for _, p := range []string{"row_first", "first_row", "chained", "exists"} {
		_, err := c.ResolveComponentRead(card, propExtract(t, card, p))
		var ve sightmap.ValidationError
		if !errors.As(err, &ve) || ve.Code != "component-read-unsupported" {
			t.Errorf("%s: got %v, want component-read-unsupported", p, err)
		}
	}
	r, err := c.ResolveComponentRead(card, propExtract(t, card, "all"))
	if err != nil {
		t.Fatalf("all-Name[] path: %v", err)
	}
	if want := []string{".row .item"}; !r.Multi || !reflect.DeepEqual(r.Selectors, want) {
		t.Errorf("all-Name[] path: multi=%v selectors=%q, want true %q", r.Multi, r.Selectors, want)
	}
}

// The older-wire fallback recovers local selectors from the loader's
// flattening order, so a parent alternative that prefixes another ("li" and
// "li span.x") doesn't corrupt them; flattened selectors that don't follow
// that order are reported rather than guessed at.
func TestResolveComponentRead_FallbackPrefixAlternatives(t *testing.T) {
	read := sightmap.Extract{From: sightmap.FromComponent, Path: "Item.text"}
	build := func(itemSels []string) (*sightmap.Corpus, sightmap.ComponentDef) {
		c := &sightmap.Corpus{GlobalComponents: []sightmap.ComponentDef{
			{Name: "List", Selectors: []string{".list"}, Properties: []sightmap.ComponentPropertyDef{{Name: "first", Extract: read}}},
			{Name: "Row", Selectors: []string{".list li", ".list li span.x"}, ParentChain: []string{"List"}},
			{Name: "Item", Selectors: itemSels, ParentChain: []string{"List", "Row"},
				Properties: []sightmap.ComponentPropertyDef{{Name: "text", Extract: sightmap.Extract{From: sightmap.FromDOMText}}}},
		}}
		c.GlobalComponents[0].Properties[0].Extract.Path = "Row.Item.text"
		return c, c.GlobalComponents[0]
	}

	c, list := build([]string{".list li em", ".list li span.x em"})
	r, err := c.ResolveComponentRead(list, list.Properties[0].Extract)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if want := []string{"li em", "li span.x em"}; !reflect.DeepEqual(r.Selectors, want) {
		t.Errorf("selectors %q, want %q", r.Selectors, want)
	}

	c, list = build([]string{".list li em", ".list li strong"}) // not parent x local order
	_, err = c.ResolveComponentRead(list, list.Properties[0].Extract)
	var ve sightmap.ValidationError
	if !errors.As(err, &ve) || ve.Code != "component-read-unsupported" {
		t.Errorf("inconsistent flattening: got %v, want component-read-unsupported", err)
	}
}

func TestLoader_LocalSelectors(t *testing.T) {
	c, _ := loadReadCorpus(t)
	for _, d := range c.GlobalComponents {
		if d.Name == "Code" {
			if want := []string{"span.code", "b.code"}; !reflect.DeepEqual(d.LocalSelectors, want) {
				t.Errorf("LocalSelectors %q, want %q", d.LocalSelectors, want)
			}
			if len(d.Selectors) != 4 {
				t.Errorf("flattened Selectors %q, want 4 (2 TripBar x 2 Code)", d.Selectors)
			}
			return
		}
	}
	t.Fatal("Code not loaded")
}

func TestValidateComponentExtract(t *testing.T) {
	if err := sightmap.ValidateComponentExtract(sightmap.Extract{From: sightmap.FromDOMText}); err != nil {
		t.Errorf("valid extract: %v", err)
	}
	err := sightmap.ValidateComponentExtract(sightmap.Extract{From: sightmap.FromDOMAttr})
	var ve sightmap.ValidationError
	if !errors.As(err, &ve) || ve.Code != "component-property-extract-invalid" {
		t.Errorf("dom.attr without path: got %v", err)
	}
}
