package observe

import (
	"bytes"
	"testing"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// TestWriteT2Trace_Basic verifies basic T2 enumeration functionality.
func TestWriteT2Trace_Basic(t *testing.T) {
	// Create a simple tree with T2 children
	parent := &comps.ComponentNode{
		Id:   "parent",
		Role: "complementary",
	}
	children := []*comps.ComponentNode{
		{Id: "c1", Role: "button", Name: "Create", IsInteractive: true},
		{Id: "c2", Role: "button", Name: "Delete", IsInteractive: true},
		{Id: "c3", Role: "link", Name: "Settings", IsInteractive: true},
	}

	t2clusters := map[*comps.ComponentNode]int{
		parent: 3,
	}
	t2children := map[*comps.ComponentNode][]*comps.ComponentNode{
		parent: children,
	}
	matches := map[*comps.ComponentNode]*match.SightmapMatch{
		parent: {Name: "AppSidebar"},
	}
	view := &sightmap.View{
		Name: "Home",
		Components: []match.SightmapComponent{
			{Name: "AppSidebar"},
		},
	}

	var buf bytes.Buffer
	WriteT2Trace(&buf, t2clusters, t2children, matches, view)

	output := buf.String()
	if output == "" {
		t.Fatal("Expected output, got empty string")
	}

	// Check for expected content
	mustContain := []string{
		"[AppSidebar] (3)",
		"button \"Create\"",
		"button \"Delete\"",
		"link \"Settings\"",
	}
	for _, substr := range mustContain {
		if !contains(output, substr) {
			t.Errorf("Output missing expected substring: %q\nOutput:\n%s", substr, output)
		}
	}
}

// TestWriteT2Trace_SingleChildSuppressed verifies single-child T2 is suppressed.
func TestWriteT2Trace_SingleChildSuppressed(t *testing.T) {
	parent := &comps.ComponentNode{
		Id:   "parent",
		Role: "navigation",
	}
	child := &comps.ComponentNode{
		Id: "c1", Role: "link", Name: "Privacy", IsInteractive: true,
	}

	t2clusters := map[*comps.ComponentNode]int{
		parent: 1, // single child
	}
	t2children := map[*comps.ComponentNode][]*comps.ComponentNode{
		parent: {child},
	}
	matches := map[*comps.ComponentNode]*match.SightmapMatch{
		parent: {Name: "NavFooter"},
	}
	view := &sightmap.View{
		Components: []match.SightmapComponent{
			{Name: "NavFooter"},
		},
	}

	var buf bytes.Buffer
	WriteT2Trace(&buf, t2clusters, t2children, matches, view)

	output := buf.String()
	// Single-child T2 should be suppressed (minT2Count = 2)
	if contains(output, "[NavFooter]") {
		t.Errorf("Expected NavFooter to be suppressed (single child), but it appears in output:\n%s", output)
	}
}

// TestWriteT2Trace_MemoryNoteSuppressed verifies memory note suppression.
func TestWriteT2Trace_MemoryNoteSuppressed(t *testing.T) {
	parent := &comps.ComponentNode{
		Id:   "parent",
		Role: "main",
	}
	children := []*comps.ComponentNode{
		{Id: "c1", Role: "button", Name: "Action 1", IsInteractive: true},
		{Id: "c2", Role: "button", Name: "Action 2", IsInteractive: true},
		{Id: "c3", Role: "button", Name: "Action 3", IsInteractive: true},
	}

	t2clusters := map[*comps.ComponentNode]int{
		parent: 3,
	}
	t2children := map[*comps.ComponentNode][]*comps.ComponentNode{
		parent: children,
	}
	matches := map[*comps.ComponentNode]*match.SightmapMatch{
		parent: {Name: "AppContent"},
	}
	view := &sightmap.View{
		Components: []match.SightmapComponent{
			{
				Name:   "AppContent",
				Memory: []string{"Investigated. All children use volatile classes."},
			},
		},
	}

	var buf bytes.Buffer
	WriteT2Trace(&buf, t2clusters, t2children, matches, view)

	output := buf.String()

	// Should show component with count
	if !contains(output, "[AppContent] (3)") {
		t.Errorf("Expected [AppContent] (3), got:\n%s", output)
	}
	// Should show memory note
	if !contains(output, "exhausted, see component memory note") {
		t.Errorf("Expected memory note message, got:\n%s", output)
	}
	// Should NOT enumerate children
	if contains(output, "button \"Action 1\"") {
		t.Errorf("Expected children to be suppressed due to memory note, but found enumeration:\n%s", output)
	}
}

// TestWriteT2Trace_Truncation verifies large lists are truncated.
func TestWriteT2Trace_Truncation(t *testing.T) {
	parent := &comps.ComponentNode{
		Id:   "parent",
		Role: "region",
	}
	// Create 10 children
	var children []*comps.ComponentNode
	for i := 1; i <= 10; i++ {
		children = append(children, &comps.ComponentNode{
			Id:            string(rune('0' + i)),
			Role:          "link",
			Name:          "Product " + string(rune('0'+i)),
			IsInteractive: true,
		})
	}

	t2clusters := map[*comps.ComponentNode]int{
		parent: 10,
	}
	t2children := map[*comps.ComponentNode][]*comps.ComponentNode{
		parent: children,
	}
	matches := map[*comps.ComponentNode]*match.SightmapMatch{
		parent: {Name: "ProductGrid"},
	}
	view := &sightmap.View{
		Components: []match.SightmapComponent{
			{Name: "ProductGrid"},
		},
	}

	var buf bytes.Buffer
	WriteT2Trace(&buf, t2clusters, t2children, matches, view)

	output := buf.String()

	// Should show count
	if !contains(output, "[ProductGrid] (10)") {
		t.Errorf("Expected [ProductGrid] (10), got:\n%s", output)
	}
	// Should show truncation message (truncateAt = 8)
	if !contains(output, "… (2 more)") {
		t.Errorf("Expected truncation message, got:\n%s", output)
	}
	// Should show first 8
	if !contains(output, "Product 1") || !contains(output, "Product 8") {
		t.Errorf("Expected first 8 products, got:\n%s", output)
	}
	// Should NOT show 9th and 10th
	if contains(output, "Product 9") || contains(output, "Product 10") {
		t.Errorf("Expected products 9-10 to be truncated, but found them:\n%s", output)
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
