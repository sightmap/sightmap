package sightmap_test

import (
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// broadComp returns a corpus with a single component that triggers
// multi-instance-no-property under static heuristics:
// a bare generic button with a utility class and no properties.
func broadComp(name string) *sightmap.Corpus {
	return corpusFrom([]sightmap.ComponentDef{
		{Name: name, Selectors: []string{"button.btn-action"}},
	}, nil)
}

// TestLintSnapshot_SuppressedWhenCountIsOne: count == 1 → warning suppressed
// (static heuristic was a false positive; exactly one instance in the snapshot).
func TestLintSnapshot_SuppressedWhenCountIsOne(t *testing.T) {
	c := broadComp("AddToCart")
	counts := map[string]int{"AddToCart": 1}
	warnings := sightmap.LintWithCounts(c, counts)
	for _, w := range warnings {
		if w.Rule == "multi-instance-no-property" {
			t.Errorf("expected multi-instance-no-property to be suppressed when count=1, got: %v", w)
		}
	}
}

// TestLintSnapshot_MessageIncludesCountWhenMultiple: count > 1 → warning kept,
// message includes "(matched N times in snapshot)".
func TestLintSnapshot_MessageIncludesCountWhenMultiple(t *testing.T) {
	c := broadComp("AddToCart")
	counts := map[string]int{"AddToCart": 5}
	warnings := sightmap.LintWithCounts(c, counts)

	found := false
	for _, w := range warnings {
		if w.Rule == "multi-instance-no-property" {
			found = true
			want := "(matched 5 times in snapshot)"
			if !strings.Contains(w.Message, want) {
				t.Errorf("message should contain %q; got: %q", want, w.Message)
			}
		}
	}
	if !found {
		t.Error("expected multi-instance-no-property warning, got none")
	}
}

// TestLintSnapshot_MessageIncludesZeroMatches: count == 0 → warning kept,
// message includes "(0 matches in snapshot — selector may be broken)".
func TestLintSnapshot_MessageIncludesZeroMatches(t *testing.T) {
	c := broadComp("AddToCart")
	counts := map[string]int{"AddToCart": 0}
	warnings := sightmap.LintWithCounts(c, counts)

	found := false
	for _, w := range warnings {
		if w.Rule == "multi-instance-no-property" {
			found = true
			want := "(0 matches in snapshot \u2014 selector may be broken)"
			if !strings.Contains(w.Message, want) {
				t.Errorf("message should contain %q; got: %q", want, w.Message)
			}
		}
	}
	if !found {
		t.Error("expected multi-instance-no-property warning, got none")
	}
}

// TestLintSnapshot_NoSnapshotUnchanged: nil counts → identical to Lint().
func TestLintSnapshot_NoSnapshotUnchanged(t *testing.T) {
	c := broadComp("AddToCart")
	base := sightmap.Lint(c)
	withCounts := sightmap.LintWithCounts(c, nil)

	if len(base) != len(withCounts) {
		t.Errorf("Lint() produced %d warnings, LintWithCounts(nil) produced %d; should be equal",
			len(base), len(withCounts))
	}
	if !hasRule(base, "multi-instance-no-property") {
		t.Error("expected multi-instance-no-property from Lint()")
	}
	if !hasRule(withCounts, "multi-instance-no-property") {
		t.Error("expected multi-instance-no-property from LintWithCounts(nil)")
	}
}

// TestLintSnapshot_EmptyCountsMap: empty map → component not in map →
// hasCount=false → same behaviour as Lint().
func TestLintSnapshot_EmptyCountsMap(t *testing.T) {
	c := broadComp("AddToCart")
	base := sightmap.Lint(c)
	withEmpty := sightmap.LintWithCounts(c, map[string]int{})

	if len(base) != len(withEmpty) {
		t.Errorf("Lint() produced %d warnings, LintWithCounts({}) produced %d; should be equal",
			len(base), len(withEmpty))
	}
	// The warning should be present (no suppression — component not in map).
	if !hasRule(withEmpty, "multi-instance-no-property") {
		t.Error("expected multi-instance-no-property when component absent from counts map")
	}
}

// TestLintSnapshot_UnrelatedComponentsUnaffected verifies that a count entry
// for one component does not affect warnings for a different component.
func TestLintSnapshot_UnrelatedComponentsUnaffected(t *testing.T) {
	c := corpusFrom([]sightmap.ComponentDef{
		{Name: "AddToCart", Selectors: []string{"button.btn-action"}},
		{Name: "QuickBuy", Selectors: []string{"button.btn-quick"}},
	}, nil)

	// Suppress only AddToCart; QuickBuy is absent from the map.
	counts := map[string]int{"AddToCart": 1}
	warnings := sightmap.LintWithCounts(c, counts)

	for _, w := range warnings {
		if w.Rule == "multi-instance-no-property" && w.Component == "AddToCart" {
			t.Errorf("AddToCart warning should be suppressed (count=1), got: %v", w)
		}
	}
	foundQuickBuy := false
	for _, w := range warnings {
		if w.Rule == "multi-instance-no-property" && w.Component == "QuickBuy" {
			foundQuickBuy = true
		}
	}
	if !foundQuickBuy {
		t.Error("expected multi-instance-no-property for QuickBuy (absent from counts map)")
	}
}
