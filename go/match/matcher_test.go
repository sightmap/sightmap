package match_test

import (
	"sort"
	"testing"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/match"
)

// ---------- tree builders ----------------------------------------------------

func node(id, tag string, classes []string, children ...*comps.ComponentNode) *comps.ComponentNode {
	return &comps.ComponentNode{
		Id:        id,
		Selector:  &comps.SelectorPart{Tag: tag, Classes: classes},
		IsVisible: true,
		Children:  children,
	}
}

func nodeAttr(id, tag string, attrs map[string]string, children ...*comps.ComponentNode) *comps.ComponentNode {
	return &comps.ComponentNode{
		Id:       id,
		Selector: &comps.SelectorPart{Tag: tag, Attrs: attrs},
		Children: children,
	}
}

func nodeInvisible(id, tag string) *comps.ComponentNode {
	return &comps.ComponentNode{
		Id:        id,
		IsVisible: false,
		Selector:  &comps.SelectorPart{Tag: tag},
	}
}

// ---------- helpers ----------------------------------------------------------

// applyDefs is a shorthand: build defs, apply, return matched node IDs by name.
func applyDefs(t *testing.T, root *comps.ComponentNode, defs []match.ComponentDef) map[string][]string {
	t.Helper()
	m := match.ApplySightmap(root, defs)
	result := make(map[string][]string)
	for node, sm := range m {
		result[sm.Name] = append(result[sm.Name], node.Id)
	}
	// Sort IDs for deterministic comparison.
	for k := range result {
		sort.Strings(result[k])
	}
	return result
}

func assertIDs(t *testing.T, got map[string][]string, name string, wantIDs ...string) {
	t.Helper()
	sort.Strings(wantIDs)
	ids := got[name]
	if len(ids) != len(wantIDs) {
		t.Errorf("%s: got IDs %v, want %v", name, ids, wantIDs)
		return
	}
	for i := range ids {
		if ids[i] != wantIDs[i] {
			t.Errorf("%s: got IDs %v, want %v", name, ids, wantIDs)
			return
		}
	}
}

func assertNoMatch(t *testing.T, got map[string][]string, name string) {
	t.Helper()
	if ids := got[name]; len(ids) > 0 {
		t.Errorf("%s: expected no match, got IDs %v", name, ids)
	}
}

// ---------- core NFA tests ---------------------------------------------------

// Tree for most tests:
//   root (div)
//     nav.main-nav
//       a.nav-link        [id=a1]
//       a.nav-link        [id=a2]
//     section
//       div.container
//         button.submit   [id=btn1]
//         button.submit   [id=btn2]
//     form.search
//       input[type=email] [id=inp1]
//       button.submit     [id=btn3]   ← direct child of form

func makeTree() *comps.ComponentNode {
	return node("root", "div", nil,
		node("nav", "nav", []string{"main-nav"},
			node("a1", "a", []string{"nav-link"}),
			node("a2", "a", []string{"nav-link"}),
		),
		node("sec", "section", nil,
			node("con", "div", []string{"container"},
				node("btn1", "button", []string{"submit"}),
				node("btn2", "button", []string{"submit"}),
			),
		),
		node("form", "form", []string{"search"},
			nodeAttr("inp1", "input", map[string]string{"type": "email"}),
			node("btn3", "button", []string{"submit"}),
		),
	)
}

func TestSingleTag(t *testing.T) {
	root := makeTree()
	defs := []match.ComponentDef{{Name: "Nav", Selectors: []string{"nav"}}}
	got := applyDefs(t, root, defs)
	assertIDs(t, got, "Nav", "nav")
}

func TestDescendant_TwoLevels(t *testing.T) {
	// nav.main-nav a.nav-link: anchor inside nav, bridging intermediate nodes.
	root := makeTree()
	defs := []match.ComponentDef{{Name: "NavLink", Selectors: []string{"nav.main-nav a.nav-link"}}}
	got := applyDefs(t, root, defs)
	assertIDs(t, got, "NavLink", "a1", "a2")
}

func TestDescendant_DeepNesting(t *testing.T) {
	// div button: button anywhere inside a div, even deeply nested.
	root := makeTree()
	defs := []match.ComponentDef{{Name: "Btn", Selectors: []string{"div button"}}}
	got := applyDefs(t, root, defs)
	// root is div, so all buttons (btn1, btn2, btn3) are descendants.
	assertIDs(t, got, "Btn", "btn1", "btn2", "btn3")
}

func TestDirectChild_Matches(t *testing.T) {
	// form.search > button.submit: button DIRECTLY under form.
	root := makeTree()
	defs := []match.ComponentDef{{Name: "SubmitBtn", Selectors: []string{"form.search > button.submit"}}}
	got := applyDefs(t, root, defs)
	assertIDs(t, got, "SubmitBtn", "btn3")
}

func TestDirectChild_DoesNotMatchGrandchild(t *testing.T) {
	// form.search > button.submit should NOT match btn1/btn2 (inside section > div.container).
	root := makeTree()
	defs := []match.ComponentDef{{Name: "DirectBtn", Selectors: []string{"div.container > button"}}}
	got := applyDefs(t, root, defs)
	// btn1 and btn2 are direct children of div.container
	assertIDs(t, got, "DirectBtn", "btn1", "btn2")

	// Verify btn3 (direct child of form.search, not div.container) is NOT matched.
	for _, id := range got["DirectBtn"] {
		if id == "btn3" {
			t.Error("btn3 is not a child of div.container, should not match")
		}
	}
}

func TestDirectChild_TwoHops(t *testing.T) {
	// A > B > C chain.
	root := &comps.ComponentNode{
		Id:       "r",
		Selector: &comps.SelectorPart{Tag: "ul"},
		Children: []*comps.ComponentNode{
			{
				Id:       "li",
				Selector: &comps.SelectorPart{Tag: "li"},
				Children: []*comps.ComponentNode{
					{Id: "a", Selector: &comps.SelectorPart{Tag: "a"}},
				},
			},
		},
	}
	defs := []match.ComponentDef{{Name: "Link", Selectors: []string{"ul > li > a"}}}
	got := applyDefs(t, root, defs)
	assertIDs(t, got, "Link", "a")
}

func TestDirectChild_NotTwoLevelsDeep(t *testing.T) {
	// ul > a should NOT match a inside li (two levels deep).
	root := &comps.ComponentNode{
		Id:       "r",
		Selector: &comps.SelectorPart{Tag: "ul"},
		Children: []*comps.ComponentNode{
			{
				Id:       "li",
				Selector: &comps.SelectorPart{Tag: "li"},
				Children: []*comps.ComponentNode{
					{Id: "a", Selector: &comps.SelectorPart{Tag: "a"}},
				},
			},
		},
	}
	defs := []match.ComponentDef{{Name: "Link", Selectors: []string{"ul > a"}}}
	got := applyDefs(t, root, defs)
	assertNoMatch(t, got, "Link")
}

func TestMultiQuery_SinglePass(t *testing.T) {
	// Two definitions matched in one pass; verify no state leakage between them.
	root := makeTree()
	defs := []match.ComponentDef{
		{Name: "NavLink", Selectors: []string{"nav.main-nav a.nav-link"}},
		{Name: "SearchInput", Selectors: []string{`input[type="email"]`}},
	}
	got := applyDefs(t, root, defs)
	assertIDs(t, got, "NavLink", "a1", "a2")
	assertIDs(t, got, "SearchInput", "inp1")
}

func TestFirstMatchWins(t *testing.T) {
	// Two definitions both match the same node; earliest in definition order wins.
	root := node("root", "div", nil,
		node("btn", "button", []string{"primary", "submit"}),
	)
	defs := []match.ComponentDef{
		{Name: "Primary", Selectors: []string{"button.primary"}},
		{Name: "Submit", Selectors: []string{"button.submit"}},
	}
	got := applyDefs(t, root, defs)
	// "btn" should be claimed by "Primary" (first in list).
	assertIDs(t, got, "Primary", "btn")
	assertNoMatch(t, got, "Submit")
}

func TestInvisibleNodesIncluded(t *testing.T) {
	// Matching runs on the FULL tree — invisible nodes still match.
	root := node("root", "div", nil,
		nodeInvisible("hidden", "button"),
		node("visible", "button", nil),
	)
	defs := []match.ComponentDef{{Name: "Btn", Selectors: []string{"button"}}}
	got := applyDefs(t, root, defs)
	// Both nodes should match, regardless of IsVisible.
	assertIDs(t, got, "Btn", "hidden", "visible")
}

func TestNilSelectorNode_DoesNotMatchSpecificRule(t *testing.T) {
	// A node with nil Selector cannot satisfy a specific selector rule.
	root := &comps.ComponentNode{
		Id:       "root",
		Selector: nil,
		Children: []*comps.ComponentNode{
			{Id: "child", Selector: &comps.SelectorPart{Tag: "button"}},
		},
	}
	defs := []match.ComponentDef{{Name: "Btn", Selectors: []string{"button"}}}
	got := applyDefs(t, root, defs)
	// Only "child" should match; "root" has no selector.
	assertIDs(t, got, "Btn", "child")
}

func TestAttrSelector_Substring(t *testing.T) {
	root := node("root", "div", nil,
		nodeAttr("n1", "div", map[string]string{"aria-label": "Search products"}),
		nodeAttr("n2", "div", map[string]string{"aria-label": "Filter results"}),
	)
	defs := []match.ComponentDef{{Name: "Search", Selectors: []string{`[aria-label*="Search"]`}}}
	got := applyDefs(t, root, defs)
	assertIDs(t, got, "Search", "n1")
}

func TestNotPseudo(t *testing.T) {
	root := node("root", "div", nil,
		node("enabled", "button", []string{"primary"}),
		node("disabled", "button", []string{"primary", "disabled"}),
	)
	defs := []match.ComponentDef{{Name: "Active", Selectors: []string{"button:not(.disabled)"}}}
	got := applyDefs(t, root, defs)
	assertIDs(t, got, "Active", "enabled")
}

func TestFramePrefixedIDs_Traversed(t *testing.T) {
	// Frame-prefixed component IDs (e.g. "1_5") are treated as regular nodes.
	root := node("0_root", "div", nil,
		node("1_nav", "nav", nil,
			node("1_5", "a", []string{"nav-link"}),
		),
	)
	defs := []match.ComponentDef{{Name: "NavLink", Selectors: []string{"nav a.nav-link"}}}
	got := applyDefs(t, root, defs)
	assertIDs(t, got, "NavLink", "1_5")
}

func TestMobileSyntheticSelector(t *testing.T) {
	// Native mobile component with synthetic selectors.
	root := &comps.ComponentNode{
		Id:       "screen",
		Selector: &comps.SelectorPart{Tag: "UIView"},
		Children: []*comps.ComponentNode{
			{
				Id: "btn",
				Selector: &comps.SelectorPart{
					Tag:   "UIButton",
					Attrs: map[string]string{"label": "Add to Cart"},
				},
			},
		},
	}
	defs := []match.ComponentDef{
		{Name: "AddToCart", Selectors: []string{`UIButton[label="Add to Cart"]`}},
	}
	got := applyDefs(t, root, defs)
	assertIDs(t, got, "AddToCart", "btn")
}

func TestParseQueries_InvalidSelectorSkipped(t *testing.T) {
	defs := []match.ComponentDef{
		{Name: "Valid", Selectors: []string{"button"}},
		{Name: "Invalid", Selectors: []string{":hover"}}, // unsupported pseudo
	}
	queries, errs := match.ParseQueries(defs)
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}
	if len(queries) != 1 || queries[0].Name != "Valid" {
		t.Errorf("expected 1 valid query named Valid, got %v", queries)
	}
}

func TestApplySightmap_NilRoot(t *testing.T) {
	defs := []match.ComponentDef{{Name: "X", Selectors: []string{"div"}}}
	result := match.ApplySightmap(nil, defs)
	if result != nil {
		t.Errorf("expected nil for nil root, got %v", result)
	}
}

func TestApplySightmap_EmptyDefs(t *testing.T) {
	root := node("r", "div", nil)
	result := match.ApplySightmap(root, nil)
	if result != nil {
		t.Errorf("expected nil for empty defs, got %v", result)
	}
}
