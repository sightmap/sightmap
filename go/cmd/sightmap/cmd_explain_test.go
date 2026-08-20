package main

import (
	"testing"

	"github.com/sightmap/sightmap/go/coverage"
	"github.com/sightmap/sightmap/go/sightmap"
)

func explainFixture() (root, nav, btn *sightmap.ComponentNode) {
	btn = &sightmap.ComponentNode{
		Id:            "b1",
		Role:          "button",
		Name:          "Refresh",
		IsInteractive: true,
		IsVisible:     true,
		Element:       &sightmap.Element{Tag: "button", Classes: []string{"refreshButton"}},
	}
	nav = &sightmap.ComponentNode{
		Id:       "n1",
		Role:     "navigation",
		Element:  &sightmap.Element{Tag: "one-appnav"},
		Children: []*sightmap.ComponentNode{btn},
	}
	root = &sightmap.ComponentNode{
		Id:       "root",
		Element:  &sightmap.Element{Tag: "div"},
		Children: []*sightmap.ComponentNode{nav},
	}
	return root, nav, btn
}

func TestSelectExplainNodes(t *testing.T) {
	root, nav, btn := explainFixture()

	if got := selectExplainNodes(root, "", "b1", "", 10); len(got) != 1 || got[0] != btn {
		t.Errorf("--id b1: got %v, want [btn]", got)
	}
	if got := selectExplainNodes(root, "", "", "refresh", 10); len(got) != 1 || got[0] != btn {
		t.Errorf("--grep refresh: got %v, want [btn] (case-insensitive on name)", got)
	}
	if got := selectExplainNodes(root, "one-appnav", "", "", 10); len(got) != 1 || got[0] != nav {
		t.Errorf("selector one-appnav: got %v, want [nav]", got)
	}
	if got := selectExplainNodes(root, "", "", "nomatch", 10); len(got) != 0 {
		t.Errorf("--grep nomatch: got %v, want []", got)
	}
}

func TestSelectExplainNodes_RespectsMax(t *testing.T) {
	// Two buttons under nav; --max 1 returns only the first in tree order.
	root, nav, _ := explainFixture()
	btn2 := &sightmap.ComponentNode{Id: "b2", Role: "button", Name: "Refresh again", IsInteractive: true, Element: &sightmap.Element{Tag: "button"}}
	nav.Children = append(nav.Children, btn2)
	if got := selectExplainNodes(root, "", "", "refresh", 1); len(got) != 1 {
		t.Errorf("--max 1: got %d nodes, want 1", len(got))
	}
}

func TestTierAndOwner(t *testing.T) {
	root, nav, btn := explainFixture()
	pm := coverage.BuildParentMap(root)
	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		nav: {Name: "NavBar"},
	}

	if tier, owner := tierAndOwner(nav, matches, pm); tier != "T1 (direct)" || owner != "NavBar" {
		t.Errorf("nav: got (%q,%q), want T1/NavBar", tier, owner)
	}
	if tier, owner := tierAndOwner(btn, matches, pm); tier != "T2 (scoped)" || owner != "NavBar" {
		t.Errorf("btn: got (%q,%q), want T2/NavBar", tier, owner)
	}
	// With no matches at all, everything is an orphan.
	if tier, _ := tierAndOwner(btn, map[*sightmap.ComponentNode]*sightmap.ComponentMatch{}, pm); tier != "T3 (orphan)" {
		t.Errorf("btn no-matches: got %q, want T3", tier)
	}
}
