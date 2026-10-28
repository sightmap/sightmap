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

// TestContentScriptRunsInEveryFrame pins the overlay into iframes. Without
// all_frames the content script runs only in the top frame, so components inside
// any iframe (an MCP Apps view, an embedded widget, a payment form) never match.
// Those views are typically about:blank or srcdoc frames written by their host
// (match_about_blank), sometimes under an opaque sandboxed origin
// (match_origin_as_fallback).
func TestContentScriptRunsInEveryFrame(t *testing.T) {
	data, err := extensionFS.ReadFile("extension/manifest.json")
	if err != nil {
		t.Fatalf("read embedded extension/manifest.json: %v", err)
	}
	var m struct {
		ContentScripts []map[string]any `json:"content_scripts"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if len(m.ContentScripts) == 0 {
		t.Fatal("manifest declares no content_scripts")
	}
	for _, key := range []string{"all_frames", "match_about_blank", "match_origin_as_fallback"} {
		if m.ContentScripts[0][key] != true {
			t.Errorf("content_scripts[0].%s must be true, or the overlay misses components inside iframes", key)
		}
	}

	src, err := extensionFS.ReadFile("extension/content.js")
	if err != nil {
		t.Fatalf("read embedded extension/content.js: %v", err)
	}
	// A host that fills the frame with document.open()/write() after injection
	// erases the overlay nodes and listeners; content.js must re-arm.
	if !strings.Contains(string(src), "function arm()") {
		t.Error("extension/content.js: arm() is gone; a document.open()-written frame would lose the overlay")
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
