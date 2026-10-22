package match_test

import (
	"reflect"
	"testing"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// A node claimed by a definition and by a strictly more specific one goes to the
// more specific, whichever is declared first, and is not a conflict (SEP-0023).
func TestMatch_MostSpecificDefinitionWins(t *testing.T) {
	root := node("root", "div", nil,
		node("plain", "span", nil),
		node("label", "span", []string{"label"}),
		nodeAttr("price", "span", map[string]string{"data-id": "row-1-price"}),
		nodeAttr("total", "span", map[string]string{"data-id": "row-1-total-price"}),
	)
	defs := []sightmap.ComponentDef{
		{Name: "Text", Selectors: []string{"span"}},
		{Name: "Label", Selectors: []string{"span.label"}},
		{Name: "Price", Selectors: []string{`span[data-id^="row-"][data-id$="-price"]`}},
		{Name: "TotalPrice", Selectors: []string{`span[data-id^="row-"][data-id$="-total-price"]`}},
	}
	m := match.NewMatcher(&sightmap.Corpus{GlobalComponents: defs})
	got := map[string]string{}
	for n, cm := range m.Match(root, "") {
		got[n.Id] = cm.Name
	}
	want := map[string]string{"plain": "Text", "label": "Label", "price": "Price", "total": "TotalPrice"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Match = %v, want %v", got, want)
	}
	if c := m.Conflicts(root, ""); len(c) != 0 {
		t.Errorf("ordered definitions are not conflicts, got %+v", c)
	}
	for id, name := range want {
		var leaf *sightmap.ComponentNode
		for _, ch := range root.Children {
			if ch.Id == id {
				leaf = ch
			}
		}
		chain := []sightmap.Element{*root.Element, *leaf.Element}
		if names := m.NamesForChain(chain, ""); !reflect.DeepEqual(names, []string{name}) {
			t.Errorf("NamesForChain(%s) = %v, want [%s]", id, names, name)
		}
	}
}

// Definitions the selector text cannot order keep the first-match tie-break
// and are still reported as a conflict.
func TestMatch_IncomparableClaimsStayAConflict(t *testing.T) {
	root := node("root", "div", nil,
		node("btn", "button", []string{"primary", "submit"}),
	)
	defs := []sightmap.ComponentDef{
		{Name: "Primary", Selectors: []string{"button.primary"}},
		{Name: "Submit", Selectors: []string{"button.submit"}},
	}
	m := match.NewMatcher(&sightmap.Corpus{GlobalComponents: defs})
	res := m.Match(root, "")
	if got := res[root.Children[0]].Name; got != "Primary" {
		t.Errorf("first declared should win an unordered overlap, got %s", got)
	}
	if c := m.Conflicts(root, ""); len(c) != 1 || len(c[0].Names) != 2 {
		t.Errorf("expected one two-way conflict, got %+v", c)
	}
	chain := []sightmap.Element{*root.Element, *root.Children[0].Element}
	if names := m.NamesForChain(chain, ""); !reflect.DeepEqual(names, []string{"Primary", "Submit"}) {
		t.Errorf("NamesForChain = %v, want both", names)
	}
}

// A child definition's chain is more specific than a global's bare selector, and a
// child-combinator chain more specific than a descendant one.
func TestMatch_MoreSpecificChainWins(t *testing.T) {
	root := node("root", "div", nil,
		nodeAttr("card", "div", map[string]string{"data-component": "Card"},
			node("btn", "button", nil),
		),
	)
	defs := []sightmap.ComponentDef{
		{Name: "Button", Selectors: []string{"button"}},
		{Name: "CardButton", Selectors: []string{`[data-component="Card"] button`}},
		{Name: "CardOwnButton", Selectors: []string{`[data-component="Card"] > button`}},
	}
	m := match.NewMatcher(&sightmap.Corpus{GlobalComponents: defs})
	res := m.Match(root, "")
	if got := res[root.Children[0].Children[0]].Name; got != "CardOwnButton" {
		t.Errorf("got %s, want CardOwnButton", got)
	}
	if c := m.Conflicts(root, ""); len(c) != 0 {
		t.Errorf("ordered chains are not conflicts, got %+v", c)
	}
}
