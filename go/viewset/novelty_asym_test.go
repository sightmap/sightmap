package viewset

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sightmap/sightmap/go/coverage"
	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// TestGateIncludeHiddenAsymmetry pins the contract that the capture-time
// novelty gate refuses a structurally-duplicate capture even when the live
// capture was scored with --include-hidden (VisibleOnly:false). Before the
// live capture's visibility was threaded into SlotsForCapture, the on-disk
// fingerprint was always built with VisibleOnly:true, so a hidden-only orphan
// slot (off-screen tray, A/B-hidden variant) could never appear in the on-disk
// union — every re-capture read it as endlessly novel and the gate wrote a
// duplicate. The fix threads the live capture's VisibleOnly through
// Gate/ViewSlots/SlotsForCapture so both sides are scored under the same filter.
func TestGateIncludeHiddenAsymmetry(t *testing.T) {
	corpus := &sightmap.Corpus{}

	// A page with two distinct orphan slots: a VISIBLE button inside
	// div[data-testid="main"] and a HIDDEN button inside
	// div[data-testid="promo-tray"]. With an empty corpus neither has a
	// matched ancestor, so both are T3 orphans — but only when the visibility
	// filter admits the hidden one.
	root := &sightmap.ComponentNode{
		Role: "generic",
		Children: []*sightmap.ComponentNode{
			{
				Role: "main",
				Element: &sightmap.Element{
					Tag:   "div",
					Attrs: map[string]string{"data-testid": "main"},
				},
				Children: []*sightmap.ComponentNode{
					{
						Role:          "button",
						IsInteractive: true,
						IsVisible:     true,
						Element:       &sightmap.Element{Tag: "button"},
					},
				},
			},
			{
				Role: "section",
				Element: &sightmap.Element{
					Tag:   "div",
					Attrs: map[string]string{"data-testid": "promo-tray"},
				},
				Children: []*sightmap.ComponentNode{
					{
						Role:          "button",
						IsInteractive: true,
						IsVisible:     false, // hidden orphan: off-screen tray / A/B-hidden variant
						Element:       &sightmap.Element{Tag: "button"},
					},
				},
			},
		},
	}

	dir := filepath.Join(t.TempDir(), ".sightmap")
	if err := writeTreeCapture(dir, "home", "20260607T000000Z", "/home", root); err != nil {
		t.Fatalf("write capture: %v", err)
	}

	const (
		visibleSlot = `button @ div[data-testid="main"]`
		hiddenSlot  = `button @ div[data-testid="promo-tray"]`
	)

	// ── SlotsForCapture threads visibleOnly ─────────────────────────────────
	t.Run("SlotsForCapture-honors-visibleOnly", func(t *testing.T) {
		snap := filepath.Join(dir, "snapshots", "home", "20260607T000000Z.snap")

		// VisibleOnly:true (the default / non-include-hidden path): the hidden
		// orphan is filtered out, so only the visible slot is in the fingerprint.
		vis, ok := SlotsForCapture(snap, corpus, true)
		if !ok {
			t.Fatal("SlotsForCapture visible failed to read capture")
		}
		if _, has := vis.Orphans[hiddenSlot]; has {
			t.Errorf("VisibleOnly=true leaked the hidden orphan slot: %v", vis.Orphans)
		}
		if _, has := vis.Orphans[visibleSlot]; !has {
			t.Errorf("VisibleOnly=true dropped the visible orphan slot: %v", vis.Orphans)
		}

		// VisibleOnly:false (--include-hidden): the hidden orphan survives the
		// filter and must appear in the fingerprint. Before the fix this branch
		// returned the same (visible-only) fingerprint as above, hiding the
		// hidden slot from the offline path.
		hid, ok := SlotsForCapture(snap, corpus, false)
		if !ok {
			t.Fatal("SlotsForCapture hidden failed to read capture")
		}
		if _, has := hid.Orphans[hiddenSlot]; !has {
			t.Errorf("VisibleOnly=false dropped the hidden orphan slot: %v", hid.Orphans)
		}
		if _, has := hid.Orphans[visibleSlot]; !has {
			t.Errorf("VisibleOnly=false dropped the visible orphan slot: %v", hid.Orphans)
		}
	})

	// ── --include-hidden: a duplicate re-capture is refused ─────────────────
	t.Run("include-hidden-refuses-duplicate", func(t *testing.T) {
		// The live candidate is the SAME page, scored with VisibleOnly:false
		// (--include-hidden), exactly as cmd_capture.go builds cand.
		matches := match.NewMatcher(corpus).Match(root, "/home")
		covCand := coverage.Score(root, matches, coverage.Options{VisibleOnly: false})
		cand := SlotsFromMatch(matches, covCand.Orphans, covCand.ParentMap)

		if _, has := cand.Orphans[hiddenSlot]; !has {
			t.Fatalf("candidate missing hidden orphan slot; fixture wrong: %v", cand.Orphans)
		}

		// Before the fix, the on-disk union was always VisibleOnly:true, the
		// hidden slot was absent from it, and ComputeNovelty reported the hidden
		// slot novel — so Gate wrote a structurally-duplicate capture. After the
		// fix, Gate threads cand's VisibleOnly=false into the offline union, the
		// hidden slot is present in others, and the gate refuses the duplicate.
		res, write := Gate(corpus, dir, "home", cand, false, false)
		if write {
			t.Fatalf("Gate wrote a structurally-duplicate capture under --include-hidden: %+v", res)
		}
		if res.IsNovel() {
			t.Fatalf("duplicate capture flagged novel under --include-hidden: %+v", res)
		}
		if res.ComparedTo != 1 {
			t.Errorf("ComparedTo = %d, want 1 (one existing capture)", res.ComparedTo)
		}
	})

	// ── Default (visible-only) control: a duplicate is refused too ──────────
	t.Run("default-aligned-refuses-duplicate", func(t *testing.T) {
		// Same page scored with the default VisibleOnly:true — the live path's
		// behaviour when --include-hidden is off. Both sides agree (visible
		// only), the gate must refuse the duplicate. This is the regression
		// guard that the alignment fix must not perturb the default path.
		matches := match.NewMatcher(corpus).Match(root, "/home")
		covCand := coverage.Score(root, matches, coverage.Options{VisibleOnly: true})
		cand := SlotsFromMatch(matches, covCand.Orphans, covCand.ParentMap)

		if _, has := cand.Orphans[hiddenSlot]; has {
			t.Fatalf("VisibleOnly=true candidate leaked the hidden slot; fixture wrong: %v", cand.Orphans)
		}

		res, write := Gate(corpus, dir, "home", cand, true, false)
		if write {
			t.Fatalf("Gate wrote a duplicate capture on the default visible-only path: %+v", res)
		}
		if res.IsNovel() {
			t.Fatalf("duplicate capture flagged novel on the default path: %+v", res)
		}
	})

	// ── --force still bypasses the gate under --include-hidden ──────────────
	t.Run("include-hidden-force-bypasses", func(t *testing.T) {
		matches := match.NewMatcher(corpus).Match(root, "/home")
		covCand := coverage.Score(root, matches, coverage.Options{VisibleOnly: false})
		cand := SlotsFromMatch(matches, covCand.Orphans, covCand.ParentMap)

		if _, write := Gate(corpus, dir, "home", cand, false, true); !write {
			t.Error("--force did not bypass the novelty gate under --include-hidden")
		}
	})
}

// TestGenuineHiddenNovelty pins guarantee G5: alignment must NOT silence real new
// signal. A candidate that introduces a NEW hidden-only orphan slot — one that is
// absent from the on-disk union — must still be reported novel and written under
// --include-hidden. The fix only stops a hidden slot already present in the
// on-disk union from reading as endlessly novel; it must not collapse genuinely
// distinct hidden slots out of the novelty signal.
func TestGenuineHiddenNovelty(t *testing.T) {
	corpus := &sightmap.Corpus{}

	// On-disk baseline: a page with only a VISIBLE orphan in div[data-testid="main"].
	baseline := &sightmap.ComponentNode{
		Role: "generic",
		Children: []*sightmap.ComponentNode{
			{
				Element: &sightmap.Element{Tag: "div", Attrs: map[string]string{"data-testid": "main"}},
				Children: []*sightmap.ComponentNode{{
					Role: "button", IsInteractive: true, IsVisible: true,
					Element: &sightmap.Element{Tag: "button"},
				}},
			},
		},
	}
	dir := filepath.Join(t.TempDir(), ".sightmap")
	if err := writeTreeCapture(dir, "home", "20260607T000000Z", "/home", baseline); err != nil {
		t.Fatalf("write baseline: %v", err)
	}

	// Live candidate: the same visible slot PLUS a NEW hidden-only orphan in a
	// distinct tray that was not in the baseline. Scored with VisibleOnly:false
	// (--include-hidden), exactly as cmd_capture.go builds cand.
	candRoot := &sightmap.ComponentNode{
		Role: "generic",
		Children: []*sightmap.ComponentNode{
			{
				Element: &sightmap.Element{Tag: "div", Attrs: map[string]string{"data-testid": "main"}},
				Children: []*sightmap.ComponentNode{{
					Role: "button", IsInteractive: true, IsVisible: true,
					Element: &sightmap.Element{Tag: "button"},
				}},
			},
			{
				Element: &sightmap.Element{Tag: "div", Attrs: map[string]string{"data-testid": "new-tray"}},
				Children: []*sightmap.ComponentNode{{
					Role: "button", IsInteractive: true, IsVisible: false,
					Element: &sightmap.Element{Tag: "button"},
				}},
			},
		},
	}
	const newHiddenSlot = `button @ div[data-testid="new-tray"]`
	matches := match.NewMatcher(corpus).Match(candRoot, "/home")
	covCand := coverage.Score(candRoot, matches, coverage.Options{VisibleOnly: false})
	cand := SlotsFromMatch(matches, covCand.Orphans, covCand.ParentMap)
	if _, has := cand.Orphans[newHiddenSlot]; !has {
		t.Fatalf("candidate missing the new hidden slot; fixture wrong: %v", cand.Orphans)
	}

	// Under --include-hidden the gate threads VisibleOnly=false into the on-disk
	// union. The baseline's on-disk tree has no hidden node, so newHiddenSlot is
	// absent from others → genuinely novel → Gate must write.
	res, write := Gate(corpus, dir, "home", cand, false, false)
	if !write {
		t.Fatalf("Gate refused a capture with a genuinely new hidden slot: %+v", res)
	}
	if !res.IsNovel() {
		t.Fatalf("genuine new hidden slot not reported novel: %+v", res)
	}
	if _, has := res.NovelOrphans[newHiddenSlot]; !has {
		t.Errorf("novelty did not list the new hidden slot: %+v", res.NovelOrphans)
	}

	// Control: the same candidate scored with the default VisibleOnly:true drops
	// the hidden slot from cand, so it is NOT novel vs the (visible-only) baseline
	// and must be refused. This shows the fix preserves the user's --include-hidden
	// intent (the hidden slot IS new signal for that user) without leaking it into
	// the default visible-only gate.
	covVis := coverage.Score(candRoot, matches, coverage.Options{VisibleOnly: true})
	candVis := SlotsFromMatch(matches, covVis.Orphans, covVis.ParentMap)
	if _, has := candVis.Orphans[newHiddenSlot]; has {
		t.Fatalf("VisibleOnly=true candidate leaked the hidden slot; fixture wrong: %v", candVis.Orphans)
	}
	if res2, write2 := Gate(corpus, dir, "home", candVis, true, false); write2 {
		t.Fatalf("default-path gate wrote a duplicate despite no visible novelty: %+v", res2)
	}
}

// TestOfflineSymmetryRecovery pins guarantee G7: the two offline commands
// (capture-prune, capture-novelty) score BOTH sides via SlotsForCapture with
// VisibleOnly:true, so the live-vs-offline visibility mismatch that causes this
// bug cannot arise there, and capture-prune remains the recovery path that drops
// the duplicates the pre-fix capture-time gate let in.
//
// We simulate the pre-fix bloat on disk (two identical captures of a page with
// a hidden-only orphan, exactly what the buggy gate wrote every re-capture) and
// show that scoring them through SlotsForCapture(..., true) — as both offline
// commands do — collapses the hidden slot out of every fingerprint, leaving both
// captures structurally identical, so PlanPrune drops one (and ComputeNovelty of
// one vs the other reports "nothing new" / symmetric).
func TestOfflineSymmetryRecovery(t *testing.T) {
	corpus := &sightmap.Corpus{}

	// A page with a hidden-only orphan slot — the shape the pre-fix gate let
	// through repeatedly under --include-hidden.
	page := &sightmap.ComponentNode{
		Role: "generic",
		Children: []*sightmap.ComponentNode{
			{
				Element: &sightmap.Element{Tag: "div", Attrs: map[string]string{"data-testid": "promo-tray"}},
				Children: []*sightmap.ComponentNode{{
					Role: "button", IsInteractive: true, IsVisible: false,
					Element: &sightmap.Element{Tag: "button"},
				}},
			},
		},
	}
	dir := filepath.Join(t.TempDir(), ".sightmap")
	if err := writeTreeCapture(dir, "home", "20260607T000000Z", "/home", page); err != nil {
		t.Fatalf("write first: %v", err)
	}
	if err := writeTreeCapture(dir, "home", "20260607T010000Z", "/home", page); err != nil {
		t.Fatalf("write second: %v", err)
	}

	// capture-prune's pruneView re-materialises each capture via
	// SlotsForCapture(..., true). Under VisibleOnly:true the hidden orphan is
	// filtered out, so both captures share an identical (empty-orphan)
	// fingerprint → PlanPrune drops one.
	snapA := filepath.Join(dir, "snapshots", "home", "20260607T000000Z.snap")
	snapB := filepath.Join(dir, "snapshots", "home", "20260607T010000Z.snap")
	caps := []Slots{}
	for _, p := range []string{snapA, snapB} {
		cs, ok := SlotsForCapture(p, corpus, true)
		if !ok {
			t.Fatalf("SlotsForCapture failed for %s", p)
		}
		caps = append(caps, cs)
	}
	if len(caps[0].Orphans) != 0 || len(caps[1].Orphans) != 0 {
		t.Fatalf("VisibleOnly:true fingerprints must have no hidden orphan: %#v / %#v", caps[0].Orphans, caps[1].Orphans)
	}
	prune := PlanPrune(caps)
	if len(prune) != 1 {
		t.Fatalf("PlanPrune dropped %d, want 1 (recovery: collapse the pre-fix duplicate): %v", len(prune), prune)
	}
	if len(prune) == 1 {
		// The survivor must still cover the union (here empty, trivially true),
		// and removing the duplicate must not change the union — i.e. the
		// surviving capture must not be reported novel vs the pruned one.
		keep := 0
		if prune[0] == 0 {
			keep = 1
		}
		if res := ComputeNovelty(caps[keep], []Slots{caps[prune[0]]}); res.IsNovel() {
			t.Errorf("survivor is novel vs the pruned duplicate — union shrank: %+v", res)
		}
	}

	// capture-novelty's symmetry: candidate and others both via
	// SlotsForCapture(..., true). For two identical captures, comparing one to
	// the other reports "nothing new" — no hidden-only asymmetry can inflate
	// novelty here.
	candSlots, ok := SlotsForCapture(snapA, corpus, true)
	if !ok {
		t.Fatal("SlotsForCapture candidate failed")
	}
	others := ViewSlots(dir, "home", corpus, snapA, true)
	if len(others) != 1 {
		t.Fatalf("ViewSlots(exclude candidate) = %d, want 1: %+v", len(others), others)
	}
	if res := ComputeNovelty(candSlots, others); res.IsNovel() {
		t.Errorf("capture-novelty flagged an identical saved capture novel: %+v", res)
	}
}

// writeTreeCapture persists a real component tree as a timestamped capture in
// a view set: a .snap file carrying the route header (read by RouteOf so the
// offline re-match uses the same page URL) plus its .tree.json sibling carrying
// the serialized tree (read by SlotsForCapture). This is the on-disk shape
// cmd_capture.go's writeCapture produces.
func writeTreeCapture(sightmapDir, viewBasename, stamp, route string, root *sightmap.ComponentNode) error {
	dir := filepath.Join(sightmapDir, "snapshots", viewBasename)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	snap := filepath.Join(dir, stamp+".snap")
	if err := os.WriteFile(snap, []byte("[View: "+viewBasename+"]\nroute: "+route+"\n"), 0o644); err != nil {
		return err
	}
	tree, err := json.Marshal(root)
	if err != nil {
		return err
	}
	return os.WriteFile(snap+".tree.json", tree, 0o644)
}
