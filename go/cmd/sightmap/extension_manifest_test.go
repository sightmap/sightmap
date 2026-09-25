package main

import (
	"encoding/json"
	"strings"
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

// TestContentScriptDoesNotGateOnGlobals pins the fix for the "overlay renders
// empty" bug: content.js used `state.globals.length` as its "corpus is loaded"
// signal in the hover and poll paths.
//
// `globals` is the OPTIONAL file-root components.yaml list and is omitempty on
// the wire, so a corpus whose components are all view-scoped (the common case)
// ships no `globals` key at all. That made the gate permanently false: hover
// never rendered, while clicks kept resolving because the click path gates on
// state.version instead. The corpus also got re-fetched on every poll.
//
// Readiness must be corpusLoaded() (i.e. state.version), never globals.
func TestContentScriptDoesNotGateOnGlobals(t *testing.T) {
	data, err := extensionFS.ReadFile("extension/content.js")
	if err != nil {
		t.Fatalf("read embedded extension/content.js: %v", err)
	}
	src := string(data)
	if !strings.Contains(src, "function corpusLoaded()") {
		t.Error("extension/content.js: corpusLoaded() helper is gone; readiness checks must not be open-coded")
	}
	if strings.Contains(src, "!state.globals.length") {
		t.Error("extension/content.js: '!state.globals.length' is used as a readiness gate.\n\n" +
			"A corpus with no components.yaml has zero globals, so this gate is permanently\n" +
			"false and silently disables the overlay. Use corpusLoaded() instead.")
	}
}
