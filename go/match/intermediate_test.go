package match_test

// Regression tests for child-component annotation through intermediate structural
// and ignored AX nodes (go-0030/go-0031).

import (
	"testing"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/match"
)

// buildBreadcrumbTree returns a component tree that mirrors the homedepot
// breadcrumb DOM:
//
//	nav[data-component=...] (Breadcrumb container)
//	  ol (list — structural, no data attributes)
//	    li (listitem — structural)
//	      a (link — BreadcrumbLink target)
func buildBreadcrumbTree() *comps.ComponentNode {
	return &comps.ComponentNode{
		Id:       "page",
		Selector: &comps.SelectorPart{Tag: "div"},
		Children: []*comps.ComponentNode{
			{
				Id:   "breadcrumb",
				Role: "navigation",
				Selector: &comps.SelectorPart{
					Tag:   "nav",
					Attrs: map[string]string{"data-component": "breadcrumbs:Breadcrumbs:v14.0.1"},
				},
				Children: []*comps.ComponentNode{
					{
						Id:       "list",
						Role:     "list",
						Selector: &comps.SelectorPart{Tag: "ol"},
						Children: []*comps.ComponentNode{
							{
								Id:        "listitem",
								Role:      "listitem",
								IsIgnored: false,
								Selector:  &comps.SelectorPart{Tag: "li"},
								Children: []*comps.ComponentNode{
									{
										Id:            "link",
										Role:          "link",
										IsInteractive: true,
										IsVisible:     true,
										Selector:      &comps.SelectorPart{Tag: "a"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// TestChildAnnotation_ThroughStructuralIntermediate is the core regression test
// for go-0030: BreadcrumbLink must be annotated even though list/listitem
// structural nodes sit between the Breadcrumb container and the link.
func TestChildAnnotation_ThroughStructuralIntermediate(t *testing.T) {
	root := buildBreadcrumbTree()
	defs := []match.SightmapComponent{
		{Name: "Breadcrumb", Selectors: []string{`[data-component^="breadcrumbs:Breadcrumbs"]`}},
		// Flattened child selector: parent + " " + child
		{Name: "BreadcrumbLink", Selectors: []string{`[data-component^="breadcrumbs:Breadcrumbs"] a`}},
	}

	result := match.ApplySightmap(root, defs)

	// Find the specific nodes
	var breadcrumbNode, linkNode *comps.ComponentNode
	comps.Walk(root, func(n *comps.ComponentNode, _ int) bool {
		switch n.Id {
		case "breadcrumb":
			breadcrumbNode = n
		case "link":
			linkNode = n
		}
		return true
	})
	if breadcrumbNode == nil || linkNode == nil {
		t.Fatal("test tree is wrong: could not find expected nodes")
	}

	// Breadcrumb container must match.
	if sm := result[breadcrumbNode]; sm == nil || sm.Name != "Breadcrumb" {
		t.Errorf("Breadcrumb container: expected match 'Breadcrumb', got %v", sm)
	}

	// BreadcrumbLink must match the link node despite intermediate list/listitem.
	if sm := result[linkNode]; sm == nil {
		t.Error("BreadcrumbLink: link node has no ★ annotation — child not matched through structural intermediates")
	} else if sm.Name != "BreadcrumbLink" {
		t.Errorf("BreadcrumbLink: expected 'BreadcrumbLink', got %q", sm.Name)
	}
}

// TestChildAnnotation_BroadGlobalDoesNotSteal tests that a broad global selector
// ("a") does not steal the link node from the more specific scoped child
// selector ("[data-component^=...] a") via first-match-wins.
//
// This mirrors the homedepot case where FooterLink (selector "a") could
// claim a BreadcrumbLink node before the scoped match fires.
func TestChildAnnotation_BroadGlobalDoesNotSteal(t *testing.T) {
	root := buildBreadcrumbTree()
	defs := []match.SightmapComponent{
		// FooterLink: broad global "a" selector — listed FIRST (lower index)
		{Name: "FooterLink", Selectors: []string{`a`}},
		// Breadcrumb + scoped child — listed AFTER FooterLink
		{Name: "Breadcrumb", Selectors: []string{`[data-component^="breadcrumbs:Breadcrumbs"]`}},
		{Name: "BreadcrumbLink", Selectors: []string{`[data-component^="breadcrumbs:Breadcrumbs"] a`}},
	}

	result := match.ApplySightmap(root, defs)

	var linkNode *comps.ComponentNode
	comps.Walk(root, func(n *comps.ComponentNode, _ int) bool {
		if n.Id == "link" {
			linkNode = n
		}
		return true
	})
	if linkNode == nil {
		t.Fatal("link node not found")
	}

	sm := result[linkNode]
	if sm == nil {
		t.Error("link node has no annotation at all")
	} else if sm.Name != "BreadcrumbLink" {
		t.Errorf("link node claimed by %q instead of BreadcrumbLink — scoped child lost to broad global", sm.Name)
	}
}
