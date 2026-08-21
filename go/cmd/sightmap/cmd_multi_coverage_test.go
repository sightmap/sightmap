package main

import (
	"testing"
)

// col is a tiny viewColumn builder for the tests below.
func col(view string, current bool, counts map[string]int) viewColumn {
	return viewColumn{View: view, Snaps: 1, Counts: counts, Current: current}
}

// TestGlobalCandidates_IgnoresStaleColumns is the #248 regression: a stale or
// renamed capture dir (Current=false) must not manufacture "appears in 2+ views"
// evidence for a page's own components. Here "home" (current) and "views" (stale,
// the same page under its old dir name) both render Card and Header; without the
// current-column filter both would look like they appear in two views and get
// wrongly promoted.
func TestGlobalCandidates_IgnoresStaleColumns(t *testing.T) {
	cols := []viewColumn{
		col("home", true, map[string]int{"Card": 3, "Header": 1}),
		col("views", false, map[string]int{"Card": 3, "Header": 1}), // stale dup of home
	}

	got := globalCandidatesAcrossViews(cols, map[string]bool{})
	if len(got) != 0 {
		t.Fatalf("stale column manufactured %d candidate(s), want 0: %+v", len(got), got)
	}
}

// TestGlobalCandidates_CountsCurrentViews confirms the filter doesn't suppress a
// genuine candidate: a component in two CURRENT views is still promoted, and one
// already-global component is skipped.
func TestGlobalCandidates_CountsCurrentViews(t *testing.T) {
	cols := []viewColumn{
		col("home", true, map[string]int{"Nav": 1, "Card": 2}),
		col("search", true, map[string]int{"Nav": 1, "Result": 4}),
		col("legacy", false, map[string]int{"Nav": 1}), // stale: must not inflate Nav's hits
	}

	got := globalCandidatesAcrossViews(cols, map[string]bool{"AlreadyGlobal": true})
	if len(got) != 1 {
		t.Fatalf("got %d candidates, want 1 (Nav): %+v", len(got), got)
	}
	if got[0].Name != "Nav" {
		t.Errorf("candidate = %q, want Nav", got[0].Name)
	}
	// Nav appears in home + search only — the stale "legacy" column is excluded,
	// so its two hits are exactly the two current views.
	if len(got[0].Hits) != 2 {
		t.Errorf("Nav hits = %+v, want exactly the 2 current views", got[0].Hits)
	}
	for _, h := range got[0].Hits {
		if h.View == "legacy" {
			t.Errorf("stale column 'legacy' leaked into Nav's hits: %+v", got[0].Hits)
		}
	}
}

// TestStaleColumns_Detection pins the helper the warning is built from.
func TestStaleColumns_Detection(t *testing.T) {
	cols := []viewColumn{
		col("home", true, nil),
		col("views", false, nil),
		col("old-dir", false, nil),
	}
	stale := staleColumns(cols)
	if len(stale) != 2 {
		t.Fatalf("staleColumns = %d, want 2: %+v", len(stale), stale)
	}
	if stale[0].View != "views" || stale[1].View != "old-dir" {
		t.Errorf("stale columns = %q,%q, want views,old-dir", stale[0].View, stale[1].View)
	}
}

// TestViewColumnLabel_MarksStale: the matrix label suffixes non-current columns
// with "*" (and still shows the ·N set-size marker).
func TestViewColumnLabel_MarksStale(t *testing.T) {
	if got := (viewColumn{View: "home", Snaps: 1, Current: true}).label(); got != "home" {
		t.Errorf("current single-capture label = %q, want home", got)
	}
	if got := (viewColumn{View: "home", Snaps: 3, Current: true}).label(); got != "home·3" {
		t.Errorf("current multi-capture label = %q, want home·3", got)
	}
	if got := (viewColumn{View: "views", Snaps: 1, Current: false}).label(); got != "views*" {
		t.Errorf("stale label = %q, want views*", got)
	}
}
