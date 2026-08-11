package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/sightmap"
)

// treeNode extracts the "tree" object from the annotated JSON output.
func treeNode(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	tree, ok := got["tree"].(map[string]any)
	if !ok {
		t.Fatalf("expected top-level 'tree' object, got: %v", got)
	}
	return tree
}

func TestWriteAnnotatedJSON_MatchedNode(t *testing.T) {
	node := &comps.ComponentNode{
		Id:            "1",
		Role:          "link",
		Name:          "The Home Depot",
		IsVisible:     true,
		IsInteractive: true,
		InViewport:    true,
	}

	sm := &sightmap.ComponentMatch{
		Name:   "SiteLogoLink",
		Memory: []string{"main navigation logo"},
	}
	matches := map[*comps.ComponentNode]*sightmap.ComponentMatch{
		node: sm,
	}
	propValues := map[string]map[string]string{
		"1": {"label": "The Home Depot"},
	}
	view := &sightmap.ViewDef{Name: "HomePage", Route: "/"}

	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")

	if err := writeAnnotatedJSON(node, path, view, matches, propValues); err != nil {
		t.Fatalf("writeAnnotatedJSON: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	// Check top-level view/route fields.
	var top map[string]any
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if top["view"] != "HomePage" {
		t.Errorf("view = %v, want HomePage", top["view"])
	}
	if top["route"] != "/" {
		t.Errorf("route = %v, want /", top["route"])
	}

	// Check tree node fields.
	got := treeNode(t, data)
	if got["id"] != "1" {
		t.Errorf("id = %v, want 1", got["id"])
	}
	if got["role"] != "link" {
		t.Errorf("role = %v, want link", got["role"])
	}
	if got["component"] != "SiteLogoLink" {
		t.Errorf("component = %v, want SiteLogoLink", got["component"])
	}
	memory, ok := got["memory"].([]any)
	if !ok || len(memory) == 0 || memory[0] != "main navigation logo" {
		t.Errorf("memory = %v, want [\"main navigation logo\"]", got["memory"])
	}
	props, ok := got["props"].(map[string]any)
	if !ok || props["label"] != "The Home Depot" {
		t.Errorf("props = %v, want {label: The Home Depot}", got["props"])
	}
}

func TestWriteAnnotatedJSON_UnmatchedNode(t *testing.T) {
	node := &comps.ComponentNode{
		Id:   "2",
		Role: "button",
		Name: "Add to Cart",
	}

	// node is NOT in the matches map
	other := &comps.ComponentNode{Id: "99"}
	matches := map[*comps.ComponentNode]*sightmap.ComponentMatch{
		other: {Name: "SomeOtherComponent"},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")

	if err := writeAnnotatedJSON(node, path, nil, matches, nil); err != nil {
		t.Fatalf("writeAnnotatedJSON: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	// nil view → no view/route at top level
	var top map[string]any
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if _, ok := top["view"]; ok {
		t.Errorf("expected no view field for nil view, got %v", top["view"])
	}

	got := treeNode(t, data)
	if got["id"] != "2" {
		t.Errorf("id = %v, want 2", got["id"])
	}
	if got["role"] != "button" {
		t.Errorf("role = %v, want button", got["role"])
	}
	if _, ok := got["component"]; ok {
		t.Errorf("expected no component field for unmatched node, got %v", got["component"])
	}
	if _, ok := got["memory"]; ok {
		t.Errorf("expected no memory field for unmatched node, got %v", got["memory"])
	}
	if _, ok := got["props"]; ok {
		t.Errorf("expected no props field for unmatched node, got %v", got["props"])
	}
}

func TestWriteAnnotatedJSON_NilMatches(t *testing.T) {
	node := &comps.ComponentNode{
		Id:   "3",
		Role: "heading",
		Name: "Welcome",
		Children: []*comps.ComponentNode{
			{Id: "4", Role: "StaticText", Name: "Hello"},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")

	// nil matches — should write clean tree JSON without any component fields
	if err := writeAnnotatedJSON(node, path, nil, nil, nil); err != nil {
		t.Fatalf("writeAnnotatedJSON: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	got := treeNode(t, data)
	if got["id"] != "3" {
		t.Errorf("id = %v, want 3", got["id"])
	}
	if _, ok := got["component"]; ok {
		t.Errorf("expected no component field with nil matches")
	}
	if _, ok := got["props"]; ok {
		t.Errorf("expected no props field with nil matches")
	}

	// Verify child is present
	children, ok := got["children"].([]any)
	if !ok || len(children) == 0 {
		t.Errorf("expected children in output, got %v", got["children"])
	}
}
