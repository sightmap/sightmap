package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/viewset"
)

// pruneCorpus builds a .sightmap/ corpus with the given views, each carrying the
// given capture stamps. Every capture is a minimal (.snap + empty-tree .snap
// .tree.json) pair: the empty tree re-matches the empty corpus to an empty
// fingerprint, so any two captures in a view are interchangeable and the older
// duplicate is subsumed by the newer. Returns the absolute path of the
// .sightmap/ directory.
func pruneCorpus(t *testing.T, views map[string][]string) string {
	t.Helper()
	sdir := filepath.Join(t.TempDir(), ".sightmap")
	for view, stamps := range views {
		for _, stamp := range stamps {
			writePruneCapture(t, sdir, view, stamp)
		}
	}
	return sdir
}

// writePruneCapture writes one timestamped capture (a .snap header plus its
// .snap.tree.json sibling containing {}) into snapshots/<view>/<stamp>.snap.
// The tree's emptiness yields an empty structural fingerprint against an empty
// corpus, matching the pattern used by viewset.writeFakeCapture (go/viewset/
// sets_test.go) so two captures of the same view collapse under PlanPrune.
func writePruneCapture(t *testing.T, sdir, view, stamp string) {
	t.Helper()
	dir := filepath.Join(sdir, "snapshots", view)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	snap := filepath.Join(dir, stamp+".snap")
	body := "[View: " + view + "]\nroute: /" + view + "\n(fake capture)\n"
	if err := os.WriteFile(snap, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", snap, err)
	}
	if err := os.WriteFile(snap+".tree.json", []byte("{}"), 0o644); err != nil {
		t.Fatalf("write %s.tree.json: %v", snap, err)
	}
}

// countSnaps returns the number of .snap captures currently on disk for view.
func countSnaps(t *testing.T, sdir, view string) int {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(sdir, "snapshots", view, "*.snap"))
	if err != nil {
		t.Fatalf("glob %s: %v", view, err)
	}
	return len(matches)
}

// snapExists reports whether the capture <view>/<stamp>.snap is still on disk.
func snapExists(t *testing.T, sdir, view, stamp string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(sdir, "snapshots", view, stamp+".snap"))
	return err == nil
}

// threeViewCorpus is the workhorse fixture: vA, vB, vC each with two captures
// (the older pruneStampA and the newer pruneStampB). Every capture re-matches to
// the same empty fingerprint, so a prune run against any of these views drops
// exactly one capture — the older redundant pruneStampA — and keeps pruneStampB
// (PlanPrune never drops the last capture of a view).
var (
	pruneStampA = "20260101T000000Z"
	pruneStampB = "20260102T000000Z"
)

func threeViewCorpus(t *testing.T) string {
	return pruneCorpus(t, map[string][]string{
		"vA": {pruneStampA, pruneStampB},
		"vB": {pruneStampA, pruneStampB},
		"vC": {pruneStampA, pruneStampB},
	})
}

const pruneUsageErr = "usage: sightmap capture-prune [--dry-run] (<view> | --all)"

// TestCapturePruneAllWithExtraArgs_BugReproduction pins the headline regression:
// `--all` combined with TWO positional view args (a plausible misread of --all
// as "all of these views I'm listing") used to slip past the guard and fire
// `--all` mode, silently pruning views the user never named (vC). The fix makes
// --all mutually exclusive with ANY positional arg, so this invocation must now
// be a hard usage error that prints nothing and touches nothing on disk —
// including the unmentioned vC.
func TestCapturePruneAllWithExtraArgs_BugReproduction(t *testing.T) {
	sdir := threeViewCorpus(t)

	var out bytes.Buffer
	err := runCapturePruneOut([]string{"--sightmap-dir", sdir, "--all", "vA", "vB"}, &out)

	if err == nil {
		t.Fatalf("expected usage error for `--all vA vB`, got nil (no error) and output:\n%s", out.String())
	}
	if !strings.Contains(err.Error(), pruneUsageErr) {
		t.Fatalf("error = %q, want substring %q", err.Error(), pruneUsageErr)
	}
	if got := out.String(); got != "" {
		t.Errorf("usage error should print nothing to stdout, got:\n%s", got)
	}
	for _, view := range []string{"vA", "vB", "vC"} {
		if got := countSnaps(t, sdir, view); got != 2 {
			t.Errorf("view %s: %d .snap file(s) on disk, want 2 (nothing should have been pruned)", view, got)
		}
	}
	// In particular, the unmentioned vC must keep BOTH captures.
	if !snapExists(t, sdir, "vC", pruneStampA) || !snapExists(t, sdir, "vC", pruneStampB) {
		t.Errorf("vC captures were touched by a `--all vA vB` call — the slipped guard is still present")
	}
}

// TestCapturePruneAllWithExtraArgs_SingleArgRejected is the symmetric control:
// `--all vA` (exactly one positional) was ALWAYS rejected by the buggy guard.
// It must remain rejected after the fix — the fix tightens the guard, it does
// not loosen it, and a plausible "simplification" to allow `--all <view>` as a
// shorthand would violate the (<view> | --all) contract.
func TestCapturePruneAllWithExtraArgs_SingleArgRejected(t *testing.T) {
	sdir := threeViewCorpus(t)

	var out bytes.Buffer
	err := runCapturePruneOut([]string{"--sightmap-dir", sdir, "--all", "vA"}, &out)

	if err == nil || !strings.Contains(err.Error(), pruneUsageErr) {
		t.Fatalf("error = %v, want substring %q", err, pruneUsageErr)
	}
	for _, view := range []string{"vA", "vB", "vC"} {
		if got := countSnaps(t, sdir, view); got != 2 {
			t.Errorf("view %s: %d .snap file(s), want 2 (single-arg rejection must not prune)", view, got)
		}
	}
}

// TestCapturePruneAll_HappyPath verifies the legitimate `--all` invocation is
// untouched by the fix: it prunes every view with captures, dropping only the
// redundant (fully-subsumed) older capture and keeping the newer last capture
// (PlanPrune never drops the last capture of a view). A single-capture view is
// left untouched and emits no prune line. On a non-dry-run run the `.snap` and
// its `.snap.tree.json` sibling are both deleted, and the `pruned` verb (not
// `would prune`) is used with no dry-run footer.
func TestCapturePruneAll_HappyPath(t *testing.T) {
	sdir := pruneCorpus(t, map[string][]string{
		"vA":    {pruneStampA, pruneStampB},
		"vB":    {pruneStampA, pruneStampB},
		"vSolo": {pruneStampA}, // single capture — must never be pruned
	})

	var out bytes.Buffer
	if err := runCapturePruneOut([]string{"--sightmap-dir", sdir, "--all"}, &out); err != nil {
		t.Fatalf("runCapturePruneOut --all: %v", err)
	}
	got := out.String()

	for _, view := range []string{"vA", "vB"} {
		if c := countSnaps(t, sdir, view); c != 1 {
			t.Errorf("view %s: %d .snap file(s), want 1 (redundant older capture pruned)", view, c)
		}
		if snapExists(t, sdir, view, pruneStampA) {
			t.Errorf("view %s: older capture %s should have been pruned", view, pruneStampA)
		}
		if !snapExists(t, sdir, view, pruneStampB) {
			t.Errorf("view %s: newer capture %s should have survived as the last capture", view, pruneStampB)
		}
		if !strings.Contains(got, view+": ") || !strings.Contains(got, "keep 1, pruned 1") {
			t.Errorf("output missing %s prune summary line:\n%s", view, got)
		}
		if !strings.Contains(got, "  pruned "+viewset.FormatStamp(pruneStampA)) {
			t.Errorf("output missing the pruned older-stamp line for %s:\n%s", view, got)
		}
	}
	// The single-capture view keeps its sole capture and prints no prune line.
	if c := countSnaps(t, sdir, "vSolo"); c != 1 {
		t.Errorf("vSolo: %d .snap file(s), want 1 (last capture never pruned)", c)
	}
	if strings.Contains(got, "vSolo") {
		t.Errorf("vSolo should emit no prune output (single capture), got:\n%s", got)
	}
	if strings.Contains(got, "dry run") {
		t.Errorf("non-dry-run output should not mention dry run:\n%s", got)
	}
}

// TestCapturePruneSingleView_HappyPath verifies the other legitimate invocation
// — exactly one <view> with no --all: it prunes ONLY that view and leaves every
// other view's captures untouched. This guards against the fix accidentally
// narrowing the single-view path.
func TestCapturePruneSingleView_HappyPath(t *testing.T) {
	sdir := threeViewCorpus(t)

	var out bytes.Buffer
	if err := runCapturePruneOut([]string{"--sightmap-dir", sdir, "vA"}, &out); err != nil {
		t.Fatalf("runCapturePruneOut vA: %v", err)
	}
	got := out.String()

	if c := countSnaps(t, sdir, "vA"); c != 1 {
		t.Errorf("vA: %d .snap file(s), want 1 (pruned)", c)
	}
	if !snapExists(t, sdir, "vA", pruneStampB) {
		t.Errorf("vA: newer capture should survive")
	}
	// vB and vC must be untouched.
	for _, view := range []string{"vB", "vC"} {
		if c := countSnaps(t, sdir, view); c != 2 {
			t.Errorf("%s: %d .snap file(s), want 2 (must NOT be pruned by a single-view call)", view, c)
		}
		if !snapExists(t, sdir, view, pruneStampA) || !snapExists(t, sdir, view, pruneStampB) {
			t.Errorf("%s: both captures must remain on disk", view)
		}
		if strings.Contains(got, view+": ") {
			t.Errorf("output should not mention %s (not the selected view):\n%s", view, got)
		}
	}
	if !strings.Contains(got, "vA: ") || !strings.Contains(got, "keep 1, pruned 1") {
		t.Errorf("output missing vA prune summary:\n%s", got)
	}
}
