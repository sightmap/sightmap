package render

import (
	"bytes"
	"github.com/sightmap/sightmap/go/sightmap"
	"strings"
	"testing"
)

// TestInspect_PreservesAllNodes verifies that invisible and ignored children
// both appear in the InspectNode tree (unlike Filter, which drops them).
func TestInspect_PreservesAllNodes(t *testing.T) {
	invisible := node("2", "none", "", false /*invisible*/, false, false)
	ignored := node("3", "generic", "", true, false, true /*ignored*/)
	root := node("1", "document", "", true, false, false, invisible, ignored)

	in := Inspect(root, nil)
	if len(in.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(in.Children))
	}
}

// TestInspect_NoFilteringVsFilter confirms Inspect keeps the "none" wrapper
// that Filter() would make transparent.
func TestInspect_NoFilteringVsFilter(t *testing.T) {
	none := node("2", "none", "", true, false, false,
		node("3", "button", "OK", true, true, false))
	root := node("1", "document", "", true, false, false, none)

	// Filter: root -> button (none is transparent)
	comp := Filter(root, nil)
	if comp.Children[0].Role != "button" {
		t.Errorf("Filter: expected first child role=button, got %q", comp.Children[0].Role)
	}

	// Inspect: root -> none -> button
	in := Inspect(root, nil)
	if in.Children[0].Role != "none" {
		t.Errorf("Inspect: expected first child role=none, got %q", in.Children[0].Role)
	}
	if in.Children[0].Children[0].Role != "button" {
		t.Errorf("Inspect: expected grandchild role=button, got %q", in.Children[0].Children[0].Role)
	}
}

// TestInspect_MatchAnnotation verifies that match is propagated onto InspectNode.
func TestInspect_MatchAnnotation(t *testing.T) {
	btn := node("1", "button", "OK", true, true, false)
	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		btn: {Name: "SubmitBtn"},
	}

	in := Inspect(btn, matches)
	if in.Match == nil {
		t.Fatal("expected Match to be non-nil")
	}
	if in.Match.Name != "SubmitBtn" {
		t.Errorf("expected Match.Name=SubmitBtn, got %q", in.Match.Name)
	}
}

// TestInspect_NilRoot verifies Inspect(nil, nil) returns nil without panic.
func TestInspect_NilRoot(t *testing.T) {
	if Inspect(nil, nil) != nil {
		t.Error("expected nil for nil root")
	}
}

// TestFormatInspect_Basic verifies basic output with no selector.
func TestFormatInspect_Basic(t *testing.T) {
	n := node("5", "button", "OK", true, true, false)
	in := Inspect(n, nil)
	var buf bytes.Buffer
	FormatInspect(&buf, in, InspectOpts{})
	got := buf.String()

	if !strings.Contains(got, "5 button") {
		t.Errorf("expected '5 button' in output, got %q", got)
	}
	if !strings.Contains(got, `"OK"`) {
		t.Errorf(`expected '"OK"' in output, got %q`, got)
	}
}

// TestFormatInspect_WithSelector verifies tag#id[data-testid] selector display.
func TestFormatInspect_WithSelector(t *testing.T) {
	n := &sightmap.ComponentNode{
		Id: "7", Role: "button", Name: "Add", IsVisible: true, IsInteractive: true,
		Element: &sightmap.Element{
			Tag:   "button",
			Id:    "add-btn",
			Attrs: map[string]string{"data-testid": "add-to-cart"},
		},
	}
	in := Inspect(n, nil)
	var buf bytes.Buffer
	FormatInspect(&buf, in, InspectOpts{})
	got := buf.String()

	want := `7 button#add-btn[data-testid="add-to-cart"]`
	if !strings.Contains(got, want) {
		t.Errorf("expected %q in output, got %q", want, got)
	}
	if strings.Contains(got, "[hidden]") {
		t.Error("unexpected [hidden] in output")
	}
	if strings.Contains(got, "[ignored]") {
		t.Error("unexpected [ignored] in output")
	}
}

// TestFormatInspect_Hidden verifies [hidden] annotation when !IsVisible.
func TestFormatInspect_Hidden(t *testing.T) {
	n := node("1", "div", "", false /*not visible*/, false, false)
	in := Inspect(n, nil)
	var buf bytes.Buffer
	FormatInspect(&buf, in, InspectOpts{})
	if !strings.Contains(buf.String(), "[hidden]") {
		t.Errorf("expected [hidden] in output, got %q", buf.String())
	}
}

// TestFormatInspect_Ignored verifies [ignored] annotation when IsIgnored.
func TestFormatInspect_Ignored(t *testing.T) {
	n := node("1", "none", "", true, false, true /*ignored*/)
	in := Inspect(n, nil)
	var buf bytes.Buffer
	FormatInspect(&buf, in, InspectOpts{})
	if !strings.Contains(buf.String(), "[ignored]") {
		t.Errorf("expected [ignored] in output, got %q", buf.String())
	}
}

// TestFormatInspect_Classes verifies CSS classes are shown only when Classes is set.
func TestFormatInspect_Classes(t *testing.T) {
	n := &sightmap.ComponentNode{
		Id: "1", Role: "div", IsVisible: true,
		Element: &sightmap.Element{
			Tag:     "div",
			Classes: []string{"btn", "btn-primary"},
		},
	}
	in := Inspect(n, nil)

	var buf bytes.Buffer
	// Without Classes: no class output
	FormatInspect(&buf, in, InspectOpts{})
	if strings.Contains(buf.String(), "btn") {
		t.Error("classes should not appear by default")
	}

	// With Classes: classes shown
	buf.Reset()
	FormatInspect(&buf, in, InspectOpts{Classes: true})
	if !strings.Contains(buf.String(), ".btn") {
		t.Errorf("expected .btn with Classes=true, got %q", buf.String())
	}
}

// TestFormatInspect_Selectors_ShowsAll verifies --selectors shows attrs that
// are normally hidden (e.g. tabindex).
func TestFormatInspect_Selectors_ShowsAll(t *testing.T) {
	n := &sightmap.ComponentNode{
		Id: "1", Role: "input", IsVisible: true, IsInteractive: true,
		Element: &sightmap.Element{
			Tag: "input",
			Attrs: map[string]string{
				"data-testid": "search",
				"tabindex":    "0", // NOT a key attr by default
			},
		},
	}
	in := Inspect(n, nil)

	var buf bytes.Buffer
	// Default: tabindex not shown
	FormatInspect(&buf, in, InspectOpts{})
	if strings.Contains(buf.String(), "tabindex") {
		t.Error("tabindex should not appear by default")
	}

	// With Selectors: tabindex shown
	buf.Reset()
	FormatInspect(&buf, in, InspectOpts{Selectors: true})
	if !strings.Contains(buf.String(), "tabindex") {
		t.Errorf("expected tabindex with Selectors=true, got %q", buf.String())
	}
}

// TestFormatInspect_MatchAnnotation verifies ★Name is shown for matched nodes.
func TestFormatInspect_MatchAnnotation(t *testing.T) {
	btn := node("3", "button", "Shop", true, true, false)
	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		btn: {Name: "ShopButton"},
	}
	in := Inspect(btn, matches)
	var buf bytes.Buffer
	FormatInspect(&buf, in, InspectOpts{})
	if !strings.Contains(buf.String(), "★ShopButton") {
		t.Errorf("expected ★ShopButton annotation, got %q", buf.String())
	}
}

// TestFormatInspect_Nested_IDPrefix verifies child nodes have their own ID prefix.
func TestFormatInspect_Nested_IDPrefix(t *testing.T) {
	child := node("2", "button", "OK", true, true, false)
	root := node("1", "nav", "", true, false, false, child)
	in := Inspect(root, nil)
	var buf bytes.Buffer
	FormatInspect(&buf, in, InspectOpts{})
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d: %q", len(lines), buf.String())
	}
	if !strings.HasPrefix(lines[0], "1 ") {
		t.Errorf("line[0] should start with '1 ', got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "  2 ") {
		t.Errorf("line[1] should start with '  2 ', got %q", lines[1])
	}
}

// TestFormatInspect_MaxDepth verifies that depth limit cuts the tree.
func TestFormatInspect_MaxDepth(t *testing.T) {
	grandchild := node("3", "span", "x", true, false, false)
	child := node("2", "div", "", true, false, false, grandchild)
	root := node("1", "nav", "", true, false, false, child)
	in := Inspect(root, nil)
	var buf bytes.Buffer
	FormatInspect(&buf, in, InspectOpts{MaxDepth: 1})
	// depth 0: nav, depth 1: div — span at depth 2 is cut
	if strings.Contains(buf.String(), "span") {
		t.Errorf("expected span to be cut at MaxDepth=1, got %q", buf.String())
	}
}
