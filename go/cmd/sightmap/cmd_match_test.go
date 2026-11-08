package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A capture matched by its URL: the view named, a toolbar's buttons named, an
// unhooked panel's link and heading reported as one unmatched region under
// the panel's test hook, and a node two definitions claim reported once.
func TestMatchRecord(t *testing.T) {
	dir := t.TempDir()
	sm := filepath.Join(dir, ".sightmap")
	write := func(p, s string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(sm, "views", "home.yaml"), `version: 1
views:
  - name: Home
    route: /home
    components:
      - name: Toolbar
        selector: div[data-testid="toolbar"]
        children:
          - name: ToolbarButton
            selector: button
      - name: SaveButton
        selector: button.save
`)
	node := func(role, name, tag string, attrs map[string]string, classes []string, interactive bool, kids ...map[string]any) map[string]any {
		return map[string]any{
			"id": name, "role": role, "name": name, "isVisible": true, "isInteractive": interactive,
			"element":  map[string]any{"tag": tag, "attrs": attrs, "classes": classes},
			"children": kids,
		}
	}
	tree := node("RootWebArea", "", "body", nil, nil, false,
		node("generic", "", "div", map[string]string{"data-testid": "toolbar"}, nil, false,
			node("button", "Save", "button", nil, []string{"save"}, true),
			node("button", "Undo", "button", nil, nil, true),
		),
		node("generic", "", "div", map[string]string{"data-testid": "help-panel"}, nil, false,
			node("heading", "Help", "h2", nil, nil, false),
			node("link", "Docs", "a", nil, nil, true),
		),
	)
	snap := filepath.Join(sm, "snapshots", "_inbox_", "x.snap")
	write(snap, "url: https://example.com/home?tab=1\n\n")
	b, _ := json.Marshal(tree)
	write(snap+".tree.json", string(b))

	var out bytes.Buffer
	if err := runMatchOut([]string{"--sightmap-dir", sm, snap}, &out); err != nil {
		t.Fatal(err)
	}
	var rec matchRecord
	if err := json.Unmarshal(out.Bytes(), &rec); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if rec.View != "Home" {
		t.Errorf("view = %q, want Home", rec.View)
	}
	if rec.Nodes != 4 || rec.Named != 2 {
		t.Errorf("nodes/named = %d/%d, want 4/2: %+v", rec.Nodes, rec.Named, rec)
	}
	if len(rec.Unmatched) != 1 || rec.Unmatched[0].Anchor != `div[data-testid="help-panel"]` ||
		rec.Unmatched[0].Interactive != 1 || rec.Unmatched[0].Content != 1 {
		t.Errorf("unmatched = %+v, want one help-panel region (1 interactive, 1 content)", rec.Unmatched)
	}
	if len(rec.Conflicts) != 1 || len(rec.Conflicts[0].Names) != 2 {
		t.Errorf("conflicts = %+v, want the Save button claimed by ToolbarButton and SaveButton", rec.Conflicts)
	}
}

// Captures that cannot be matched still yield one record each, with an error
// and empty (not null) slices; a directory argument is walked, and one with no
// captures fails.
func TestMatchErrorRecords(t *testing.T) {
	dir := t.TempDir()
	sm := filepath.Join(dir, ".sightmap")
	if err := os.MkdirAll(filepath.Join(sm, "views"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sm, "views", "home.yaml"), []byte("version: 1\nviews:\n  - name: Home\n    route: /home\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	caps := filepath.Join(dir, "caps")
	if err := os.MkdirAll(caps, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, s string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(caps, name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.snap", "url: https://example.com/home\n\n") // no .tree.json
	write("b.snap", "\n")                                // no url header
	write("b.snap.tree.json", `{}`)

	var out bytes.Buffer
	if err := runMatchOut([]string{"--sightmap-dir", sm, caps}, &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d records, want 2: %s", len(lines), out.String())
	}
	for i, l := range lines {
		var rec matchRecord
		if err := json.Unmarshal([]byte(l), &rec); err != nil {
			t.Fatal(err)
		}
		if rec.Error == "" {
			t.Errorf("record %d: want an error: %s", i, l)
		}
		if !strings.Contains(l, `"unmatched":[]`) || !strings.Contains(l, `"conflicts":[]`) {
			t.Errorf("record %d: unmatched and conflicts must be [], got %s", i, l)
		}
	}

	empty := filepath.Join(dir, "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runMatchOut([]string{"--sightmap-dir", sm, empty}, &out); err == nil {
		t.Error("a directory with no .snap files should fail")
	}
}
