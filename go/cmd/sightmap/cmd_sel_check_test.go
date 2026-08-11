package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// selCheckFixtureJSON is a minimal .snap.tree.json used by the sel-check tests.
// Tree structure:
//
//	root (document, id=1)
//	├── id=10  div  data-component="product-details:ProductDetails:v9.32.1"
//	├── id=20  button  data-testid="add-to-cart"  name="Add to Cart"
//	└── id=30  div  data-component="product-details:ProductDetails:v9.32.1"  (hidden)
//	    └── id=31  h1  data-component="product-details:ProductTitle:v1"  name="Product Title Here"
const selCheckFixtureJSON = `{
  "id": "1",
  "role": "document",
  "name": "Test Page",
  "isVisible": true,
  "isInteractive": false,
  "children": [
    {
      "id": "10",
      "role": "none",
      "name": "",
      "element": {
        "tag": "div",
        "attrs": {"data-component": "product-details:ProductDetails:v9.32.1"}
      },
      "isVisible": true,
      "isInteractive": false,
      "children": []
    },
    {
      "id": "20",
      "role": "button",
      "name": "Add to Cart",
      "element": {
        "tag": "button",
        "attrs": {"data-testid": "add-to-cart"}
      },
      "isVisible": true,
      "isInteractive": true,
      "children": []
    },
    {
      "id": "30",
      "role": "none",
      "name": "",
      "element": {
        "tag": "div",
        "attrs": {"data-component": "product-details:ProductDetails:v9.32.1"}
      },
      "isVisible": false,
      "isInteractive": false,
      "children": [
        {
          "id": "31",
          "role": "generic",
          "name": "Product Title Here",
          "element": {
            "tag": "h1",
            "attrs": {"data-component": "product-details:ProductTitle:v1"}
          },
          "isVisible": false,
          "isInteractive": false,
          "children": []
        }
      ]
    }
  ]
}`

// writeSelCheckFixture writes selCheckFixtureJSON to a temp dir and returns
// the path to the .snap.tree.json file.
func writeSelCheckFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	treeFile := filepath.Join(dir, "base.snap.tree.json")
	if err := os.WriteFile(treeFile, []byte(selCheckFixtureJSON), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return treeFile
}

// runSelCheckCapture runs runSelCheckOut and returns (stdout, error).
func runSelCheckCapture(args []string) (string, error) {
	var buf bytes.Buffer
	err := runSelCheckOut(args, &buf)
	return buf.String(), err
}

// ── basic matching ────────────────────────────────────────────────────────────

func TestSelCheck_matchFound(t *testing.T) {
	treeFile := writeSelCheckFixture(t)

	out, err := runSelCheckCapture([]string{`[data-component*=":ProductDetails:"]`, treeFile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should report 2 matches (nodes 10 and 30).
	if !strings.Contains(out, "2 matches") {
		t.Errorf("expected '2 matches' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "node 10") {
		t.Errorf("expected 'node 10' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "node 30") {
		t.Errorf("expected 'node 30' in output, got:\n%s", out)
	}
	// Node 31 has a different data-component — should NOT appear.
	if strings.Contains(out, "node 31") {
		t.Errorf("node 31 should not match, but appears in output:\n%s", out)
	}
}

func TestSelCheck_matchShowsAttributeValue(t *testing.T) {
	treeFile := writeSelCheckFixture(t)

	out, err := runSelCheckCapture([]string{`[data-component*=":ProductDetails:"]`, treeFile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The data-component value should be printed under matching nodes.
	if !strings.Contains(out, "data-component: product-details:ProductDetails:v9.32.1") {
		t.Errorf("expected data-component attribute line in output, got:\n%s", out)
	}
}

func TestSelCheck_matchSingleNode(t *testing.T) {
	treeFile := writeSelCheckFixture(t)

	out, err := runSelCheckCapture([]string{`button[data-testid="add-to-cart"]`, treeFile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "1 match") {
		t.Errorf("expected '1 match' in output, got:\n%s", out)
	}
	// Singular form.
	if strings.Contains(out, "1 matches") {
		t.Errorf("expected singular 'match', got 'matches':\n%s", out)
	}
	if !strings.Contains(out, "node 20") {
		t.Errorf("expected 'node 20' in output, got:\n%s", out)
	}
	if !strings.Contains(out, `"Add to Cart"`) {
		t.Errorf("expected accessible name in output, got:\n%s", out)
	}
	if !strings.Contains(out, "data-testid: add-to-cart") {
		t.Errorf("expected data-testid attribute line in output, got:\n%s", out)
	}
}

func TestSelCheck_hiddenNodesIncludedByDefault(t *testing.T) {
	treeFile := writeSelCheckFixture(t)

	// Node 30 is isVisible=false but should still appear — no filtering by default.
	out, err := runSelCheckCapture([]string{`[data-component*=":ProductDetails:"]`, treeFile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "node 30") {
		t.Errorf("expected hidden node 30 in output (default: show all), got:\n%s", out)
	}
}

// ── 0 matches ─────────────────────────────────────────────────────────────────

func TestSelCheck_noMatches(t *testing.T) {
	treeFile := writeSelCheckFixture(t)

	out, err := runSelCheckCapture([]string{`button[data-testid="nonexistent"]`, treeFile})
	if err == nil {
		t.Fatal("expected non-nil error (exit 1) for 0 matches")
	}
	if !errors.Is(err, errSelCheckNoMatches) {
		t.Errorf("expected errSelCheckNoMatches, got: %v", err)
	}
	if !strings.Contains(out, "0 matches") {
		t.Errorf("expected '0 matches' in output, got:\n%s", out)
	}
}

// ── .snap path auto-resolution ────────────────────────────────────────────────

func TestSelCheck_snapFileAutoResolvesTreeJSON(t *testing.T) {
	treeFile := writeSelCheckFixture(t)
	// Derive the .snap path (without the .tree.json suffix).
	snapFile := strings.TrimSuffix(treeFile, ".tree.json")

	// The .snap file need not exist — sel-check only reads the .tree.json.
	out, err := runSelCheckCapture([]string{`[data-component*=":ProductDetails:"]`, snapFile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "2 matches") {
		t.Errorf("expected '2 matches' in output, got:\n%s", out)
	}
	// Display name should not have the .tree.json suffix.
	if strings.Contains(out, ".tree.json") {
		t.Errorf("display name should not include .tree.json, got:\n%s", out)
	}
}

func TestSelCheck_treeJSONPassedDirectly(t *testing.T) {
	treeFile := writeSelCheckFixture(t)

	out, err := runSelCheckCapture([]string{`[data-component*=":ProductDetails:"]`, treeFile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "2 matches") {
		t.Errorf("expected '2 matches' in output, got:\n%s", out)
	}
}

// ── error cases ───────────────────────────────────────────────────────────────

func TestSelCheck_noFileArgError(t *testing.T) {
	_, err := runSelCheckCapture([]string{`[data-testid="foo"]`})
	if err == nil {
		t.Fatal("expected error when no file argument is given")
	}
	if !strings.Contains(err.Error(), "snapshot file") {
		t.Errorf("expected 'snapshot file' in error message, got: %v", err)
	}
}

func TestSelCheck_missingFileError(t *testing.T) {
	_, err := runSelCheckCapture([]string{`[data-testid="foo"]`, "/nonexistent/path.snap"})
	if err == nil {
		t.Fatal("expected error for missing tree file")
	}
}

func TestSelCheck_invalidSelectorError(t *testing.T) {
	treeFile := writeSelCheckFixture(t)
	_, err := runSelCheckCapture([]string{`[unclosed`, treeFile})
	if err == nil {
		t.Fatal("expected error for invalid selector")
	}
}

// ── --limit flag ──────────────────────────────────────────────────────────────

func TestSelCheck_limitFlag(t *testing.T) {
	treeFile := writeSelCheckFixture(t)

	// There are 2 ProductDetails matches; limit=1 should show 1 and report "1 more".
	out, err := runSelCheckCapture([]string{"--limit", "1", `[data-component*=":ProductDetails:"]`, treeFile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Header still shows the total count.
	if !strings.Contains(out, "2 matches") {
		t.Errorf("expected '2 matches' total in header, got:\n%s", out)
	}
	// Only one node detail block should be printed; the second is cut off.
	if !strings.Contains(out, "1 more") {
		t.Errorf("expected '1 more' overflow line in output, got:\n%s", out)
	}
}

// ── selCheckResolvePaths ──────────────────────────────────────────────────────

func TestSelCheckResolvePaths_snapFile(t *testing.T) {
	treeFile, displayName := selCheckResolvePaths("foo/bar.snap")
	if treeFile != "foo/bar.snap.tree.json" {
		t.Errorf("treeFile = %q, want foo/bar.snap.tree.json", treeFile)
	}
	if displayName != "foo/bar.snap" {
		t.Errorf("displayName = %q, want foo/bar.snap", displayName)
	}
}

func TestSelCheckResolvePaths_treeJSONFile(t *testing.T) {
	treeFile, displayName := selCheckResolvePaths("foo/bar.snap.tree.json")
	if treeFile != "foo/bar.snap.tree.json" {
		t.Errorf("treeFile = %q, want foo/bar.snap.tree.json", treeFile)
	}
	if displayName != "foo/bar.snap" {
		t.Errorf("displayName = %q, want foo/bar.snap", displayName)
	}
}
