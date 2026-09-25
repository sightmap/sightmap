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

// TestFormat_ChromeOnlyAdvisory: a clean pass (T3==0) whose only matches are
// GLOBAL components must warn that the view isn't really modeled (a11e.9).
func TestFormat_ChromeOnlyAdvisory(t *testing.T) {
	n := &sightmap.ComponentNode{Id: "1", Role: "banner", IsVisible: true, IsInteractive: true}
	r := &Result{
		Root:          &sightmap.ComponentNode{Id: "root", Children: []*sightmap.ComponentNode{n}},
		URL:           "https://app.example.com/x",
		CorpusApplied: true,
		View:          &sightmap.ViewDef{Name: "Stub", Route: "/x"},
		Matches:       map[*sightmap.ComponentNode]*sightmap.ComponentMatch{n: {Name: "GlobalHeader"}},
		GlobalNames:   map[string]bool{"GlobalHeader": true},
		Coverage:      coverage.Result{Total: 3, T1: 1, T2: 2, T3: 0},
	}
	out := renderResult(r)
	if !strings.Contains(out, "contributed 0 components") {
		t.Errorf("expected chrome-only advisory, got:\n%s", out)
	}
}

// TestFormat_NoChromeOnlyAdvisoryWhenViewModels: a view-scoped match suppresses
// the advisory — a legitimate pass must stay quiet.
func TestFormat_NoChromeOnlyAdvisoryWhenViewModels(t *testing.T) {
	n := &sightmap.ComponentNode{Id: "1", Role: "button", IsVisible: true, IsInteractive: true}
	r := &Result{
		Root:          &sightmap.ComponentNode{Id: "root", Children: []*sightmap.ComponentNode{n}},
		URL:           "https://app.example.com/x",
		CorpusApplied: true,
		View:          &sightmap.ViewDef{Name: "Stub", Route: "/x"},
		Matches:       map[*sightmap.ComponentNode]*sightmap.ComponentMatch{n: {Name: "SaveButton"}},
		GlobalNames:   map[string]bool{"GlobalHeader": true},
		Coverage:      coverage.Result{Total: 1, T1: 1, T3: 0},
	}
	out := renderResult(r)
	if strings.Contains(out, "contributed 0 components") || strings.Contains(out, "no view modeled") {
		t.Errorf("advisory should NOT fire when a view-scoped component matched, got:\n%s", out)
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

// TestFormat_MatchedInvisible_TreeAgreesWithT2Trace reproduces the user-visible
// symptom of the matched-invisible bug: a sightmap-matched container that did
// not paint (e.g. a display:contents or zero-size wrapper) whose interactive
// descendants ARE visible. coverage.Score attributes those descendants to the
// matched ancestor with no visibility check (T2 scopes → "[Navbar] (2)"), and
// the [Guide] reports the match. But render.Filter's invisible branch lacked
// the `&& !matched` guard its sibling transparency rules carried, so it made
// the matched node transparent: the [Navbar] wrapper vanished from the
// component tree and the visible links were promoted under a synthetic
// `document` root — a self-contradiction within a single observe.Format stream.
//
// Under the fix the matched invisible node is kept, so the tree and the
// T2 trace agree: a `[Navbar]` wrapper renders above the visible links and no
// synthetic `document` root appears. This is the end-to-end counterpart to the
// render.Filter unit tests (see go/render/render_test.go:
// TestFilter_MatchedInvisible_KeepsWrapper).
func TestFormat_MatchedInvisible_TreeAgreesWithT2Trace(t *testing.T) {
	navbar := &sightmap.ComponentNode{
		Role: "generic", Name: "", IsVisible: false, IsInteractive: false,
		Children: []*sightmap.ComponentNode{
			{Role: "link", Name: "Open", IsVisible: true, IsInteractive: true},
			{Role: "link", Name: "Settings", IsVisible: true, IsInteractive: true},
		},
	}
	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		navbar: {Name: "Navbar"},
	}
	cov := coverage.Score(navbar, matches, coverage.Options{VisibleOnly: true})
	r := &Result{
		Root:          navbar,
		Matches:       matches,
		Coverage:      cov,
		View:          &sightmap.ViewDef{Name: "page", Route: "/"},
		GlobalNames:   map[string]bool{},
		CorpusApplied: true,
	}
	var b bytes.Buffer
	Format(&b, r, FormatOpts{Trace: true})
	out := b.String()

	// T2 trace must still attribute both links to [Navbar] (grouping is
	// visibility-agnostic by design — this was correct even under the bug).
	if !strings.Contains(out, "[Navbar] (2)") {
		t.Errorf("expected T2 trace to show '[Navbar] (2)', got:\n%s", out)
	}
	// The component tree must now agree: the [Navbar] wrapper renders on its own
	// line above its visible descendants (not promoted under a synthetic root).
	if !strings.Contains(out, "\n[Navbar]\n") {
		t.Errorf("expected component tree to include a '[Navbar]' wrapper line, got:\n%s", out)
	}
	// The synthetic `document` root (Filter's ≥2-child fallback, fired when the
	// matched node was incorrectly made transparent) must NOT appear.
	if strings.Contains(out, "\ndocument\n") {
		t.Errorf("expected no synthetic 'document' root in the component tree, got:\n%s", out)
	}
	// Both visible interactive descendants must still render under the wrapper.
	if !strings.Contains(out, `link "Open"`) || !strings.Contains(out, `link "Settings"`) {
		t.Errorf("expected both visible links to be preserved, got:\n%s", out)
	}
}
