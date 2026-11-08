package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

func matchNode(role, name, tag string, classes []string, interactive, visible bool, kids ...map[string]any) map[string]any {
	return map[string]any{
		"id": name, "role": role, "name": name, "isVisible": visible, "isInteractive": interactive,
		"element":  map[string]any{"tag": tag, "classes": classes},
		"children": kids,
	}
}

func matchButtons(n int, visible bool) []map[string]any {
	var b []map[string]any
	for i := 0; i < n; i++ {
		b = append(b, matchNode("button", fmt.Sprintf("b%d", i), "button", nil, true, visible))
	}
	return b
}

// One capture per case, matched through a directory argument.
func TestMatchCases(t *testing.T) {
	const home = "version: 1\nviews:\n  - name: Home\n    route: /home\n"
	const bigSmall = home + "    components:\n      - name: Box\n        selector: div.box\n"
	root := func(kids ...map[string]any) map[string]any {
		return matchNode("RootWebArea", "", "body", nil, false, true, kids...)
	}
	box := func(n int) map[string]any {
		return matchNode("generic", "", "div", []string{"box"}, false, true, matchButtons(n, true)...)
	}
	long := strings.Repeat("€", 41)

	tests := []struct {
		name       string
		yaml       string
		header     string         // capture header; "" writes no url: line
		tree       map[string]any // nil writes no .tree.json
		noCapture  bool           // the directory holds no .snap
		args       []string       // extra flags
		wantErr    string         // run error substring
		wantRecErr string         // record error substring
		wantView   string
		wantTied   []string
		wantNodes  int
		wantInside int
		wantGaps   int // number of unmatched regions
		wantAnchor string
		wantSample string
	}{
		{
			name:   "url resolves to no view",
			yaml:   home,
			header: "https://example.com/other",
			tree:   root(),
		},
		{
			name:   "tied routes",
			yaml:   "version: 1\nviews:\n  - name: A\n    route: /t/:x\n  - name: B\n    route: /t/:y\n",
			header: "https://example.com/t/1", tree: root(),
			wantView: "A", wantTied: []string{"A", "B"},
		},
		{
			name: "missing url header", yaml: home, header: "", tree: root(),
			wantRecErr: "no url: header",
		},
		{
			name: "missing tree", yaml: home, header: "https://example.com/home",
			wantRecErr: "tree.json",
		},
		{
			name: "no captures", yaml: home, noCapture: true,
			wantErr: "no .snap captures found",
		},
		{
			name: "hidden node skipped", yaml: home, header: "https://example.com/home",
			tree:     root(matchNode("button", "x", "button", nil, true, false)),
			wantView: "Home",
		},
		{
			name: "hidden node counted with --include-hidden", yaml: home, header: "https://example.com/home",
			tree:     root(matchNode("button", "x", "button", nil, true, false)),
			args:     []string{"--include-hidden"},
			wantView: "Home", wantNodes: 1, wantGaps: 1, wantAnchor: "(page)",
		},
		{
			name: "named ancestor of 20 nodes encloses", yaml: bigSmall, header: "https://example.com/home",
			tree:     root(box(19)),
			wantView: "Home", wantNodes: 19, wantInside: 19,
		},
		{
			name: "named ancestor of 21 nodes only prefixes the gap", yaml: bigSmall, header: "https://example.com/home",
			tree:     root(box(20)),
			wantView: "Home", wantNodes: 20, wantGaps: 1, wantAnchor: "Box > (page)",
		},
		{
			name: "sample name cut on a rune boundary", yaml: home, header: "https://example.com/home",
			tree:     root(matchNode("button", long, "button", nil, true, true)),
			wantView: "Home", wantNodes: 1, wantGaps: 1, wantAnchor: "(page)",
			wantSample: fmt.Sprintf("button %q", strings.Repeat("€", 40)+"…"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			sm := filepath.Join(dir, ".sightmap")
			caps := filepath.Join(dir, "caps")
			for p, s := range map[string]string{filepath.Join(sm, "views", "v.yaml"): tc.yaml} {
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.MkdirAll(caps, 0o755); err != nil {
				t.Fatal(err)
			}
			snap := filepath.Join(caps, "c.snap")
			if !tc.noCapture {
				body := "\n"
				if tc.header != "" {
					body = "url: " + tc.header + "\n\n"
				}
				if err := os.WriteFile(snap, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
				if tc.tree != nil {
					b, _ := json.Marshal(tc.tree)
					if err := os.WriteFile(snap+".tree.json", b, 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}

			var out bytes.Buffer
			args := append(append([]string{"--sightmap-dir", sm}, tc.args...), caps)
			err := runMatchOut(args, &out)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			raw := strings.TrimSpace(out.String())
			var rec matchRecord
			if err := json.Unmarshal([]byte(raw), &rec); err != nil {
				t.Fatalf("%v: %s", err, raw)
			}
			if strings.Contains(raw, "null") {
				t.Errorf("record must not contain null: %s", raw)
			}
			if tc.wantRecErr != "" {
				if !strings.Contains(rec.Error, tc.wantRecErr) {
					t.Fatalf("record error = %q, want %q", rec.Error, tc.wantRecErr)
				}
				return
			}
			if rec.Error != "" {
				t.Fatalf("unexpected record error: %s", rec.Error)
			}
			if rec.View != tc.wantView {
				t.Errorf("view = %q, want %q", rec.View, tc.wantView)
			}
			if !slices.Equal(rec.TiedViews, tc.wantTied) {
				t.Errorf("tiedViews = %v, want %v", rec.TiedViews, tc.wantTied)
			}
			if rec.Nodes != tc.wantNodes || rec.Inside != tc.wantInside {
				t.Errorf("nodes/inside = %d/%d, want %d/%d", rec.Nodes, rec.Inside, tc.wantNodes, tc.wantInside)
			}
			if len(rec.Unmatched) != tc.wantGaps {
				t.Fatalf("unmatched = %+v, want %d region(s)", rec.Unmatched, tc.wantGaps)
			}
			if tc.wantGaps > 0 {
				g := rec.Unmatched[0]
				if g.Anchor != tc.wantAnchor {
					t.Errorf("anchor = %q, want %q", g.Anchor, tc.wantAnchor)
				}
				if tc.wantSample != "" && (len(g.Samples) == 0 || g.Samples[0] != tc.wantSample) {
					t.Errorf("samples = %q, want %q first", g.Samples, tc.wantSample)
				}
			}
		})
	}
}
