package match_test

// Regression tests for child-component annotation through intermediate structural
// and ignored AX nodes.

import (
	"github.com/sightmap/sightmap/go/sightmap"
	"testing"

	"github.com/sightmap/sightmap/go/match"
)

// buildBreadcrumbTree returns a component tree that mirrors the homedepot
// breadcrumb DOM:
//
//	nav[data-component=...] (Breadcrumb container)
//	  ol (list — structural, no data attributes)
//	    li (listitem — structural)
//	      a (link — BreadcrumbLink target)
func buildBreadcrumbTree() *sightmap.ComponentNode {
	return &sightmap.ComponentNode{
		Id:      "page",
		Element: &sightmap.Element{Tag: "div"},
		Children: []*sightmap.ComponentNode{
			{
				Id:   "breadcrumb",
				Role: "navigation",
				Element: &sightmap.Element{
					Tag:   "nav",
					Attrs: map[string]string{"data-component": "breadcrumbs:Breadcrumbs:v14.0.1"},
				},
				Children: []*sightmap.ComponentNode{
					{
						Id:      "list",
						Role:    "list",
						Element: &sightmap.Element{Tag: "ol"},
						Children: []*sightmap.ComponentNode{
							{
								Id:        "listitem",
								Role:      "listitem",
								IsIgnored: false,
								Element:   &sightmap.Element{Tag: "li"},
								Children: []*sightmap.ComponentNode{
									{
										Id:            "link",
										Role:          "link",
										IsInteractive: true,
										IsVisible:     true,
										Element:       &sightmap.Element{Tag: "a"},
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
// BreadcrumbLink must be annotated even though list/listitem
// structural nodes sit between the Breadcrumb container and the link.
func TestChildAnnotation_ThroughStructuralIntermediate(t *testing.T) {
	root := buildBreadcrumbTree()
	defs := []sightmap.ComponentDef{
		{Name: "Breadcrumb", Selectors: []string{`[data-component^="breadcrumbs:Breadcrumbs"]`}},
		// Flattened child selector: parent + " " + child
		{Name: "BreadcrumbLink", Selectors: []string{`[data-component^="breadcrumbs:Breadcrumbs"] a`}},
	}

	result := match.ApplySightmap(root, defs)

	// Find the specific nodes
	var breadcrumbNode, linkNode *sightmap.ComponentNode
	sightmap.Walk(root, func(n *sightmap.ComponentNode, _ int) bool {
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
	defs := []sightmap.ComponentDef{
		// FooterLink: broad global "a" selector — listed FIRST (lower index)
		{Name: "FooterLink", Selectors: []string{`a`}},
		// Breadcrumb + scoped child — listed AFTER FooterLink
		{Name: "Breadcrumb", Selectors: []string{`[data-component^="breadcrumbs:Breadcrumbs"]`}},
		{Name: "BreadcrumbLink", Selectors: []string{`[data-component^="breadcrumbs:Breadcrumbs"] a`}},
	}

	result := match.ApplySightmap(root, defs)

	var linkNode *sightmap.ComponentNode
	sightmap.Walk(root, func(n *sightmap.ComponentNode, _ int) bool {
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
