package match_test

import (
	"github.com/sightmap/sightmap/go/sightmap"
	"testing"

	"github.com/sightmap/sightmap/go/match"
)

// TestFindConflicts_NodeClaimedByTwoNames: one node matched by two distinct
// component names is a conflict.
func TestFindConflicts_NodeClaimedByTwoNames(t *testing.T) {
	root := node("root", "div", nil,
		nodeAttr("dlg", "div", map[string]string{"role": "dialog", "data-testid": "login"}),
	)
	defs := []sightmap.ComponentDef{
		{Name: "AppDialog", Selectors: []string{`[role="dialog"]`}},
		{Name: "LoginDialog", Selectors: []string{`[data-testid="login"]`}},
	}
	conflicts := match.NewMatcher(&sightmap.Corpus{GlobalComponents: defs}).Conflicts(root, "")
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d: %+v", len(conflicts), conflicts)
	}
	if conflicts[0].Node.Id != "dlg" {
		t.Errorf("conflict on wrong node: %s", conflicts[0].Node.Id)
	}
	if len(conflicts[0].Names) != 2 {
		t.Errorf("expected 2 competing names, got %v", conflicts[0].Names)
	}
}

// TestFindConflicts_SameNameManyNodes: one component name matching many nodes is
// normal, not a conflict.
func TestFindConflicts_SameNameManyNodes(t *testing.T) {
	root := node("root", "div", nil,
		node("c1", "div", []string{"card"}),
		node("c2", "div", []string{"card"}),
		node("c3", "div", []string{"card"}),
	)
	defs := []sightmap.ComponentDef{{Name: "Card", Selectors: []string{".card"}}}
	if c := match.NewMatcher(&sightmap.Corpus{GlobalComponents: defs}).Conflicts(root, ""); len(c) != 0 {
		t.Errorf("same name matching many nodes must not conflict, got %+v", c)
	}
}

// TestFindConflicts_DistinctNodes: two names matching two different nodes is not
// a conflict.
func TestFindConflicts_DistinctNodes(t *testing.T) {
	root := node("root", "div", nil,
		node("a", "div", []string{"a"}),
		node("b", "div", []string{"b"}),
	)
	defs := []sightmap.ComponentDef{
		{Name: "A", Selectors: []string{".a"}},
		{Name: "B", Selectors: []string{".b"}},
	}
	if c := match.NewMatcher(&sightmap.Corpus{GlobalComponents: defs}).Conflicts(root, ""); len(c) != 0 {
		t.Errorf("distinct nodes must not conflict, got %+v", c)
	}
}
