package match_test

import (
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"

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

// TestFindConflicts_SameNameDistinctDefs: two DEFINITIONS that share a leaf name
// and both claim one node is a genuine conflict, and must be reported.
//
// A component name is unique only within its parent, so this is legal and — on a
// real page — common. Deduplicating claims by name hid it completely: an Intuit
// sign-in corpus had five such nodes and reported none of them. The shape here is
// exactly that one: the same component observed at two depths across different
// states, whose shallower definition over-matches via the descendant combinator.
func TestFindConflicts_SameNameDistinctDefs(t *testing.T) {
	root := node("root", "div", nil,
		node("widget", "div", []string{"widget"},
			node("card", "div", []string{"card"},
				node("recaptcha", "div", []string{"recaptcha"}),
			),
		),
	)
	defs := []sightmap.ComponentDef{
		{Name: "Recaptcha", Selectors: []string{".widget .recaptcha"}},
		{Name: "Recaptcha", Selectors: []string{".widget .card .recaptcha"}},
	}
	conflicts := match.NewMatcher(&sightmap.Corpus{GlobalComponents: defs}).Conflicts(root, "")
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d: %+v", len(conflicts), conflicts)
	}
	c := conflicts[0]
	if c.Node.Id != "recaptcha" {
		t.Errorf("conflict on wrong node: %s", c.Node.Id)
	}
	if len(c.Names) != 2 {
		t.Errorf("expected both claimants, got %v", c.Names)
	}
	// Names cannot tell them apart; Defs must.
	if len(c.Defs) != 2 || c.Defs[0] == c.Defs[1] {
		t.Fatalf("expected two distinct Defs, got %+v", c.Defs)
	}
	if c.Defs[0].Selectors[0] != ".widget .recaptcha" {
		t.Errorf("Defs must be index-aligned with Names in first-seen order, got %q", c.Defs[0].Selectors[0])
	}
}

// TestFindConflicts_OneDefManySelectors: a definition offering several
// alternative selectors is ONE claimant, however many of them match.
func TestFindConflicts_OneDefManySelectors(t *testing.T) {
	root := node("root", "div", nil,
		nodeAttr("btn", "button", map[string]string{"data-testid": "go", "data-qa": "go"}),
	)
	defs := []sightmap.ComponentDef{
		{Name: "Go", Selectors: []string{`[data-testid="go"]`, `[data-qa="go"]`}},
	}
	if c := match.NewMatcher(&sightmap.Corpus{GlobalComponents: defs}).Conflicts(root, ""); len(c) != 0 {
		t.Errorf("one def's alternative selectors must not self-conflict, got %+v", c)
	}
}

// TestFindConflicts_DefsAlignWithNames pins the index alignment the report relies
// on to disambiguate same-named claimants.
func TestFindConflicts_DefsAlignWithNames(t *testing.T) {
	root := node("root", "div", nil,
		nodeAttr("dlg", "div", map[string]string{"role": "dialog", "data-testid": "login"}),
	)
	defs := []sightmap.ComponentDef{
		{Name: "AppDialog", Selectors: []string{`[role="dialog"]`}},
		{Name: "LoginDialog", Selectors: []string{`[data-testid="login"]`}},
	}
	c := match.NewMatcher(&sightmap.Corpus{GlobalComponents: defs}).Conflicts(root, "")[0]
	if len(c.Defs) != len(c.Names) {
		t.Fatalf("Defs (%d) and Names (%d) must be index-aligned", len(c.Defs), len(c.Names))
	}
	for i := range c.Names {
		if c.Defs[i].Name != c.Names[i] {
			t.Errorf("index %d: Names=%q but Defs=%q", i, c.Names[i], c.Defs[i].Name)
		}
	}
}
