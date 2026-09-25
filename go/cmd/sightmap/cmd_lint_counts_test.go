package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// These tests exercise computeSnapshotCounts end-to-end against tree JSON
// files shaped like real captures (every node carries an Element, as
// go/extract/build.go's elementFor does). They pin the ancestor-aware count
// contract for the [multi-instance-no-property] snapshot reconciliation:
// compound selectors must be counted by their full scope, not by their leaf.

// el is a small helper that builds a populated *sightmap.Element, the shape
// every node has in a real capture.
func el(tag string, classes ...string) *sightmap.Element {
	return &sightmap.Element{Tag: tag, Classes: classes}
}

// node builds a *ComponentNode with the given element and children.
func node(el *sightmap.Element, children ...*sightmap.ComponentNode) *sightmap.ComponentNode {
	return &sightmap.ComponentNode{Element: el, Children: children}
}

// writeSnapTreeJSON marshals root to a fresh .snap.tree.json in t.TempDir()
// and returns its path. (Named to avoid colliding with cmd_snapshot.go's
// writeTreeJSON, which has a different signature in this package.)
func writeSnapTreeJSON(t *testing.T, root sightmap.ComponentNode) string {
	t.Helper()
	b, err := json.Marshal(&root)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "p.snap.tree.json")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// reproTree is the captured-shape tree used by the core compound-selector
// tests:
//
//	<div.page>
//	  <div.sidebar>
//	    <div.card>      <- the ONLY node satisfying `.sidebar div.card`
//	  <div.other>
//	    <div.card>      <- a sibling .card NOT under .sidebar
//	    <div.card>      <- a second sibling .card NOT under .sidebar
//
// There are 3 div.card leaves, but `.sidebar div.card` resolves to one; the
// absent-ancestor `.missing div.card` resolves to zero. Under the prior
// leaf-only walk all three selectors were counted as `div.card` → 3.
func reproTree() sightmap.ComponentNode {
	sidebar := sightmap.ComponentNode{
		Element:  el("div", "sidebar"),
		Children: []*sightmap.ComponentNode{node(el("div", "card"))},
	}
	other := sightmap.ComponentNode{
		Element:  el("div", "other"),
		Children: []*sightmap.ComponentNode{node(el("div", "card")), node(el("div", "card"))},
	}
	return sightmap.ComponentNode{
		Element:  el("div", "page"),
		Children: []*sightmap.ComponentNode{&sidebar, &other},
	}
}

// TestComputeSnapshotCounts_CompoundDescendantScoped counts a compound
// descendant selector only at nodes whose ancestor chain satisfies the
// preceding parts. This is the core bug: previously the leaf `div.card` was
// matched against every node, inflating `.sidebar div.card` to 3 and masking
// the genuinely-broken `.missing div.card` (real count 0) as a healthy 3.
func TestComputeSnapshotCounts_CompoundDescendantScoped(t *testing.T) {
	corpus := &sightmap.Corpus{
		GlobalComponents: []sightmap.ComponentDef{
			{Name: "Card", Selectors: []string{".sidebar div.card"}},
			{Name: "BrokenCard", Selectors: []string{".missing div.card"}},
		},
	}
	root := reproTree()
	treeFile := writeSnapTreeJSON(t, root)

	counts, err := computeSnapshotCounts(corpus, []string{treeFile})
	if err != nil {
		t.Fatal(err)
	}
	if counts["Card"] != 1 {
		t.Errorf("Card (.sidebar div.card) = %d, want 1 (only the .card under .sidebar)", counts["Card"])
	}
	if counts["BrokenCard"] != 0 {
		t.Errorf("BrokenCard (.missing div.card) = %d, want 0 (no .missing ancestor → absent → 0)", counts["BrokenCard"])
	}
	if _, ok := counts["BrokenCard"]; !ok {
		t.Error("BrokenCard must be present in counts with value 0 (absent-selector contract), not missing")
	}
}

// TestComputeSnapshotCounts_DirectChildScoped counts a `>` combinator only at
// direct children. The prior leaf-only walk counted every `input` in the tree,
// so `form.search > input` returned 3 instead of 1.
func TestComputeSnapshotCounts_DirectChildScoped(t *testing.T) {
	root := sightmap.ComponentNode{
		Element: el("div", "page"),
		Children: []*sightmap.ComponentNode{
			{
				Element: el("form", "search"),
				Children: []*sightmap.ComponentNode{
					node(el("input")),                  // direct child → matches
					node(el("div"), node(el("input"))), // grandchild → does NOT match
				},
			},
			node(el("div", "other"), node(el("input"))), // input outside the form → does NOT match
		},
	}
	corpus := &sightmap.Corpus{
		GlobalComponents: []sightmap.ComponentDef{
			{Name: "SearchInput", Selectors: []string{"form.search > input"}},
		},
	}
	treeFile := writeSnapTreeJSON(t, root)
	counts, err := computeSnapshotCounts(corpus, []string{treeFile})
	if err != nil {
		t.Fatal(err)
	}
	if counts["SearchInput"] != 1 {
		t.Errorf("SearchInput (form.search > input) = %d, want 1 (only the direct child input)", counts["SearchInput"])
	}
}

// TestComputeSnapshotCounts_MaxAcrossTreeFiles preserves the documented "max
// across tree files" reduction: a component that matches one node in file A
// and two in file B reports 2, and a zero-match compound selector stays 0
// across both.
func TestComputeSnapshotCounts_MaxAcrossTreeFiles(t *testing.T) {
	fileA := writeSnapTreeJSON(t, sightmap.ComponentNode{
		Element: el("div", "page"),
		Children: []*sightmap.ComponentNode{
			{Element: el("div", "sidebar"), Children: []*sightmap.ComponentNode{node(el("div", "card"))}},
		},
	})
	fileB := writeSnapTreeJSON(t, sightmap.ComponentNode{
		Element: el("div", "page"),
		Children: []*sightmap.ComponentNode{
			{
				Element:  el("div", "sidebar"),
				Children: []*sightmap.ComponentNode{node(el("div", "card")), node(el("div", "card"))},
			},
			{Element: el("div", "other"), Children: []*sightmap.ComponentNode{node(el("div", "card"))}},
		},
	})
	corpus := &sightmap.Corpus{
		GlobalComponents: []sightmap.ComponentDef{
			{Name: "Card", Selectors: []string{".sidebar div.card"}},
			{Name: "BrokenCard", Selectors: []string{".missing div.card"}},
		},
	}
	counts, err := computeSnapshotCounts(corpus, []string{fileA, fileB})
	if err != nil {
		t.Fatal(err)
	}
	if counts["Card"] != 2 {
		t.Errorf("Card max across trees = %d, want 2 (fileA=1, fileB=2)", counts["Card"])
	}
	if counts["BrokenCard"] != 0 {
		t.Errorf("BrokenCard max across trees = %d, want 0 (no .missing ancestor in either tree)", counts["BrokenCard"])
	}
}

// TestComputeSnapshotCounts_MaxAcrossSelectors preserves the documented "max
// across a component's selectors" reduction. Two compound selectors of the
// same component share the leaf `div.card`; the broader one (under `.other`,
// matches 2) must win over the narrower one (under `.sidebar`, matches 1),
// giving a component count of 2 — not the leaf-only 3.
func TestComputeSnapshotCounts_MaxAcrossSelectors(t *testing.T) {
	root := reproTree()
	corpus := &sightmap.Corpus{
		GlobalComponents: []sightmap.ComponentDef{
			{Name: "Card", Selectors: []string{".sidebar div.card", ".other div.card"}},
		},
	}
	treeFile := writeSnapTreeJSON(t, root)
	counts, err := computeSnapshotCounts(corpus, []string{treeFile})
	if err != nil {
		t.Fatal(err)
	}
	if counts["Card"] != 2 {
		t.Errorf("Card max across selectors = %d, want 2 (.sidebar→1, .other→2)", counts["Card"])
	}
}

// TestComputeSnapshotCounts_UnrelatedComponentsIndependent verifies that
// overlapping selectors across components do not steal each other's counts:
// a node matching two broad single-part selectors is an instance of each
// component, matching the independent per-component count that the prior
// leaf-only walk produced.
func TestComputeSnapshotCounts_UnrelatedComponentsIndependent(t *testing.T) {
	root := sightmap.ComponentNode{
		Element: el("div", "page"),
		Children: []*sightmap.ComponentNode{
			node(el("button", "btn", "primary")),
			node(el("button", "btn")),
		},
	}
	corpus := &sightmap.Corpus{
		GlobalComponents: []sightmap.ComponentDef{
			{Name: "Primary", Selectors: []string{"button.btn.primary"}},
			{Name: "AnyBtn", Selectors: []string{"button.btn"}},
		},
	}
	treeFile := writeSnapTreeJSON(t, root)
	counts, err := computeSnapshotCounts(corpus, []string{treeFile})
	if err != nil {
		t.Fatal(err)
	}
	if counts["Primary"] != 1 {
		t.Errorf("Primary (button.btn.primary) = %d, want 1", counts["Primary"])
	}
	if counts["AnyBtn"] != 2 {
		t.Errorf("AnyBtn (button.btn) = %d, want 2 (counted independently of Primary)", counts["AnyBtn"])
	}
}

// TestComputeSnapshotCounts_EndToEnd_LintWithCounts runs the full
// reconciliation path: computeSnapshotCounts → LintWithCounts, and asserts the
// three documented annotation branches for the compound-selector case the
// prior leaf-only walk got wrong.
func TestComputeSnapshotCounts_EndToEnd_LintWithCounts(t *testing.T) {
	// Tree: 1 card under .sidebar (Card matches once), 0 cards under .missing
	// (BrokenCard matches zero), and 3 standalone buttons.btn-action
	// (AddToCart matches thrice).
	root := sightmap.ComponentNode{
		Element: el("div", "page"),
		Children: []*sightmap.ComponentNode{
			{Element: el("div", "sidebar"), Children: []*sightmap.ComponentNode{node(el("div", "card"))}},
			node(el("div", "other"),
				node(el("div", "card")),
				node(el("div", "card")),
			),
			node(el("button", "btn-action")),
			node(el("button", "btn-action")),
			node(el("button", "btn-action")),
		},
	}
	corpus := &sightmap.Corpus{
		GlobalComponents: []sightmap.ComponentDef{
			{Name: "Card", Selectors: []string{".sidebar div.card"}},
			{Name: "BrokenCard", Selectors: []string{".missing div.card"}},
			{Name: "AddToCart", Selectors: []string{"button.btn-action"}},
		},
	}
	treeFile := writeSnapTreeJSON(t, root)
	counts, err := computeSnapshotCounts(corpus, []string{treeFile})
	if err != nil {
		t.Fatal(err)
	}

	warns := sightmap.LintWithCounts(corpus, counts)
	rule := func(name string) *sightmap.LintWarning {
		for i := range warns {
			if warns[i].Component == name && warns[i].Rule == "multi-instance-no-property" {
				return &warns[i]
			}
		}
		return nil
	}

	if w := rule("Card"); w != nil {
		t.Errorf("Card (count=1) should be suppressed; got: %s", w.Message)
	}
	w := rule("BrokenCard")
	if w == nil {
		t.Fatal("expected BrokenCard multi-instance-no-property warning (count=0)")
	}
	if !strings.Contains(w.Message, "(0 matches in snapshot \u2014 selector may be broken)") {
		t.Errorf("BrokenCard (count=0) should be annotated 'may be broken'; got: %s", w.Message)
	}
	if strings.Contains(w.Message, "matched 0 times in snapshot") {
		t.Errorf("BrokenCard must use the may-be-broken annotation, not the matched-N form: %s", w.Message)
	}
	a := rule("AddToCart")
	if a == nil {
		t.Fatal("expected AddToCart multi-instance-no-property warning (count=3)")
	}
	if !strings.Contains(a.Message, "(matched 3 times in snapshot)") {
		t.Errorf("AddToCart (count=3) should be annotated 'matched 3 times'; got: %s", a.Message)
	}
}
