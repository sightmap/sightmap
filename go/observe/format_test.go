package observe

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/coverage"
	"github.com/sightmap/sightmap/go/sightmap"
)

func renderResult(r *Result) string {
	var b bytes.Buffer
	Format(&b, r, FormatOpts{CoverageOnly: true})
	return b.String()
}

// TestFormat_NoViewMatched: a corpus is loaded but no view matched the URL — the
// renderer must say so, not silently omit the header.
func TestFormat_NoViewMatched(t *testing.T) {
	r := &Result{
		Root:          &sightmap.ComponentNode{Id: "1", Role: "document", IsVisible: true},
		URL:           "https://app.example.com/login",
		CorpusApplied: true,
		View:          nil,
		Coverage:      coverage.Result{Total: 2, T3: 2},
	}
	out := renderResult(r)
	if !strings.Contains(out, "[No view matched] https://app.example.com/login") {
		t.Errorf("expected a no-view-matched notice, got:\n%s", out)
	}
	if !strings.Contains(out, "[Coverage]") {
		t.Errorf("expected a [Coverage] line even with no view, got:\n%s", out)
	}
}

// TestFormat_CoverageWithZeroMatches: a view matched but zero components did — the
// [Coverage] line must still print (all orphans), not vanish.
func TestFormat_CoverageWithZeroMatches(t *testing.T) {
	r := &Result{
		Root:          &sightmap.ComponentNode{Id: "1", Role: "document", IsVisible: true},
		URL:           "https://app.example.com/x",
		CorpusApplied: true,
		View:          &sightmap.ViewDef{Name: "Stub", Route: "/x"},
		Matches:       nil, // zero components matched
		Coverage:      coverage.Result{Total: 5, T3: 5},
	}
	out := renderResult(r)
	if !strings.Contains(out, "[View: Stub]") {
		t.Errorf("expected the view header, got:\n%s", out)
	}
	if !strings.Contains(out, "[Coverage]") || !strings.Contains(out, "5 interactive") {
		t.Errorf("expected a [Coverage] line with 5 interactive, got:\n%s", out)
	}
}

// TestFormat_Conflicts: runtime ambiguities (tied views + a node matched by
// multiple components) surface in a [Conflicts] section.
func TestFormat_Conflicts(t *testing.T) {
	r := &Result{
		Root:          &sightmap.ComponentNode{Id: "1", Role: "document", IsVisible: true},
		URL:           "https://app.example.com/a/b",
		CorpusApplied: true,
		View:          &sightmap.ViewDef{Name: "AStar", Route: "/a/*"},
		TiedViews:     []string{"AStar", "StarB"},
		ComponentConflicts: []sightmap.Conflict{{
			Node:  &sightmap.ComponentNode{Id: "7", Role: "button", Name: "Save"},
			Names: []string{"AppDialog", "LoginDialog"},
		}},
	}
	out := renderResult(r)
	if !strings.Contains(out, "[Conflicts]") {
		t.Fatalf("expected a [Conflicts] section, got:\n%s", out)
	}
	if !strings.Contains(out, "equal specificity") || !strings.Contains(out, "AStar, StarB") {
		t.Errorf("expected the tied-views line, got:\n%s", out)
	}
	if !strings.Contains(out, "AppDialog, LoginDialog") || !strings.Contains(out, "#7") {
		t.Errorf("expected the component-conflict line, got:\n%s", out)
	}
}

// TestFormat_NoCorpus: no corpus at all — neither the no-view notice nor a
// [Coverage] line should appear (the raw tree stands on its own).
func TestFormat_NoCorpus(t *testing.T) {
	r := &Result{
		Root:          &sightmap.ComponentNode{Id: "1", Role: "document", IsVisible: true},
		URL:           "https://app.example.com/x",
		CorpusApplied: false,
	}
	out := renderResult(r)
	if strings.Contains(out, "[No view matched]") {
		t.Errorf("did not expect a no-view-matched notice without a corpus, got:\n%s", out)
	}
	if strings.Contains(out, "[Coverage]") {
		t.Errorf("did not expect a [Coverage] line without a corpus, got:\n%s", out)
	}
}
