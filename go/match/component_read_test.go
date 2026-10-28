package match_test

import (
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// TestResolveComponentRead_AgreesWithMatch checks the contract a rule-compiling
// consumer relies on: reading a ComponentRead by selector (the first element its
// Selectors match within the declaring component's subtree, or every one when
// Multi, refined by Patterns and joined by Join) yields the value the matcher
// resolves over the matched tree.
func TestResolveComponentRead_AgreesWithMatch(t *testing.T) {
	ex := func(from, path string) sightmap.Extract { return sightmap.Extract{From: from, Path: path} }
	text := []sightmap.ComponentPropertyDef{{Name: "text", Extract: ex(sightmap.FromDOMText, "")}}
	corpus := &sightmap.Corpus{GlobalComponents: []sightmap.ComponentDef{
		{Name: "Card", Selectors: []string{".card"}, LocalSelectors: []string{".card"}, Properties: []sightmap.ComponentPropertyDef{
			{Name: "sku", Extract: ex(sightmap.FromDOMAttr, "data-sku")},
			{Name: "title", Extract: ex(sightmap.FromComponent, "Title.text")},
			{Name: "amount", Extract: sightmap.Extract{From: sightmap.FromComponent, Path: "Price.amount", Pattern: `^(\d+)`}},
			{Name: "sold_out", Extract: ex(sightmap.FromComponentExists, "SoldOut")},
			{Name: "tags", Extract: sightmap.Extract{From: sightmap.FromComponent, Path: "Tag[].text", Join: ","}},
		}},
		{Name: "Title", Selectors: []string{".card h2"}, LocalSelectors: []string{"h2"}, ParentChain: []string{"Card"}, Properties: text},
		{Name: "Body", Selectors: []string{".card .body"}, LocalSelectors: []string{".body"}, ParentChain: []string{"Card"}},
		{Name: "Price", Selectors: []string{".card .body .price"}, LocalSelectors: []string{".price"}, ParentChain: []string{"Card", "Body"}, Properties: []sightmap.ComponentPropertyDef{
			{Name: "amount", Extract: sightmap.Extract{From: sightmap.FromDOMText, Pattern: `\$(\S+)`}},
		}},
		{Name: "SoldOut", Selectors: []string{".card .sold-out"}, LocalSelectors: []string{".sold-out"}, ParentChain: []string{"Card"}},
		{Name: "Tag", Selectors: []string{".card .tag"}, LocalSelectors: []string{".tag"}, ParentChain: []string{"Card"}, Properties: text},
	}}

	el := func(tag, class string, attrs ...string) *sightmap.Element {
		e := &sightmap.Element{Tag: tag, Attrs: map[string]string{}}
		if class != "" {
			e.Classes = []string{class}
		}
		for i := 0; i+1 < len(attrs); i += 2 {
			e.Attrs[attrs[i]] = attrs[i+1]
		}
		return e
	}
	node := func(id, name string, e *sightmap.Element, kids ...*sightmap.ComponentNode) *sightmap.ComponentNode {
		return &sightmap.ComponentNode{Id: id, Name: name, Element: e, Children: kids}
	}
	card := node("card", "", el("div", "card", "data-sku", "A-1"),
		node("title", "Lisbon", el("h2", "")),
		node("body", "", el("div", "body"),
			node("price", "$398.00 total", el("span", "price"))),
		node("sold", "", el("span", "sold-out")),
		node("t1", "warm", el("span", "tag")),
		node("t2", "", el("span", "tag")),
		node("t3", "coastal", el("span", "tag")),
	)

	matched := match.NewMatcher(corpus).Match(card, "")
	want := map[string]string{}
	for _, p := range matched[card].Properties {
		want[p.Name] = p.Value
	}
	if len(want) != 5 {
		t.Fatalf("matcher resolved %v, want all 5 properties", want)
	}

	owner := corpus.GlobalComponents[0]
	got := map[string]string{}
	for _, p := range owner.Properties {
		r, err := corpus.ResolveComponentRead(owner, p.Extract)
		if err != nil {
			t.Fatalf("%s: %v", p.Name, err)
		}
		if v, ok := readBySelector(t, card, r); ok {
			got[p.Name] = v
		}
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s: read by selector %q, matcher %q", name, got[name], w)
		}
	}
}

// readBySelector evaluates a ComponentRead the way a selector-driven extractor
// would: it finds the elements r.Selectors match within root's subtree.
func readBySelector(t *testing.T, root *sightmap.ComponentNode, r sightmap.ComponentRead) (string, bool) {
	t.Helper()
	targets := []*sightmap.ComponentNode{root}
	if r.Selectors != nil {
		targets = nil
		defs := make([]sightmap.ComponentDef, 0, len(r.Selectors))
		for _, s := range r.Selectors {
			defs = append(defs, sightmap.ComponentDef{Name: "q", Selectors: []string{s}})
		}
		queries, errs := match.ParseQueries(defs)
		if len(errs) > 0 {
			t.Fatalf("parse %q: %v", r.Selectors, errs)
		}
		for _, kid := range root.Children {
			match.FindAllMatches(kid, queries, func(n *sightmap.ComponentNode, _ *match.MatchQuery) {
				if len(targets) == 0 || targets[len(targets)-1] != n {
					targets = append(targets, n)
				}
			})
		}
		if !r.Multi && len(targets) > 1 {
			targets = targets[:1]
		}
	}
	var vals []string
	for _, n := range targets {
		var v string
		switch r.Source.From {
		case sightmap.FromComponentExists:
			v = "true"
		case sightmap.FromDOMAttr:
			v = n.Element.Attrs[r.Source.Path]
		default:
			v = n.Name
		}
		ok := v != ""
		for _, p := range r.Patterns {
			if !ok {
				break
			}
			v, ok = sightmap.Extract{Pattern: p}.Refine(v)
		}
		if ok && v != "" {
			vals = append(vals, v)
		}
	}
	if len(vals) == 0 {
		return "", false
	}
	return strings.Join(vals, r.Join), true
}
