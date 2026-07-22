package main

import (
	"encoding/json"
	"testing"
)

// TestExtensionManifestValid ensures the embedded manifest.json is valid JSON
// and has a non-empty version field. This catches the recurring "dropped
// trailing comma on version bump" mistake before it reaches users.
func TestExtensionManifestValid(t *testing.T) {
	data, err := extensionFS.ReadFile("extension/manifest.json")
	if err != nil {
		t.Fatalf("read embedded extension/manifest.json: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("extension/manifest.json is invalid JSON: %v\n\nHint: check for a missing comma after the version field.", err)
	}
	v, _ := m["version"].(string)
	if v == "" {
		t.Error("extension/manifest.json: 'version' field is missing or empty")
	}
}
