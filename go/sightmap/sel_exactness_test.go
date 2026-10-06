package sightmap_test

import (
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// TestParse_RejectsLooseningForms covers compounds the parser used to accept by
// silently dropping a constraint: a second #id overwrote the first, a type
// selector after other tokens was accepted where CSS rejects it, and a second
// :is() replaced the first. Each would match more than CSS does, so each is now
// a parse error. Valid neighbours stay valid, including a repeated attribute
// name, whose tests now all apply (see TestMatchesNodeChain_CSSExactness).
func TestParse_RejectsLooseningForms(t *testing.T) {
	for _, s := range []string{
		`#a#b`,
		`div#a#b`,
		`[x]div`,
		`span*div`,
		`.a*`,
		`:is(.a):is(.b)`,
		`:where(.a):is(.b)`,
		`li:not(#a#b)`,
	} {
		t.Run(s, func(t *testing.T) { parseErr(t, s) })
	}
	for _, s := range []string{`div.a#b[x]`, `*.a`, `*`, `.a div`, `[a][b=c]`, `[class*="a"][class*="b"]`, `div:is(.a, .b)`, `input:not([type=hidden]):not(.x)`} {
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
		{"legacy value on SVG is case-sensitive", `a[target=_blank]`, []*sightmap.Element{el("svg", nil), el("a", map[string]string{"target": "_BLANK"})}, false},
		{"svg element itself is foreign", `[type=a]`, []*sightmap.Element{el("svg", map[string]string{"type": "A"})}, false},
		{"foreignObject content is HTML", `input[type=password]`, []*sightmap.Element{el("svg", nil), el("foreignobject", nil), el("input", pw)}, true},
		{"MathML is foreign", `[dir=rtl]`, []*sightmap.Element{el("math", nil), el("mi", map[string]string{"dir": "RTL"})}, false},
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
