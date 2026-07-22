package comps_test

import (
	"testing"

	"github.com/sightmap/sightmap/go/comps"
)

// buildTestTree constructs the shared test tree used across multiple tests:
//
//	root    (role=document,    isVisible=true)
//	  section (role=generic,   isVisible=true)
//	    nav   (role=navigation, isVisible=true,  isInteractive=false)
//	      a   (role=link,       isVisible=true,  isInteractive=true, inViewport=true)
//	      a   (role=link,       isVisible=true,  isInteractive=true, inViewport=false)
//	    button (role=button,    isVisible=true,  isInteractive=true, inViewport=true, bounds={10,10,100,30})
//	  p     (role=paragraph,   isVisible=false)
//	    span  (role=StaticText, isVisible=false)
func buildTestTree() *comps.ComponentNode {
	a1 := &comps.ComponentNode{
		Id:            "a1",
		Role:          "link",
		IsVisible:     true,
		IsInteractive: true,
		InViewport:    true,
	}
	a2 := &comps.ComponentNode{
		Id:            "a2",
		Role:          "link",
		IsVisible:     true,
		IsInteractive: true,
		InViewport:    false,
	}
	nav := &comps.ComponentNode{
		Id:            "nav",
		Role:          "navigation",
		IsVisible:     true,
		IsInteractive: false,
		Children:      []*comps.ComponentNode{a1, a2},
	}
	button := &comps.ComponentNode{
		Id:            "button",
		Role:          "button",
		IsVisible:     true,
		IsInteractive: true,
		InViewport:    true,
		Bounds:        &comps.Bounds{X: 10, Y: 10, Width: 100, Height: 30},
	}
	section := &comps.ComponentNode{
		Id:        "section",
		Role:      "generic",
		IsVisible: true,
		Children:  []*comps.ComponentNode{nav, button},
	}
	span := &comps.ComponentNode{
		Id:        "span",
		Role:      "StaticText",
		IsVisible: false,
	}
	p := &comps.ComponentNode{
		Id:        "p",
		Role:      "paragraph",
		IsVisible: false,
		Children:  []*comps.ComponentNode{span},
	}
	root := &comps.ComponentNode{
		Id:        "root",
		Role:      "document",
		IsVisible: true,
		Children:  []*comps.ComponentNode{section, p},
	}
	return root
}

// assertNodeIds verifies that nodes have exactly the expected IDs in order.
func assertNodeIds(t *testing.T, nodes []*comps.ComponentNode, wantIds []string) {
	t.Helper()
	gotIds := make([]string, len(nodes))
	for i, n := range nodes {
		gotIds[i] = n.Id
	}
	if len(nodes) != len(wantIds) {
		t.Errorf("got %d nodes %v, want %d %v", len(nodes), gotIds, len(wantIds), wantIds)
		return
	}
	for i, want := range wantIds {
		if nodes[i].Id != want {
			t.Errorf("nodes[%d].Id = %q, want %q", i, nodes[i].Id, want)
		}
	}
}

// ---- Walk ----

func TestWalk_AllNodes_DFSPreOrder(t *testing.T) {
	root := buildTestTree()

	var visitedIds []string
	var visitedDepths []int
	comps.Walk(root, func(node *comps.ComponentNode, depth int) bool {
		visitedIds = append(visitedIds, node.Id)
		visitedDepths = append(visitedDepths, depth)
		return true
	})

	wantIds := []string{"root", "section", "nav", "a1", "a2", "button", "p", "span"}
	wantDepths := []int{0, 1, 2, 3, 3, 2, 1, 2}

	if len(visitedIds) != len(wantIds) {
		t.Fatalf("visited %v, want %v", visitedIds, wantIds)
	}
	for i := range wantIds {
		if visitedIds[i] != wantIds[i] {
			t.Errorf("node[%d]: got %q, want %q", i, visitedIds[i], wantIds[i])
		}
		if visitedDepths[i] != wantDepths[i] {
			t.Errorf("depth[%d]: got %d, want %d (node %q)",
				i, visitedDepths[i], wantDepths[i], visitedIds[i])
		}
	}
}

func TestWalk_StopsDescending_WhenFnReturnsFalse(t *testing.T) {
	root := buildTestTree()

	var visitedIds []string
	comps.Walk(root, func(node *comps.ComponentNode, depth int) bool {
		visitedIds = append(visitedIds, node.Id)
		// Returning false for "section" skips its entire subtree.
		return node.Id != "section"
	})

	// root and section are visited; section's children (nav, a1, a2, button)
	// are skipped. p and span are still visited.
	wantIds := []string{"root", "section", "p", "span"}
	if len(visitedIds) != len(wantIds) {
		t.Fatalf("visited %v, want %v", visitedIds, wantIds)
	}
	for i, want := range wantIds {
		if visitedIds[i] != want {
			t.Errorf("node[%d]: got %q, want %q", i, visitedIds[i], want)
		}
	}
}

// ---- Collect + built-in predicates ----

func TestCollect_IsInteractive(t *testing.T) {
	root := buildTestTree()
	got := comps.Collect(root, comps.IsInteractive())
	assertNodeIds(t, got, []string{"a1", "a2", "button"})
}

func TestCollect_InViewport(t *testing.T) {
	root := buildTestTree()
	got := comps.Collect(root, comps.InViewport())
	assertNodeIds(t, got, []string{"a1", "button"})
}

func TestCollect_And_VisibleAndInteractive(t *testing.T) {
	root := buildTestTree()
	// All interactive nodes in the tree are also visible, so the result
	// should be the same as IsInteractive() alone.
	got := comps.Collect(root, comps.And(comps.IsVisible(), comps.IsInteractive()))
	assertNodeIds(t, got, []string{"a1", "a2", "button"})
}

func TestCollect_Not_Visible(t *testing.T) {
	root := buildTestTree()
	got := comps.Collect(root, comps.Not(comps.IsVisible()))
	assertNodeIds(t, got, []string{"p", "span"})
}

// ---- Filter ----

func TestFilter_IsInteractive_StructuralConnectors(t *testing.T) {
	root := buildTestTree()
	filtered := comps.Filter(root, comps.IsInteractive())

	if filtered == nil {
		t.Fatal("Filter returned nil, want non-nil")
	}

	// root → structural connector
	if filtered.Id != "root" {
		t.Errorf("root id: got %q, want root", filtered.Id)
	}
	if len(filtered.Children) != 1 {
		t.Fatalf("root children: got %d, want 1 (section only; p excluded)",
			len(filtered.Children))
	}

	// section → structural connector
	section := filtered.Children[0]
	if section.Id != "section" {
		t.Errorf("section id: got %q, want section", section.Id)
	}
	if len(section.Children) != 2 {
		t.Fatalf("section children: got %d, want 2 (nav, button)", len(section.Children))
	}

	// nav → structural connector (holds a1, a2)
	nav := section.Children[0]
	if nav.Id != "nav" {
		t.Errorf("nav id: got %q, want nav", nav.Id)
	}
	if len(nav.Children) != 2 {
		t.Fatalf("nav children: got %d, want 2", len(nav.Children))
	}
	if nav.Children[0].Id != "a1" || nav.Children[1].Id != "a2" {
		t.Errorf("nav children ids: got %q %q, want a1 a2",
			nav.Children[0].Id, nav.Children[1].Id)
	}

	// button → directly matched
	button := section.Children[1]
	if button.Id != "button" {
		t.Errorf("button id: got %q, want button", button.Id)
	}
	if len(button.Children) != 0 {
		t.Errorf("button children: got %d, want 0", len(button.Children))
	}
}

func TestFilter_HasBounds_OnlyButtonMatches(t *testing.T) {
	root := buildTestTree()
	filtered := comps.Filter(root, comps.HasBounds())

	if filtered == nil {
		t.Fatal("Filter returned nil, want non-nil")
	}

	// root → structural connector
	if filtered.Id != "root" {
		t.Errorf("root id: got %q, want root", filtered.Id)
	}
	if len(filtered.Children) != 1 {
		t.Fatalf("root children: got %d, want 1 (section only; p excluded)",
			len(filtered.Children))
	}

	// section → structural connector
	section := filtered.Children[0]
	if section.Id != "section" {
		t.Errorf("section id: got %q, want section", section.Id)
	}
	// nav has no bounds and no matching descendants → excluded
	if len(section.Children) != 1 {
		t.Fatalf("section children: got %d, want 1 (button only; nav excluded)",
			len(section.Children))
	}

	// button → directly matched
	button := section.Children[0]
	if button.Id != "button" {
		t.Errorf("section.Children[0] id: got %q, want button", button.Id)
	}
}

func TestFilter_NothingMatches_ReturnsNil(t *testing.T) {
	root := buildTestTree()
	// IsIgnored is false on every node in the test tree.
	neverMatch := func(node *comps.ComponentNode, _ int) (bool, bool) {
		return node.IsIgnored, true
	}
	filtered := comps.Filter(root, neverMatch)
	if filtered != nil {
		t.Errorf("Filter returned %q, want nil", filtered.Id)
	}
}

// ---- Or combinator ----

func TestCollect_Or_InteractiveOrNotVisible(t *testing.T) {
	root := buildTestTree()
	// Interactive nodes (a1, a2, button) plus not-visible nodes (p, span).
	got := comps.Collect(root, comps.Or(comps.IsInteractive(), comps.Not(comps.IsVisible())))
	assertNodeIds(t, got, []string{"a1", "a2", "button", "p", "span"})
}
