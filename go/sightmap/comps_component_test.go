package sightmap

import (
	"encoding/json"
	"testing"
)

// TestUnmarshalJSON_Minimal verifies that minimal JSON fixtures parse correctly
// with sensible defaults applied.
func TestUnmarshalJSON_Minimal(t *testing.T) {
	jsonData := `{"role": "button", "name": "Submit"}`
	var node ComponentNode
	err := json.Unmarshal([]byte(jsonData), &node)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify required fields
	if node.Role != "button" {
		t.Errorf("Role = %q, want %q", node.Role, "button")
	}
	if node.Name != "Submit" {
		t.Errorf("Name = %q, want %q", node.Name, "Submit")
	}

	// Verify defaults applied
	if node.IsVisible != true {
		t.Errorf("IsVisible = %v, want %v", node.IsVisible, true)
	}
	if node.IsInteractive != false {
		t.Errorf("IsInteractive = %v, want %v", node.IsInteractive, false)
	}
	if node.Properties == nil {
		t.Error("Properties = nil, want empty map")
	}
	if len(node.Properties) != 0 {
		t.Errorf("Properties = %v, want empty map", node.Properties)
	}
	if node.Children == nil {
		t.Error("Children = nil, want empty slice")
	}
	if len(node.Children) != 0 {
		t.Errorf("Children = %v, want empty slice", node.Children)
	}
}

// TestUnmarshalJSON_WithChildren verifies that children are parsed correctly.
func TestUnmarshalJSON_WithChildren(t *testing.T) {
	jsonData := `{
		"role": "WebArea",
		"children": [
			{"role": "button", "name": "Delete"},
			{"role": "button", "name": "Delete"},
			{"role": "link", "name": "Cancel"}
		]
	}`
	var node ComponentNode
	err := json.Unmarshal([]byte(jsonData), &node)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if node.Role != "WebArea" {
		t.Errorf("Role = %q, want %q", node.Role, "WebArea")
	}
	if len(node.Children) != 3 {
		t.Fatalf("len(Children) = %d, want 3", len(node.Children))
	}
	if node.Children[0].Role != "button" || node.Children[0].Name != "Delete" {
		t.Errorf("Children[0] = {%q, %q}, want {button, Delete}", node.Children[0].Role, node.Children[0].Name)
	}
	if node.Children[1].Role != "button" || node.Children[1].Name != "Delete" {
		t.Errorf("Children[1] = {%q, %q}, want {button, Delete}", node.Children[1].Role, node.Children[1].Name)
	}
	if node.Children[2].Role != "link" || node.Children[2].Name != "Cancel" {
		t.Errorf("Children[2] = {%q, %q}, want {link, Cancel}", node.Children[2].Role, node.Children[2].Name)
	}
}

// TestUnmarshalJSON_ExplicitValues verifies that explicitly set values override defaults.
func TestUnmarshalJSON_ExplicitValues(t *testing.T) {
	jsonData := `{
		"role": "button",
		"name": "Hidden Button",
		"isVisible": false,
		"isInteractive": true
	}`
	var node ComponentNode
	err := json.Unmarshal([]byte(jsonData), &node)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if node.IsVisible != false {
		t.Errorf("IsVisible = %v, want false", node.IsVisible)
	}
	if node.IsInteractive != true {
		t.Errorf("IsInteractive = %v, want true", node.IsInteractive)
	}
}

// TestUnmarshalJSON_FullSnapshot verifies backward compatibility with fully-populated snapshots.
func TestUnmarshalJSON_FullSnapshot(t *testing.T) {
	jsonData := `{
		"id": "1",
		"role": "button",
		"name": "Submit",
		"value": "",
		"properties": {"aria-label": "Submit form"},
		"element": {
			"tag": "button",
			"classes": ["btn", "btn-primary"]
		},
		"bounds": {
			"x": 100,
			"y": 200,
			"width": 80,
			"height": 40
		},
		"isVisible": true,
		"isInteractive": true,
		"inViewport": true,
		"isIgnored": false,
		"nthChild": 1,
		"children": []
	}`
	var node ComponentNode
	err := json.Unmarshal([]byte(jsonData), &node)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify all fields preserved
	if node.Id != "1" {
		t.Errorf("Id = %q, want %q", node.Id, "1")
	}
	if node.Role != "button" {
		t.Errorf("Role = %q, want %q", node.Role, "button")
	}
	if node.IsVisible != true {
		t.Errorf("IsVisible = %v, want true", node.IsVisible)
	}
	if node.IsInteractive != true {
		t.Errorf("IsInteractive = %v, want true", node.IsInteractive)
	}
	if node.Bounds == nil || node.Bounds.X != 100 {
		t.Errorf("Bounds.X = %v, want 100", node.Bounds)
	}
	if len(node.Properties) != 1 || node.Properties["aria-label"] != "Submit form" {
		t.Errorf("Properties = %v, want map with aria-label", node.Properties)
	}
}

// TestUnmarshalJSON_SelectorPart verifies that SelectorPart unmarshals correctly.
func TestUnmarshalJSON_SelectorPart(t *testing.T) {
	jsonData := `{
		"role": "button",
		"element": {
			"tag": "button",
			"classes": ["btn", "btn-primary"],
			"attrs": {"data-testid": "submit-btn"}
		}
	}`
	var node ComponentNode
	err := json.Unmarshal([]byte(jsonData), &node)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if node.Element == nil {
		t.Fatal("Selector = nil, want non-nil")
	}
	if node.Element.Tag != "button" {
		t.Errorf("Selector.Tag = %q, want %q", node.Element.Tag, "button")
	}
	if len(node.Element.Classes) != 2 {
		t.Errorf("len(Selector.Classes) = %d, want 2", len(node.Element.Classes))
	}
	if node.Element.Attrs["data-testid"] != "submit-btn" {
		t.Errorf("Selector.Attrs[data-testid] = %q, want %q", node.Element.Attrs["data-testid"], "submit-btn")
	}
}

// TestUnmarshalJSON_EmptyChildren verifies that empty children array is preserved.
func TestUnmarshalJSON_EmptyChildren(t *testing.T) {
	jsonData := `{"role": "button", "children": []}`
	var node ComponentNode
	err := json.Unmarshal([]byte(jsonData), &node)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if node.Children == nil {
		t.Error("Children = nil, want empty slice")
	}
	if len(node.Children) != 0 {
		t.Errorf("len(Children) = %d, want 0", len(node.Children))
	}
}
