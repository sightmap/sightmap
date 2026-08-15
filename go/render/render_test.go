package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// node builds a ComponentNode for use in tests.
func node(id, role, name string, visible, interactive, ignored bool, children ...*sightmap.ComponentNode) *sightmap.ComponentNode {
	return &sightmap.ComponentNode{
		Id: id, Role: role, Name: name,
		IsVisible: visible, IsInteractive: interactive, IsIgnored: ignored,
		Children: children,
	}
}

// nodeWithTag builds a ComponentNode with a Selector tag set.
func nodeWithTag(id, role, name, tag string, visible, interactive, ignored bool, children ...*sightmap.ComponentNode) *sightmap.ComponentNode {
	n := node(id, role, name, visible, interactive, ignored, children...)
	n.Element = &sightmap.Element{Tag: tag}
	return n
}

// --- Filter tests ---

func TestFilter_NilRoot(t *testing.T) {
	comp := Filter(nil, nil)
	if comp != nil {
		t.Errorf("expected nil for nil root, got %+v", comp)
	}
}

func TestFilter_Drop_Invisible(t *testing.T) {
	root := node("1", "generic", "", false, false, false)
	comp := Filter(root, nil)
	if comp != nil {
		t.Errorf("expected nil for invisible non-interactive node, got role=%q", comp.Role)
	}
}

// An invisible node with no visible descendants renders nothing: it is made
// transparent (the node itself is dropped) and has nothing to promote. probe.js
// reports effective visibility, so an interactive control hidden by display:none
// (e.g. a closed menu's item) arrives here invisible with invisible children and
// must not appear — keeping the tree in lockstep with coverage, which skips the
// same nodes. (Contrast TestFilter_InvisibleAncestor_KeepsVisibleDescendant,
// where an invisible ancestor DOES have visible descendants.)
func TestFilter_Drop_InvisibleInteractive(t *testing.T) {
	root := node("1", "button", "Hidden", false /* visible */, true /* interactive */, false)
	if comp := Filter(root, nil); comp != nil {
		t.Errorf("expected nil for invisible interactive leaf, got role=%q", comp.Role)
	}
}

// TestFilter_InvisibleAncestor_KeepsVisibleDescendant guards the fix for an
// invisible ancestor that nonetheless has VISIBLE descendants — the zero-height
// <html> document root (probe.js reports it height 0 / ignored), or a
// visibility:hidden wrapper whose child overrides it with visibility:visible.
// The ancestor is made transparent (dropped as a node), but its visible child
// must survive; the whole subtree must NOT vanish (the bug this fixes: an
// invisible root dropped the entire rendered tree while coverage still counted
// the visible descendants).
func TestFilter_InvisibleAncestor_KeepsVisibleDescendant(t *testing.T) {
	child := node("2", "button", "Add", true /* visible */, true /* interactive */, false)
	root := node("1", "none", "", false /* visible */, false, false, child)
	comp := Filter(root, nil)
	if comp == nil {
		t.Fatal("expected the visible descendant to survive an invisible ancestor, got nil")
	}
	if comp.Role != "button" || comp.Name != "Add" {
		t.Errorf("expected surviving role=button name=Add, got role=%q name=%q", comp.Role, comp.Name)
	}
}

func TestFilter_Drop_Script(t *testing.T) {
	root := nodeWithTag("1", "none", "", "script", true, false, false)
	comp := Filter(root, nil)
	if comp != nil {
		t.Errorf("expected nil for script tag, got role=%q", comp.Role)
	}
}

func TestFilter_Drop_Style(t *testing.T) {
	root := nodeWithTag("1", "none", "", "style", true, false, false)
	comp := Filter(root, nil)
	if comp != nil {
		t.Errorf("expected nil for style tag, got role=%q", comp.Role)
	}
}

func TestFilter_Drop_Noscript(t *testing.T) {
	root := nodeWithTag("1", "none", "", "noscript", true, false, false)
	comp := Filter(root, nil)
	if comp != nil {
		t.Errorf("expected nil for noscript tag, got role=%q", comp.Role)
	}
}

func TestFilter_Drop_ScriptCaseInsensitive(t *testing.T) {
	root := nodeWithTag("1", "none", "", "SCRIPT", true, false, false)
	comp := Filter(root, nil)
	if comp != nil {
		t.Errorf("expected nil for SCRIPT tag (case-insensitive), got role=%q", comp.Role)
	}
}

func TestFilter_Transparent_NoneRole(t *testing.T) {
	btn := node("2", "button", "OK", true, true, false)
	root := node("1", "none", "", true, false, false, btn)
	comp := Filter(root, nil)
	if comp == nil {
		t.Fatal("expected non-nil comp (button child should be promoted)")
	}
	if comp.Role != "button" {
		t.Errorf("expected role=button, got %q", comp.Role)
	}
	if comp.Id != "2" {
		t.Errorf("expected id=2, got %q", comp.Id)
	}
}

func TestFilter_Transparent_Ignored(t *testing.T) {
	btn := node("2", "button", "OK", true, true, false)
	root := node("1", "generic", "", true, false, true /* ignored */, btn)
	comp := Filter(root, nil)
	if comp == nil {
		t.Fatal("expected non-nil comp (button child should be promoted)")
	}
	if comp.Role != "button" {
		t.Errorf("expected role=button, got %q", comp.Role)
	}
}

func TestFilter_Transparent_GenericSingleChild(t *testing.T) {
	btn := node("2", "button", "Submit", true, true, false)
	root := node("1", "generic", "", true, false, false, btn)
	comp := Filter(root, nil)
	if comp == nil {
		t.Fatal("expected non-nil comp")
	}
	if comp.Role != "button" {
		t.Errorf("expected role=button after generic passthrough, got %q", comp.Role)
	}
}

func TestFilter_Keep_GenericMultipleChildren(t *testing.T) {
	a := node("2", "link", "Home", true, true, false)
	b := node("3", "link", "About", true, true, false)
	root := node("1", "generic", "", true, false, false, a, b)
	comp := Filter(root, nil)
	if comp == nil {
		t.Fatal("expected non-nil comp")
	}
	if comp.Role != "generic" {
		t.Errorf("expected role=generic (2+ non-text children), got %q", comp.Role)
	}
	if len(comp.Children) != 2 {
		t.Errorf("expected 2 children, got %d", len(comp.Children))
	}
}

func TestFilter_CollapseStaticText_Solo(t *testing.T) {
	st := node("2", "StaticText", "Add to Cart", true, false, false)
	btn := node("1", "button", "", true, true, false, st)
	comp := Filter(btn, nil)
	if comp == nil {
		t.Fatal("expected non-nil comp")
	}
	if comp.Role != "button" {
		t.Errorf("expected role=button, got %q", comp.Role)
	}
	if comp.Name != "Add to Cart" {
		t.Errorf("expected Name='Add to Cart', got %q", comp.Name)
	}
	if len(comp.Children) != 0 {
		t.Errorf("expected 0 children after collapse, got %d", len(comp.Children))
	}
}

func TestFilter_CollapseStaticText_SoloPreservesExistingName(t *testing.T) {
	st := node("2", "StaticText", "Add to Cart", true, false, false)
	btn := node("1", "button", "A11Y Name", true, true, false, st)
	comp := Filter(btn, nil)
	if comp == nil {
		t.Fatal("expected non-nil comp")
	}
	if comp.Name != "A11Y Name" {
		t.Errorf("expected Name='A11Y Name' (preserved), got %q", comp.Name)
	}
}

func TestFilter_MergeAdjacentStaticText(t *testing.T) {
	t1 := node("2", "StaticText", "Hello", true, false, false)
	t2 := node("3", "StaticText", "World", true, false, false)
	// paragraph is visible but not interactive — we need IsInteractive or IsVisible+role kept
	// Use a heading so it's kept (not generic)
	root := node("1", "paragraph", "", true, false, false, t1, t2)
	comp := Filter(root, nil)
	if comp == nil {
		t.Fatal("expected non-nil comp")
	}
	// All children are StaticText → collapse into parent Name
	if comp.Name != "Hello World" {
		t.Errorf("expected Name='Hello World', got %q", comp.Name)
	}
	if len(comp.Children) != 0 {
		t.Errorf("expected 0 children (all text collapsed), got %d", len(comp.Children))
	}
}

func TestFilter_MergeAdjacentStaticText_Mixed(t *testing.T) {
	// When there's a mix of text and non-text children, adjacent text nodes
	// should be merged but the list should not be fully collapsed.
	t1 := node("2", "StaticText", "Hello", true, false, false)
	t2 := node("3", "StaticText", "World", true, false, false)
	btn := node("4", "button", "Click", true, true, false)
	root := node("1", "navigation", "", true, false, false, t1, t2, btn)
	comp := Filter(root, nil)
	if comp == nil {
		t.Fatal("expected non-nil comp")
	}
	// Should have 2 children: merged StaticText + button
	if len(comp.Children) != 2 {
		t.Errorf("expected 2 children (merged text + button), got %d", len(comp.Children))
	}
	if comp.Children[0].Role != "StaticText" {
		t.Errorf("expected first child role=StaticText, got %q", comp.Children[0].Role)
	}
	if comp.Children[0].Name != "Hello World" {
		t.Errorf("expected merged text='Hello World', got %q", comp.Children[0].Name)
	}
}

func TestFilter_MatchProtectsFromTransparency(t *testing.T) {
	div := node("1", "none", "", true, false, false)
	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		div: {Name: "ProductCard"},
	}
	comp := Filter(div, matches)
	if comp == nil {
		t.Fatal("expected non-nil comp: match should protect from transparency")
	}
	if comp.Role != "ProductCard" {
		t.Errorf("expected role='ProductCard' (match name replaces structural role), got %q", comp.Role)
	}
	if comp.Match == nil {
		t.Error("expected Match to be non-nil")
	}
}

func TestFilter_MatchedLinkRoleReplaced(t *testing.T) {
	// Mirrors the canonical extractor's DisplayRole(): match name always replaces AX role,
	// even for interactive roles like "link". No ★ annotation.
	a := node("1", "link", "Home", true, true, false)
	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		a: {Name: "NavLink"},
	}
	comp := Filter(a, matches)
	if comp == nil {
		t.Fatal("expected non-nil comp")
	}
	if comp.Role != "NavLink" {
		t.Errorf("expected role='NavLink' (match name replaces AX role), got %q", comp.Role)
	}
	if comp.Match == nil || comp.Match.Name != "NavLink" {
		t.Errorf("expected Match.Name='NavLink', got %v", comp.Match)
	}
}

func TestFilter_StructuralRoleReplacedByMatchName(t *testing.T) {
	nav := node("1", "navigation", "", true, false, false)
	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		nav: {Name: "GlobalNav"},
	}
	comp := Filter(nav, matches)
	if comp == nil {
		t.Fatal("expected non-nil comp")
	}
	if comp.Role != "GlobalNav" {
		t.Errorf("expected role='GlobalNav' (structural role replaced by match name), got %q", comp.Role)
	}
}

func TestFilter_GenericWithNameIsKept(t *testing.T) {
	// generic with a name should be kept even with ≤1 non-text child.
	root := node("1", "generic", "Section Title", true, false, false)
	comp := Filter(root, nil)
	if comp == nil {
		t.Fatal("expected non-nil comp: generic with name should be kept")
	}
	if comp.Role != "generic" {
		t.Errorf("expected role=generic, got %q", comp.Role)
	}
}

func TestFilter_Value_Preserved(t *testing.T) {
	n := &sightmap.ComponentNode{
		Id: "1", Role: "textbox", Name: "Search", Value: "hello world",
		IsVisible: true, IsInteractive: true,
	}
	comp := Filter(n, nil)
	if comp == nil {
		t.Fatal("expected non-nil comp")
	}
	if comp.Value != "hello world" {
		t.Errorf("expected Value='hello world', got %q", comp.Value)
	}
}

func TestFilter_EmptyMatches(t *testing.T) {
	root := node("1", "button", "OK", true, true, false)
	comp := Filter(root, map[*sightmap.ComponentNode]*sightmap.ComponentMatch{})
	if comp == nil {
		t.Fatal("expected non-nil comp")
	}
	if comp.Role != "button" {
		t.Errorf("expected role=button, got %q", comp.Role)
	}
}

// --- Format tests ---

func TestFormat_Basic(t *testing.T) {
	root := node("1", "button", "Submit", true, true, false)
	comp := Filter(root, nil)
	var buf bytes.Buffer
	comp.Format(&buf, "", FormatOpts{})
	got := buf.String()
	want := "1 button \"Submit\"\n"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestFormat_NilComp(t *testing.T) {
	var c *Comp
	var buf bytes.Buffer
	c.Format(&buf, "", FormatOpts{}) // should not panic
	if buf.Len() != 0 {
		t.Errorf("expected empty output for nil Comp, got %q", buf.String())
	}
}

func TestFormat_WithMatchAnnotation_Interactive(t *testing.T) {
	a := node("1", "link", "Home", true, true, false)
	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		a: {Name: "NavLink"},
	}
	comp := Filter(a, matches)
	var buf bytes.Buffer
	comp.Format(&buf, "", FormatOpts{})
	got := buf.String()
	// No props → no Rule A → name kept last
	want := "1 [NavLink \"Home\"]\n"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestFormat_WithMatchAnnotation_Structural(t *testing.T) {
	nav := node("1", "navigation", "", true, false, false)
	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		nav: {Name: "GlobalNav"},
	}
	comp := Filter(nav, matches)
	var buf bytes.Buffer
	comp.Format(&buf, "", FormatOpts{})
	got := buf.String()
	want := "1 [GlobalNav]\n"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestFormat_WithPropValues(t *testing.T) {
	a := node("1", "link", "Garden Center", true, true, false)
	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		a: {Name: "CategoryTile", Properties: []sightmap.PropertyValue{{Name: "category", Value: "Garden Center"}}},
	}
	comp := Filter(a, matches)
	var buf bytes.Buffer
	comp.Format(&buf, "", FormatOpts{})
	got := buf.String()
	// Rule A: name exactly matches category prop → name suppressed inside brackets.
	want := "1 [CategoryTile category=\"Garden Center\"]\n"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestFormat_WithPropValues_SortedKeys(t *testing.T) {
	a := node("1", "link", "Item", true, true, false)
	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		a: {Name: "Tile", Properties: []sightmap.PropertyValue{{Name: "zzz", Value: "last"}, {Name: "aaa", Value: "first"}, {Name: "mmm", Value: "mid"}}},
	}
	comp := Filter(a, matches)
	var buf bytes.Buffer
	comp.Format(&buf, "", FormatOpts{})
	got := buf.String()
	// Keys sorted alphabetically; name "Item" ≠ any prop value → kept last.
	want := "1 [Tile aaa=\"first\" mmm=\"mid\" zzz=\"last\" \"Item\"]\n"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestFormat_Nested(t *testing.T) {
	nav := node("1", "navigation", "", true, false, false,
		node("2", "link", "Home", true, true, false),
		node("3", "link", "About", true, true, false),
	)
	comp := Filter(nav, nil)
	var buf bytes.Buffer
	comp.Format(&buf, "", FormatOpts{})
	got := buf.String()
	want := "1 navigation\n  2 link \"Home\"\n  3 link \"About\"\n"
	if got != want {
		t.Errorf("expected:\n%q\ngot:\n%q", want, got)
	}
}

func TestFormat_Selectors(t *testing.T) {
	n := &sightmap.ComponentNode{
		Id: "1", Role: "button", Name: "Add", IsVisible: true, IsInteractive: true,
		Element: &sightmap.Element{
			Tag:   "button",
			Attrs: map[string]string{"data-testid": "add-btn"},
		},
	}
	comp := Filter(n, nil)
	var buf bytes.Buffer
	comp.Format(&buf, "", FormatOpts{Selectors: true})
	got := buf.String()
	if !strings.Contains(got, `<button`) {
		t.Errorf("expected selector hint with 'button', got %q", got)
	}
	if !strings.Contains(got, `data-testid`) {
		t.Errorf("expected selector hint with 'data-testid', got %q", got)
	}
	if !strings.Contains(got, `add-btn`) {
		t.Errorf("expected selector hint with 'add-btn', got %q", got)
	}
}

func TestFormat_Selectors_NoClasses(t *testing.T) {
	n := &sightmap.ComponentNode{
		Id: "1", Role: "button", Name: "Add", IsVisible: true, IsInteractive: true,
		Element: &sightmap.Element{
			Tag:     "button",
			Classes: []string{"btn", "btn-primary"},
			Id:      "submit-btn",
		},
	}
	comp := Filter(n, nil)
	var buf bytes.Buffer
	comp.Format(&buf, "", FormatOpts{Selectors: true})
	got := buf.String()
	if strings.Contains(got, "btn-primary") || strings.Contains(got, "btn ") {
		t.Errorf("selector hint should NOT include CSS classes, got %q", got)
	}
	if !strings.Contains(got, "#submit-btn") {
		t.Errorf("expected #id in selector hint, got %q", got)
	}
}

func TestFormat_Selectors_DataComponent(t *testing.T) {
	n := &sightmap.ComponentNode{
		Id: "1", Role: "generic", Name: "Canvas", IsVisible: true, IsInteractive: false,
		Element: &sightmap.Element{
			Tag:   "div",
			Attrs: map[string]string{"data-component": "LayoutCanvas"},
		},
	}
	// generic with a name is kept
	comp := Filter(n, nil)
	if comp == nil {
		t.Fatal("expected non-nil comp")
	}
	var buf bytes.Buffer
	comp.Format(&buf, "", FormatOpts{Selectors: true})
	got := buf.String()
	if !strings.Contains(got, "data-component") {
		t.Errorf("expected data-component in selector hint, got %q", got)
	}
	if !strings.Contains(got, "LayoutCanvas") {
		t.Errorf("expected 'LayoutCanvas' in selector hint, got %q", got)
	}
}

func TestFormat_MaxDepth(t *testing.T) {
	// Use a generic wrapper (transparent: 1 non-text child) so the link
	// ends up directly under navigation at depth 1 after filtering.
	nav := node("1", "navigation", "", true, false, false,
		node("2", "generic", "", true, false, false,
			node("3", "link", "Home", true, true, false),
		),
	)
	comp := Filter(nav, nil)
	var buf bytes.Buffer
	// MaxDepth=1: navigation at depth 0, link promoted to depth 1 (generic is transparent)
	comp.Format(&buf, "", FormatOpts{MaxDepth: 1})
	got := buf.String()
	// navigation should appear at depth 0
	if !strings.Contains(got, "navigation") {
		t.Errorf("expected 'navigation' in output, got %q", got)
	}
	// link is at depth 1 (generic was transparent), so it should appear
	if !strings.Contains(got, "link") {
		t.Errorf("expected 'link' in output at depth 1, got %q", got)
	}
}

func TestFormat_Interactive_SkipsNonInteractive(t *testing.T) {
	nav := node("1", "navigation", "", true, false, false,
		node("2", "link", "Home", true, true, false),
		node("3", "link", "About", true, true, false),
	)
	// Add a non-interactive sibling paragraph
	para := node("4", "paragraph", "Some text", true, false, false)
	root := node("0", "region", "", true, false, false, nav, para)
	comp := Filter(root, nil)
	var buf bytes.Buffer
	comp.Format(&buf, "", FormatOpts{Interactive: true})
	got := buf.String()
	// Interactive links should appear
	if !strings.Contains(got, "link") {
		t.Errorf("expected links in interactive output, got %q", got)
	}
	// Non-interactive paragraph should not appear
	if strings.Contains(got, "paragraph") {
		t.Errorf("expected paragraph to be skipped in interactive mode, got %q", got)
	}
}

func TestFormat_ValueInOutput(t *testing.T) {
	n := &sightmap.ComponentNode{
		Id: "1", Role: "textbox", Name: "Search", Value: "hello",
		IsVisible: true, IsInteractive: true,
	}
	comp := Filter(n, nil)
	var buf bytes.Buffer
	comp.Format(&buf, "", FormatOpts{})
	got := buf.String()
	if !strings.Contains(got, `value="hello"`) {
		t.Errorf("expected value annotation in output, got %q", got)
	}
}

func TestFormat_Indent(t *testing.T) {
	root := node("1", "button", "OK", true, true, false)
	comp := Filter(root, nil)
	var buf bytes.Buffer
	comp.Format(&buf, "    ", FormatOpts{})
	got := buf.String()
	if !strings.HasPrefix(got, "    1 button") {
		t.Errorf("expected output to start with '    1 button', got %q", got)
	}
}

// --- redundant accessible-name suppression ---

func TestFilter_SoleChildNameSuppressesParentName(t *testing.T) {
	// link "Foo" → [heading "Foo"] should produce link with no name;
	// the heading child already shows the text.
	heading := node("2", "heading", "Appliances", true, false, false)
	link := node("1", "link", "Appliances", true, true, false, heading)
	comp := Filter(link, nil)
	if comp == nil {
		t.Fatal("expected non-nil comp")
	}
	if comp.Name != "" {
		t.Errorf("expected parent Name to be suppressed, got %q", comp.Name)
	}
	if len(comp.Children) != 1 || comp.Children[0].Name != "Appliances" {
		t.Errorf("expected heading child with Name='Appliances', got %v", comp.Children)
	}
}

func TestFilter_SoleChildNameDiffers_ParentNameKept(t *testing.T) {
	// Parent and sole child have different names → parent name kept.
	heading := node("2", "heading", "Appliances Dept", true, false, false)
	link := node("1", "link", "Appliances", true, true, false, heading)
	comp := Filter(link, nil)
	if comp == nil {
		t.Fatal("expected non-nil comp")
	}
	if comp.Name != "Appliances" {
		t.Errorf("expected parent Name 'Appliances' kept, got %q", comp.Name)
	}
}

func TestFilter_MultipleChildren_ParentNameKept(t *testing.T) {
	// Multiple children: sole-child rule doesn't fire, parent name kept.
	img := node("2", "img", "Appliances", true, false, false)
	heading := node("3", "heading", "Appliances", true, false, false)
	link := node("1", "link", "Appliances", true, true, false, img, heading)
	comp := Filter(link, nil)
	if comp == nil {
		t.Fatal("expected non-nil comp")
	}
	if comp.Name != "Appliances" {
		t.Errorf("expected parent Name kept with multiple children, got %q", comp.Name)
	}
}

// --- New tests for the bracket format and ID prefix ---

func TestFormat_Bracket_NoPropsNoName(t *testing.T) {
	// Matched structural node with no text and no props → bare [CompName]
	nav := node("42", "navigation", "", true, false, false)
	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		nav: {Name: "SiteHeader"},
	}
	comp := Filter(nav, matches)
	var buf bytes.Buffer
	comp.Format(&buf, "", FormatOpts{})
	want := "42 [SiteHeader]\n"
	if got := buf.String(); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestFormat_Bracket_NameKept_NoProps(t *testing.T) {
	// Matched node, name present, no props → name shown last inside brackets
	a := node("7", "link", "Shop Now", true, true, false)
	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		a: {Name: "BannerLink"},
	}
	comp := Filter(a, matches)
	var buf bytes.Buffer
	comp.Format(&buf, "", FormatOpts{})
	want := "7 [BannerLink \"Shop Now\"]\n"
	if got := buf.String(); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestFormat_RuleA_NameSuppressed(t *testing.T) {
	// Rule A: name exactly matches a prop value → name omitted
	a := node("3", "link", "Explore", true, true, false)
	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		a: {Name: "DesktopNavLink", Properties: []sightmap.PropertyValue{{Name: "label", Value: "Explore"}}},
	}
	comp := Filter(a, matches)
	var buf bytes.Buffer
	comp.Format(&buf, "", FormatOpts{})
	want := "3 [DesktopNavLink label=\"Explore\"]\n"
	if got := buf.String(); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestFormat_RuleA_NameKept_CaseDiffers(t *testing.T) {
	// Rule A is exact — case difference means name is kept last
	a := node("5", "link", "FRI Aug 21 7:00pm", true, true, false)
	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		a: {Name: "ShowDateRow", Properties: []sightmap.PropertyValue{{Name: "date", Value: "Fri Aug 21 7:00pm"}}},
	}
	comp := Filter(a, matches)
	var buf bytes.Buffer
	comp.Format(&buf, "", FormatOpts{})
	// "FRI Aug..." ≠ "Fri Aug..." → name kept last
	want := "5 [ShowDateRow date=\"Fri Aug 21 7:00pm\" \"FRI Aug 21 7:00pm\"]\n"
	if got := buf.String(); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestFormat_RuleB_StaticTextSuppressed(t *testing.T) {
	// Rule B: StaticText child whose name exactly matches a prop value is suppressed.
	// The parent has a non-StaticText child too, so the StaticText survived Filter().
	st := node("2", "StaticText", "Explore", true, false, false)
	btn := node("3", "button", "Open menu", true, true, false)
	parent := node("1", "generic", "foo bar", true, false, false, st, btn)
	// generic with name "foo bar" is kept (name != "")
	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		parent: {Name: "NavItem", Properties: []sightmap.PropertyValue{{Name: "label", Value: "Explore"}}},
	}
	comp := Filter(parent, matches)
	if comp == nil {
		t.Fatal("expected non-nil comp")
	}
	var buf bytes.Buffer
	comp.Format(&buf, "", FormatOpts{})
	got := buf.String()
	// StaticText "Explore" == prop "Explore" → suppressed
	if strings.Contains(got, "StaticText") {
		t.Errorf("expected StaticText 'Explore' to be suppressed, got:\n%s", got)
	}
	// button child is kept
	if !strings.Contains(got, "button") {
		t.Errorf("expected button child to be present, got:\n%s", got)
	}
}

func TestFormat_RuleB_StaticTextKept_NoMatch(t *testing.T) {
	// Rule B: StaticText child whose name does NOT match any prop value is kept.
	st := node("2", "StaticText", "other text", true, false, false)
	btn := node("3", "button", "OK", true, true, false)
	parent := node("1", "generic", "foo bar", true, false, false, st, btn)
	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		parent: {Name: "MyComp", Properties: []sightmap.PropertyValue{{Name: "label", Value: "Explore"}}},
	}
	comp := Filter(parent, matches)
	var buf bytes.Buffer
	comp.Format(&buf, "", FormatOpts{})
	got := buf.String()
	// "other text" ≠ "Explore" → StaticText kept
	if !strings.Contains(got, "StaticText") {
		t.Errorf("expected StaticText 'other text' to be kept, got:\n%s", got)
	}
}

func TestFormat_ValueFallback_InBrackets(t *testing.T) {
	// AX Comp.Value appears as value="..." inside brackets for matched nodes
	// when no sightmap "value" property is declared.
	n := &sightmap.ComponentNode{
		Id: "7", Role: "textbox", Name: "Search", Value: "query text",
		IsVisible: true, IsInteractive: true,
	}
	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		n: {Name: "SearchInput"},
	}
	comp := Filter(n, matches)
	var buf bytes.Buffer
	comp.Format(&buf, "", FormatOpts{})
	got := buf.String()
	// No PropValues → AX value is the fallback; Rule A: "Search" ≠ "query text" → name kept
	want := "7 [SearchInput value=\"query text\" \"Search\"]\n"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestFormat_ValueOverride_SightmapWins(t *testing.T) {
	// Sightmap-extracted "value" prop overrides AX Comp.Value.
	n := &sightmap.ComponentNode{
		Id: "8", Role: "textbox", Name: "Dropdown", Value: "AX value",
		IsVisible: true, IsInteractive: true,
	}
	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		n: {Name: "CustomSelect", Properties: []sightmap.PropertyValue{{Name: "value", Value: "sightmap value"}}},
	}
	comp := Filter(n, matches)
	var buf bytes.Buffer
	comp.Format(&buf, "", FormatOpts{})
	got := buf.String()
	// Sightmap "value" wins; AX "AX value" is ignored; name "Dropdown" ≠ "sightmap value" → kept
	want := "8 [CustomSelect value=\"sightmap value\" \"Dropdown\"]\n"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestFormat_UnmatchedNode_ValueAsSuffix(t *testing.T) {
	// Unmatched nodes: AX value is a suffix, NOT inside brackets.
	n := &sightmap.ComponentNode{
		Id: "9", Role: "textbox", Name: "Search", Value: "hello",
		IsVisible: true, IsInteractive: true,
	}
	comp := Filter(n, nil)
	var buf bytes.Buffer
	comp.Format(&buf, "", FormatOpts{})
	want := "9 textbox \"Search\" value=\"hello\"\n"
	if got := buf.String(); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestFormat_EmptyId_NoPrefix(t *testing.T) {
	// Comp with empty Id produces no leading space.
	n := &sightmap.ComponentNode{
		Id: "", Role: "button", Name: "OK",
		IsVisible: true, IsInteractive: true,
	}
	comp := Filter(n, nil)
	var buf bytes.Buffer
	comp.Format(&buf, "", FormatOpts{})
	got := buf.String()
	if strings.HasPrefix(got, " ") {
		t.Errorf("expected no leading space for empty Id, got %q", got)
	}
	if !strings.Contains(got, "button") {
		t.Errorf("expected role in output, got %q", got)
	}
}

func TestFormat_Selectors_AfterBracket(t *testing.T) {
	// --selectors hint appears AFTER the closing bracket for matched nodes.
	n := &sightmap.ComponentNode{
		Id: "5", Role: "link", Name: "Home", IsVisible: true, IsInteractive: true,
		Element: &sightmap.Element{
			Tag:   "a",
			Attrs: map[string]string{"data-testid": "home-link"},
		},
	}
	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		n: {Name: "HomeLink"},
	}
	comp := Filter(n, matches)
	var buf bytes.Buffer
	comp.Format(&buf, "", FormatOpts{Selectors: true})
	got := buf.String()
	// Bracket closes before selector hint
	bracketEnd := strings.Index(got, "]")
	selectorStart := strings.Index(got, "<a")
	if bracketEnd == -1 || selectorStart == -1 {
		t.Fatalf("expected both ] and <a in output, got %q", got)
	}
	if bracketEnd > selectorStart {
		t.Errorf("expected selector hint to appear AFTER ], got %q", got)
	}
}

// --- selectorHint tests ---

func TestSelectorHint_Empty(t *testing.T) {
	got := selectorHint(nil)
	if got != "" {
		t.Errorf("expected empty string for nil selector, got %q", got)
	}
}

func TestSelectorHint_TagOnly(t *testing.T) {
	sel := &sightmap.Element{Tag: "div"}
	got := selectorHint(sel)
	if got != "div" {
		t.Errorf("expected 'div', got %q", got)
	}
}

func TestSelectorHint_TagAndId(t *testing.T) {
	sel := &sightmap.Element{Tag: "nav", Id: "main-nav"}
	got := selectorHint(sel)
	if got != "nav#main-nav" {
		t.Errorf("expected 'nav#main-nav', got %q", got)
	}
}

func TestSelectorHint_NoClasses(t *testing.T) {
	sel := &sightmap.Element{
		Tag:     "button",
		Classes: []string{"foo", "bar"},
	}
	got := selectorHint(sel)
	if strings.Contains(got, "foo") || strings.Contains(got, "bar") {
		t.Errorf("selector hint should not include classes, got %q", got)
	}
}

// --- truncate and quoteName tests ---

func TestTruncate_Short(t *testing.T) {
	got := truncate("hello", 10)
	if got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestTruncate_Exact(t *testing.T) {
	got := truncate("hello", 5)
	if got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestTruncate_Long(t *testing.T) {
	got := truncate("hello world", 5)
	if !strings.HasPrefix(got, "hello") {
		t.Errorf("expected truncated string to start with 'hello', got %q", got)
	}
	if !strings.Contains(got, "…") {
		t.Errorf("expected truncation ellipsis, got %q", got)
	}
}

func TestTruncate_Unicode(t *testing.T) {
	// 3 multibyte runes
	got := truncate("héllo", 3)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected truncation ellipsis for unicode string, got %q", got)
	}
}

func TestQuoteName_Basic(t *testing.T) {
	got := quoteName("Submit")
	if got != `"Submit"` {
		t.Errorf(`expected '"Submit"', got %q`, got)
	}
}

func TestQuoteName_EscapesQuotes(t *testing.T) {
	got := quoteName(`Say "hello"`)
	if !strings.Contains(got, `\"hello\"`) {
		t.Errorf("expected escaped quotes, got %q", got)
	}
}

func TestQuoteName_EscapesBackslash(t *testing.T) {
	got := quoteName(`foo\bar`)
	if !strings.Contains(got, `\\`) {
		t.Errorf("expected escaped backslash, got %q", got)
	}
}
