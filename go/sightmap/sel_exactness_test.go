package sightmap_test

import (
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// TestParse_RejectsLooseningForms covers compounds the parser used to accept by
// silently dropping a constraint: a type selector after other tokens was
// accepted where CSS rejects it. Valid neighbours stay valid, including a
// repeated attribute name, a repeated #id and a second :is(), whose tests all
// apply (see TestMatchesNodeChain_CSSExactness).
func TestParse_RejectsLooseningForms(t *testing.T) {
	for _, s := range []string{
		`[x]div`,
		`span*div`,
		`.a*`,
	} {
		t.Run(s, func(t *testing.T) { parseErr(t, s) })
	}
	for _, s := range []string{`div.a#b[x]`, `*.a`, `*`, `.a div`, `[a][b=c]`, `[class*="a"][class*="b"]`, `#a#a`, `div#a#b`, `:is(.a):is(.b)`, `div:is(.a, .b)`, `input:not([type=hidden]):not(.x)`} {
		t.Run("valid "+s, func(t *testing.T) { parseOK(t, s) })
	}
}

// chainOf builds a root-first ancestor chain of nodes from elements.
func chainOf(els ...*sightmap.Element) []*sightmap.ComponentNode {
	out := make([]*sightmap.ComponentNode, len(els))
	for i, el := range els {
		out[i] = &sightmap.ComponentNode{Element: el}
	}
	return out
}

// TestMatchesNodeChain_CSSExactness pins matching to what a browser does for
// the same selector and element, including the HTML attributes whose values
// compare case-insensitively (on HTML elements only), ASCII-only type folding,
// and ~= values that can never match.
func TestMatchesNodeChain_CSSExactness(t *testing.T) {
	el := func(tag string, attrs map[string]string) *sightmap.Element {
		return &sightmap.Element{Tag: tag, Attrs: attrs}
	}
	pw := map[string]string{"type": "PASSWORD"}
	cases := []struct {
		name     string
		selector string
		chain    []*sightmap.Element // root-first; the last is the subject
		want     bool
	}{
		{"a repeated :is() ANDs", `:is(.a):is(.b)`, []*sightmap.Element{{Tag: "div", Classes: []string{"a", "b"}}}, true},
		{"a repeated :is() ANDs, one missing", `:is(.a):is(.b)`, []*sightmap.Element{{Tag: "div", Classes: []string{"a"}}}, false},
		{"the same #id twice matches", `#a#a`, []*sightmap.Element{{Tag: "div", Id: "a"}}, true},
		{"two different #ids match nothing", `#a#b`, []*sightmap.Element{{Tag: "div", Id: "a"}}, false},
		{"repeated attribute: both tests apply", `[class*=Card][class*=link]`, []*sightmap.Element{el("a", map[string]string{"class": "Card_x link_y"})}, true},
		{"repeated attribute: second test fails", `[class*=Card][class*=link]`, []*sightmap.Element{el("a", map[string]string{"class": "Card_x other"})}, false},
		{"repeated attribute: first test fails", `[a^=x][a=y]`, []*sightmap.Element{el("div", map[string]string{"a": "y"})}, false},
		{"repeated attribute: presence then value", `[a][a=x]`, []*sightmap.Element{el("div", map[string]string{"a": "y"})}, false},
		{"~= empty value never matches", `[a~=""]`, []*sightmap.Element{el("div", map[string]string{"a": "p  q"})}, false},
		{"~= value with whitespace never matches", `[a~="p q"]`, []*sightmap.Element{el("div", map[string]string{"a": "p q"})}, false},
		{"~= word", `[a~=q]`, []*sightmap.Element{el("div", map[string]string{"a": "p q"})}, true},
		{"type folds ASCII only", `a-s`, []*sightmap.Element{el("a-ſ", nil)}, false},
		{"type folds ASCII case", `button`, []*sightmap.Element{el("BUTTON", nil)}, true},
		{"legacy type value on HTML", `input[type=password]`, []*sightmap.Element{el("input", pw)}, true},
		{"legacy value under every operator", `[type^=pass]`, []*sightmap.Element{el("input", map[string]string{"type": "PASSword"})}, true},
		{"legacy lang |=", `[lang|=en]`, []*sightmap.Element{el("p", map[string]string{"lang": "EN-us"})}, true},
		{"non-legacy value is case-sensitive", `[data-x=a]`, []*sightmap.Element{el("div", map[string]string{"data-x": "A"})}, false},
		// In an HTML document the legacy list applies to every element, SVG and
		// MathML included (verified against Chrome).
		{"legacy value on an SVG element folds too", `a[target=_blank]`, []*sightmap.Element{el("svg", nil), el("a", map[string]string{"target": "_BLANK"})}, true},
		{"legacy value on a MathML element folds too", `[dir=rtl]`, []*sightmap.Element{el("math", nil), el("mi", map[string]string{"dir": "RTL"})}, true},
		{"legacy value on an SVG element with no ancestors", `[type=a]`, []*sightmap.Element{el("path", map[string]string{"type": "A"})}, true},
		{"a non-legacy attribute on an SVG element stays exact", `[data-x=a]`, []*sightmap.Element{el("svg", nil), el("path", map[string]string{"data-x": "A"})}, false},
		{"foreignObject content is HTML", `input[type=password]`, []*sightmap.Element{el("svg", nil), el("foreignobject", nil), el("input", pw)}, true},
		// A camelCase SVG attribute name is never one the legacy list covers.
		{"a camelCase SVG attribute compares exactly", `[attributetype=xml]`, []*sightmap.Element{el("svg", nil), el("animate", map[string]string{"attributeType": "XML"})}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ps, err := sightmap.ParseSightmapSelector(c.selector)
			if err != nil {
				t.Fatal(err)
			}
			subject := ps.Parts[len(ps.Parts)-1]
			if got := sightmap.MatchesNodeChain(chainOf(c.chain...), subject); got != c.want {
				t.Errorf("%s on %s: got %v, want %v", c.selector, c.chain[len(c.chain)-1].Tag, got, c.want)
			}
		})
	}
}
