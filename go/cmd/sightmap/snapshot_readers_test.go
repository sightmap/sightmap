package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The numbers here mirror the worked example in docs/reference.md (the report /
// multi-coverage sections) so the tests double as a worked example of the
// set aggregation.

// home A/B/C from the doc: load A is full (3 orphans), B/C are reduced (0).
func TestAggregateViewCoverage(t *testing.T) {
	caps := []captureCoverage{
		{t1: 410, t2: 122, t3: 3, total: 535, worstT2Name: "GlobalNav", worstT2Count: 7},
		{t1: 270, t2: 70, t3: 0, total: 340, worstT2Name: "GlobalNav", worstT2Count: 7},
		{t1: 250, t2: 70, t3: 0, total: 320, worstT2Name: "GlobalNav", worstT2Count: 7},
	}
	agg := aggregateViewCoverage(caps)

	if agg.snaps != 3 {
		t.Errorf("snaps = %d, want 3", agg.snaps)
	}
	if agg.sumT1 != 930 || agg.sumT2 != 262 || agg.sumTotal != 1195 {
		t.Errorf("sums = (%d,%d,%d), want (930,262,1195)", agg.sumT1, agg.sumT2, agg.sumTotal)
	}
	// maxT3 must reflect the worst capture so report's gate matches coverage's
	// union fail (any capture with an orphan → ✗).
	if agg.maxT3 != 3 {
		t.Errorf("maxT3 = %d, want 3 (orphan in load A must surface)", agg.maxT3)
	}
	if got := agg.t1pct(); got != 78 {
		t.Errorf("t1pct = %d, want 78", got)
	}
	if got := agg.t2pct(); got != 22 {
		t.Errorf("t2pct = %d, want 22", got)
	}
	if agg.worstT2Name != "GlobalNav" || agg.worstT2Count != 7 {
		t.Errorf("worst T2 = [%s](%d), want [GlobalNav](7)", agg.worstT2Name, agg.worstT2Count)
	}
}

func TestAggregateViewCoverageEmpty(t *testing.T) {
	agg := aggregateViewCoverage(nil)
	if agg.snaps != 0 || agg.t1pct() != 0 || agg.maxT3 != 0 {
		t.Errorf("empty agg = %+v, want zero-valued", agg)
	}
}

// unionViewColumn must MAX each component across the set, rescuing a carousel
// (DigitalEndcap) that rendered in only one of three home loads.
func TestUnionViewColumn(t *testing.T) {
	caps := []map[string]int{
		{"SiteHeader": 1, "GlobalNav": 7, "Footer": 1, "DigitalEndcap": 4, "ProductPod": 12, "PromoBanner": 2},
		{"SiteHeader": 1, "GlobalNav": 7, "Footer": 1, "ProductPod": 12, "PromoBanner": 2},
		{"SiteHeader": 1, "GlobalNav": 7, "Footer": 1, "PromoBanner": 2},
	}
	col := unionViewColumn("home", caps)

	if col.View != "home" || col.Snaps != 3 {
		t.Errorf("col = {View:%q Snaps:%d}, want {home 3}", col.View, col.Snaps)
	}
	want := map[string]int{
		"SiteHeader": 1, "GlobalNav": 7, "Footer": 1,
		"DigitalEndcap": 4, "ProductPod": 12, "PromoBanner": 2,
	}
	if !reflect.DeepEqual(col.Counts, want) {
		t.Errorf("counts = %v, want %v", col.Counts, want)
	}
	if got := col.label(); got != "home·3" {
		t.Errorf("label = %q, want home·3", got)
	}
}

func TestViewColumnLabelSingleCapture(t *testing.T) {
	col := unionViewColumn("pdp", []map[string]int{{"AddToCartButton": 1}})
	if got := col.label(); got != "pdp" {
		t.Errorf("single-capture label = %q, want pdp (no ·N suffix)", got)
	}
}

// globalCandidatesAcrossViews must count VIEWS not files: FacetFilter/PromoBanner
// each live in one view (even if multi-capture) and must NOT be flagged; only
// ProductPod (3 views) is a true candidate.
func TestGlobalCandidatesAcrossViews(t *testing.T) {
	cols := []viewColumn{
		{View: "home", Snaps: 3, Counts: map[string]int{
			"SiteHeader": 1, "GlobalNav": 7, "Footer": 1,
			"DigitalEndcap": 4, "ProductPod": 12, "PromoBanner": 2,
		}},
		{View: "plp", Snaps: 2, Counts: map[string]int{
			"SiteHeader": 1, "GlobalNav": 7, "Footer": 1,
			"ProductPod": 36, "FacetFilter": 8, "SortDropdown": 1,
		}},
		{View: "pdp", Snaps: 1, Counts: map[string]int{
			"SiteHeader": 1, "GlobalNav": 7, "Footer": 1,
			"AddToCartButton": 1, "ProductPod": 6,
		}},
	}
	globals := map[string]bool{"SiteHeader": true, "GlobalNav": true, "Footer": true}

	got := globalCandidatesAcrossViews(cols, globals)
	if len(got) != 1 {
		t.Fatalf("got %d candidates, want 1 (ProductPod only): %+v", len(got), got)
	}
	c := got[0]
	if c.Name != "ProductPod" {
		t.Fatalf("candidate = %q, want ProductPod", c.Name)
	}
	wantHits := []viewHit{{"home", 12}, {"plp", 36}, {"pdp", 6}}
	if !reflect.DeepEqual(c.Hits, wantHits) {
		t.Errorf("hits = %+v, want %+v", c.Hits, wantHits)
	}
}

func TestViewSnapshotSet(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "snapshots", "home")
	if err := os.MkdirAll(filepath.Join(home, "stale-subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(rel string) {
		p := filepath.Join(home, rel)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Given out of order, with .tree.json siblings that must NOT become entries.
	write("20260607T193000Z.snap")
	write("20260607T193000Z.snap.tree.json")
	write("20260607T090000Z.snap")
	write("20260607T090000Z.snap.tree.json")
	write("20260607T140000Z.snap")

	set := viewSnapshotSet(dir, "home")
	if len(set) != 3 {
		t.Fatalf("got %d entries, want 3 (.snap only, subdir excluded): %+v", len(set), set)
	}
	// Must be ordered oldest→newest by stamp.
	wantStamps := []string{"20260607T090000Z", "20260607T140000Z", "20260607T193000Z"}
	for i, e := range set {
		if e.Stamp != wantStamps[i] {
			t.Errorf("entry %d stamp = %q, want %q", i, e.Stamp, wantStamps[i])
		}
	}
}

func TestViewSnapshotSetMissing(t *testing.T) {
	if set := viewSnapshotSet(t.TempDir(), "nope"); len(set) != 0 {
		t.Errorf("missing view set = %+v, want empty", set)
	}
}
