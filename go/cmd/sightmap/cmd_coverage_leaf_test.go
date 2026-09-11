package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
	"github.com/sightmap/sightmap/go/viewset"
)

// These tests guard coverCapture's leaf-probe: the dead-component diagnostic
// that distinguishes a broken scoped selector (leaf part matches nodes but the
// full scoped selector matched none) from a genuinely-absent component (even
// the leaf matches nothing). The probe runs only over components with 0 full
// matches and iterates every selector ALTERNATIVE, OR-ed the way the matcher
// does — probing only Selectors[0] misclassifies a broken selector as absent
// when the first alternative's leaf is missing but a later one's is present.

// TestCoverCaptureLeafProbe_ProbesAllAlternatives asserts the leaf-probe
// classifies a broken selector as [Warnings], not [Absent], when the component
// has multiple distinct-leaf alternatives and only a LATER alternative's leaf
// is present in the DOM (the canonical "parent × comma-child alternatives"
// flattening shape). Drives the real matcher, UnionPresence, and
// emitUnionWarnings so the whole dead-component pipeline is covered.
func TestCoverCaptureLeafProbe_ProbesAllAlternatives(t *testing.T) {
	legacyRoot := &sightmap.ComponentNode{
		Element: &sightmap.Element{Tag: "div", Classes: []string{"legacy-modal-foo"}},
		Children: []*sightmap.ComponentNode{
			{Element: &sightmap.Element{Tag: "input", Classes: []string{"card-field"}}},
		},
	}
	comp := sightmap.ComponentDef{
		Name: "CardField",
		Selectors: []string{
			`.checkout [data-testid="card-input"]`, // primary leaf, absent
			".legacy-modal input.card-field",       // fallback leaf, present
		},
	}
	corpus := &sightmap.Corpus{Views: []sightmap.ViewDef{{
		Name: "checkout", Route: "/checkout",
		Components: []sightmap.ComponentDef{comp},
	}}}

	counts := matcherCounts(corpus, legacyRoot, "/checkout")
	if got := counts["CardField"]; got != 0 {
		t.Fatalf("precondition: full matcher must report 0 CardField matches, got %d", got)
	}

	leafCounts := probeDeadComponentLeaves(legacyRoot, []sightmap.ComponentDef{comp}, counts)
	if leafCounts["CardField"] == 0 {
		t.Fatalf("expected a leaf match for CardField, got %v", leafCounts)
	}

	out := captureStdout(func() {
		pres := viewset.UnionPresence(viewComponentNames(corpus, "checkout"), []viewset.MatchCounts{
			{Stamp: "20260903T120000Z", Counts: counts, LeafCounts: leafCounts},
		})
		emitUnionWarnings("checkout", pres)
	})

	if !strings.Contains(out, "[Warnings]") {
		t.Errorf("expected [Warnings] classification, got:\n%s", out)
	}
	if !strings.Contains(out, "selector likely broken") {
		t.Errorf("expected 'selector likely broken' advisory, got:\n%s", out)
	}
	if strings.Contains(out, "[Absent]") {
		t.Errorf("did not expect [Absent] once a broken selector is detected, got:\n%s", out)
	}
}

// TestCoverCaptureLeafProbe_BreakOnFirstMatchingLeaf guards the "break on first
// leaf-matching alternative" choice: when alternatives share a leaf token, the
// probe must record the leaf node count once and stop — never sum across
// alternatives (which would double-count and break the per-leaf node-count
// contract documented on viewset.MatchCounts.LeafCounts).
func TestCoverCaptureLeafProbe_BreakOnFirstMatchingLeaf(t *testing.T) {
	root := &sightmap.ComponentNode{
		Element: &sightmap.Element{Tag: "div", Classes: []string{"checkout"}},
		Children: []*sightmap.ComponentNode{
			{Element: &sightmap.Element{Tag: "input", Classes: []string{"card-field"}}},
			{Element: &sightmap.Element{Tag: "input", Classes: []string{"card-field"}}},
		},
	}
	// Both alternatives share the leaf `input.card-field`; a naive sum would
	// double-count the two matching nodes.
	comp := sightmap.ComponentDef{
		Name: "CardField",
		Selectors: []string{
			".checkout input.card-field",
			".checkout input.card-field",
		},
	}
	counts := map[string]int{"CardField": 0}

	leafCounts := probeDeadComponentLeaves(root, []sightmap.ComponentDef{comp}, counts)
	if got := leafCounts["CardField"]; got != 2 {
		t.Errorf("expected leaf count 2 (no double-count), got %d", got)
	}
}

// TestCoverCaptureLeafProbe_AllLeavesAbsentIsAbsent confirms the fix did not
// regress the genuinely-absent path: when NO alternative's leaf matches, the
// probe records nothing and the component is classified [Absent], not
// [Warnings].
func TestCoverCaptureLeafProbe_AllLeavesAbsentIsAbsent(t *testing.T) {
	root := &sightmap.ComponentNode{
		Element: &sightmap.Element{Tag: "div", Classes: []string{"checkout"}},
		Children: []*sightmap.ComponentNode{
			{Element: &sightmap.Element{Tag: "button", Classes: []string{"submit"}}},
		},
	}
	comp := sightmap.ComponentDef{
		Name: "CardField",
		Selectors: []string{
			`.checkout [data-testid="card-input"]`,
			".legacy-modal input.card-field",
		},
	}
	corpus := &sightmap.Corpus{Views: []sightmap.ViewDef{{
		Name: "checkout", Route: "/checkout",
		Components: []sightmap.ComponentDef{comp},
	}}}

	counts := matcherCounts(corpus, root, "/checkout")
	if got := counts["CardField"]; got != 0 {
		t.Fatalf("precondition: matcher must report 0 CardField matches, got %d", got)
	}

	leafCounts := probeDeadComponentLeaves(root, []sightmap.ComponentDef{comp}, counts)
	if leafCounts != nil {
		t.Fatalf("genuinely-absent component must produce no leaf counts, got %v", leafCounts)
	}

	out := captureStdout(func() {
		pres := viewset.UnionPresence(viewComponentNames(corpus, "checkout"), []viewset.MatchCounts{
			{Stamp: "20260903T120000Z", Counts: counts, LeafCounts: leafCounts},
		})
		emitUnionWarnings("checkout", pres)
	})

	if !strings.Contains(out, "[Absent]") {
		t.Errorf("expected [Absent], got:\n%s", out)
	}
	if strings.Contains(out, "[Warnings]") {
		t.Errorf("absent component must not emit [Warnings], got:\n%s", out)
	}
}

// probeDeadComponentLeaves mirrors the leaf-probe block of coverCapture in
// cmd_coverage.go. It exists as a test helper so the dead-component diagnostic
// can be exercised without a saved .snap + .tree.json fixture on disk.
func probeDeadComponentLeaves(root *sightmap.ComponentNode, comps []sightmap.ComponentDef, counts map[string]int) map[string]int {
	var leafCounts map[string]int
	for _, comp := range comps {
		if counts[comp.Name] > 0 || len(comp.Selectors) == 0 {
			continue
		}
		for _, sel := range comp.Selectors {
			if n := countLeafMatches(root, sel); n > 0 {
				if leafCounts == nil {
					leafCounts = make(map[string]int)
				}
				leafCounts[comp.Name] = n
				break
			}
		}
	}
	return leafCounts
}

// matcherCounts runs the real matcher over root and aggregates per-name counts,
// the same aggregation coverCapture performs.
func matcherCounts(corpus *sightmap.Corpus, root *sightmap.ComponentNode, route string) map[string]int {
	counts := make(map[string]int)
	for _, m := range match.NewMatcher(corpus).Match(root, route) {
		if m != nil {
			counts[m.Name]++
		}
	}
	return counts
}

func captureStdout(fn func()) string {
	orig := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	done := make(chan string)
	go func() {
		var b bytes.Buffer
		_, _ = io.Copy(&b, r)
		done <- b.String()
	}()
	fn()
	_ = w.Close()
	os.Stdout = orig
	return <-done
}
