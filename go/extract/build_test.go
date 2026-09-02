package extract

import (
	"testing"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// probe1 builds a minimal ProbeResult with a single component.
func singleProbe(id, tagName, selector string) *ProbeResult {
	return &ProbeResult{
		Success: true,
		Results: []ProbeComponent{
			{Id: id, TagName: tagName, Selector: selector},
		},
	}
}

// domChild builds a DOMNode that looks like a child element with a sightmap ID.
func domChild(nodeID, backendNodeID int, nodeName, sightmapID string, children ...*DOMNode) *DOMNode {
	return &DOMNode{
		NodeID:        nodeID,
		BackendNodeID: backendNodeID,
		NodeName:      nodeName,
		Attributes:    []string{"data-sightmap-id", sightmapID},
		Children:      children,
	}
}

// domWrapper is a node without a sightmap ID (e.g. #document).
func domWrapper(nodeName string, children ...*DOMNode) *DOMNode {
	return &DOMNode{NodeName: nodeName, Children: children}
}

// a11yNode builds a minimal A11YNode.
func a11yNode(backendDOMNodeID int, role, name string) A11YNode {
	return A11YNode{
		BackendDOMNodeID: backendDOMNodeID,
		Role:             &A11YValue{Value: role},
		Name:             &A11YValue{Value: name},
	}
}

// ---------------------------------------------------------------------------
// Test 1 – Minimal tree
// ---------------------------------------------------------------------------

func TestMinimalTree(t *testing.T) {
	probe := singleProbe("1", "BUTTON", "button")
	a11y := []A11YNode{a11yNode(10, "button", "Submit")}
	domRoot := domWrapper("#document",
		domChild(1, 10, "BUTTON", "1"),
	)

	root, err := BuildTree(probe, a11y, domRoot)
	if err != nil {
		t.Fatalf("BuildTree error: %v", err)
	}
	if root.Role != "button" {
		t.Errorf("Role = %q, want %q", root.Role, "button")
	}
	if root.Name != "Submit" {
		t.Errorf("Name = %q, want %q", root.Name, "Submit")
	}
	if root.Element == nil {
		t.Fatal("Selector is nil")
	}
	if root.Element.Tag != "button" {
		t.Errorf("Selector.Tag = %q, want %q", root.Element.Tag, "button")
	}
}

// ---------------------------------------------------------------------------
// Test 2 – A11Y miss: DOM node has sightmap ID but no matching A11Y node
// ---------------------------------------------------------------------------

func TestA11YMiss(t *testing.T) {
	probe := singleProbe("1", "DIV", "div")
	a11y := []A11YNode{} // empty — no match for BackendNodeID 99
	domRoot := domWrapper("#document",
		// BackendNodeID=99 not present in a11y
		&DOMNode{
			NodeID:        1,
			BackendNodeID: 99,
			NodeName:      "DIV",
			Attributes:    []string{"data-sightmap-id", "1"},
		},
	)

	root, err := BuildTree(probe, a11y, domRoot)
	if err != nil {
		t.Fatalf("BuildTree error: %v", err)
	}
	if root.Role != "none" {
		t.Errorf("Role = %q, want %q", root.Role, "none")
	}
	if !root.IsIgnored {
		t.Error("IsIgnored = false, want true")
	}
}

// ---------------------------------------------------------------------------
// Test 3 – Text node: NodeName="#text", TagName="#text", no A11Y node
// ---------------------------------------------------------------------------

func TestTextNode(t *testing.T) {
	probe := singleProbe("1", "#text", "")
	a11y := []A11YNode{} // text nodes typically have no A11Y entry
	domRoot := domWrapper("#document",
		&DOMNode{
			NodeID:        1,
			BackendNodeID: 10,
			NodeName:      "#text",
			Attributes:    []string{"data-sightmap-id", "1"},
		},
	)

	root, err := BuildTree(probe, a11y, domRoot)
	if err != nil {
		t.Fatalf("BuildTree error: %v", err)
	}
	if root.Role != "StaticText" {
		t.Errorf("Role = %q, want %q", root.Role, "StaticText")
	}
}

// ---------------------------------------------------------------------------
// Text capture + normalization
// ---------------------------------------------------------------------------

// A role-less node has no accessible name, but its captured (rendered) text is
// carried onto the ComponentNode and normalized to a single clean shape.
func TestTextCapturedAndNormalized(t *testing.T) {
	probe := &ProbeResult{
		Success: true,
		Results: []ProbeComponent{
			{Id: "1", TagName: "SPAN", Selector: "span", Text: "  B6\n   123  "},
		},
	}
	a11y := []A11YNode{} // role-less: no accessible name
	domRoot := domWrapper("#document",
		domChild(1, 10, "SPAN", "1"),
	)
	root, err := BuildTree(probe, a11y, domRoot)
	if err != nil {
		t.Fatalf("BuildTree error: %v", err)
	}
	if root.Name != "" {
		t.Errorf("Name = %q, want empty (role-less)", root.Name)
	}
	if root.Text != "B6 123" {
		t.Errorf("Text = %q, want %q (whitespace collapsed + trimmed)", root.Text, "B6 123")
	}
}

func TestNormalizeText(t *testing.T) {
	cases := map[string]string{
		"":              "",
		"  hi  ":        "hi",
		"a\n\t b   c":   "a b c",
		"Nonstop":       "Nonstop",
		"\n  B6 123 \n": "B6 123",
	}
	for in, want := range cases {
		if got := normalizeText(in); got != want {
			t.Errorf("normalizeText(%q) = %q, want %q", in, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 4 – Nested tree: parent > child1 > grandchild
// ---------------------------------------------------------------------------

func TestNestedTree(t *testing.T) {
	probe := &ProbeResult{
		Success: true,
		Results: []ProbeComponent{
			{Id: "1", TagName: "DIV", Selector: "div"},
			{Id: "2", TagName: "SPAN", Selector: "span"},
			{Id: "3", TagName: "P", Selector: "p"},
		},
	}
	a11y := []A11YNode{
		a11yNode(10, "group", ""),
		a11yNode(20, "group", ""),
		a11yNode(30, "paragraph", ""),
	}
	domRoot := domWrapper("#document",
		domChild(1, 10, "DIV", "1",
			domChild(2, 20, "SPAN", "2",
				domChild(3, 30, "P", "3"),
			),
		),
	)

	root, err := BuildTree(probe, a11y, domRoot)
	if err != nil {
		t.Fatalf("BuildTree error: %v", err)
	}

	// Root is the div.
	if root.Id != "1" {
		t.Errorf("root.Id = %q, want %q", root.Id, "1")
	}

	// Child1 is span with NthChild=1.
	if len(root.Children) != 1 {
		t.Fatalf("root.Children len = %d, want 1", len(root.Children))
	}
	child1 := root.Children[0]
	if child1.Id != "2" {
		t.Errorf("child1.Id = %q, want %q", child1.Id, "2")
	}
	if child1.NthChild != 1 {
		t.Errorf("child1.NthChild = %d, want 1", child1.NthChild)
	}

	// Grandchild with NthChild=1.
	if len(child1.Children) != 1 {
		t.Fatalf("child1.Children len = %d, want 1", len(child1.Children))
	}
	gc := child1.Children[0]
	if gc.Id != "3" {
		t.Errorf("grandchild.Id = %q, want %q", gc.Id, "3")
	}
	if gc.NthChild != 1 {
		t.Errorf("grandchild.NthChild = %d, want 1", gc.NthChild)
	}
}

// ---------------------------------------------------------------------------
// Test 5 – Shadow DOM: DOMNode has ShadowRoots
// ---------------------------------------------------------------------------

func TestShadowDOM(t *testing.T) {
	probe := &ProbeResult{
		Success: true,
		Results: []ProbeComponent{
			{Id: "1", TagName: "DIV", Selector: "div"},
			{Id: "2", TagName: "SPAN", Selector: "span"},
		},
	}
	a11y := []A11YNode{
		a11yNode(10, "group", ""),
		a11yNode(20, "group", ""),
	}
	shadowChild := &DOMNode{
		NodeID:        2,
		BackendNodeID: 20,
		NodeName:      "SPAN",
		Attributes:    []string{"data-sightmap-id", "2"},
	}
	domRoot := domWrapper("#document",
		&DOMNode{
			NodeID:        1,
			BackendNodeID: 10,
			NodeName:      "DIV",
			Attributes:    []string{"data-sightmap-id", "1"},
			ShadowRoots:   []*DOMNode{shadowChild},
		},
	)

	root, err := BuildTree(probe, a11y, domRoot)
	if err != nil {
		t.Fatalf("BuildTree error: %v", err)
	}
	if root.Id != "1" {
		t.Errorf("root.Id = %q, want %q", root.Id, "1")
	}
	if len(root.Children) != 1 {
		t.Fatalf("root.Children len = %d, want 1", len(root.Children))
	}
	sc := root.Children[0]
	if sc.Id != "2" {
		t.Errorf("shadow child Id = %q, want %q", sc.Id, "2")
	}
}

// ---------------------------------------------------------------------------
// Test 6 – Selector parse: "button.primary" → SelectorPart{Tag:"button", Classes:["primary"]}
// ---------------------------------------------------------------------------

func TestSelectorParse(t *testing.T) {
	probe := singleProbe("1", "BUTTON", "button.primary")
	a11y := []A11YNode{a11yNode(10, "button", "Click me")}
	domRoot := domWrapper("#document",
		domChild(1, 10, "BUTTON", "1"),
	)

	root, err := BuildTree(probe, a11y, domRoot)
	if err != nil {
		t.Fatalf("BuildTree error: %v", err)
	}
	if root.Element == nil {
		t.Fatal("Selector is nil")
	}
	if root.Element.Tag != "button" {
		t.Errorf("Selector.Tag = %q, want %q", root.Element.Tag, "button")
	}
	if len(root.Element.Classes) != 1 || root.Element.Classes[0] != "primary" {
		t.Errorf("Selector.Classes = %v, want [primary]", root.Element.Classes)
	}
}

// ---------------------------------------------------------------------------
// Test 7 – Selector fallback: invalid selector string → fallback uses TagName
// ---------------------------------------------------------------------------

func TestSelectorFallback(t *testing.T) {
	probe := singleProbe("1", "SPAN", "!!!invalid!!!")
	a11y := []A11YNode{a11yNode(10, "generic", "")}
	domRoot := domWrapper("#document",
		domChild(1, 10, "SPAN", "1"),
	)

	root, err := BuildTree(probe, a11y, domRoot)
	if err != nil {
		t.Fatalf("BuildTree error: %v", err)
	}
	if root.Element == nil {
		t.Fatal("Selector is nil")
	}
	if root.Element.Tag != "span" {
		t.Errorf("Selector.Tag = %q, want %q", root.Element.Tag, "span")
	}
	// No classes should be set from the invalid selector.
	if len(root.Element.Classes) != 0 {
		t.Errorf("Selector.Classes = %v, want []", root.Element.Classes)
	}
}

// ---------------------------------------------------------------------------
// Test 8 – Class attribute duplication: classes are also in Attrs["class"]
// ---------------------------------------------------------------------------

func TestClassAttrDuplication(t *testing.T) {
	probe := singleProbe("1", "BUTTON", "button.btn.btn-primary")
	a11y := []A11YNode{a11yNode(10, "button", "Submit")}
	domRoot := domWrapper("#document",
		domChild(1, 10, "BUTTON", "1"),
	)

	root, err := BuildTree(probe, a11y, domRoot)
	if err != nil {
		t.Fatalf("BuildTree error: %v", err)
	}
	if root.Element == nil {
		t.Fatal("Selector is nil")
	}

	// Classes should be parsed
	if len(root.Element.Classes) != 2 {
		t.Errorf("Selector.Classes len = %d, want 2", len(root.Element.Classes))
	}
	if root.Element.Classes[0] != "btn" || root.Element.Classes[1] != "btn-primary" {
		t.Errorf("Selector.Classes = %v, want [btn, btn-primary]", root.Element.Classes)
	}

	// Classes should also be in Attrs["class"]
	if root.Element.Attrs == nil {
		t.Fatal("Selector.Attrs is nil")
	}
	classAttr, ok := root.Element.Attrs["class"]
	if !ok {
		t.Fatal("Selector.Attrs[\"class\"] not found")
	}
	if classAttr != "btn btn-primary" {
		t.Errorf("Selector.Attrs[\"class\"] = %q, want %q", classAttr, "btn btn-primary")
	}
}
