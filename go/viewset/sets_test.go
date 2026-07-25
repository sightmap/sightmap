package viewset

import (
	"testing"
	"time"
)

func TestSnapshotStamp(t *testing.T) {
	// A fixed instant in a non-UTC zone must render as UTC basic ISO-8601.
	loc := time.FixedZone("UTC-5", -5*3600)
	at := time.Date(2026, 6, 7, 14, 30, 0, 0, loc) // 19:30:00 UTC
	if got, want := Stamp(at), "20260607T193000Z"; got != want {
		t.Fatalf("Stamp = %q, want %q", got, want)
	}
	// Lexical order must equal chronological order.
	earlier := Stamp(time.Date(2026, 6, 7, 9, 0, 0, 0, time.UTC))
	later := Stamp(time.Date(2026, 6, 7, 19, 30, 0, 0, time.UTC))
	if !(earlier < later) {
		t.Fatalf("stamp lexical order broken: %q !< %q", earlier, later)
	}
}

func TestViewSnapshotPath(t *testing.T) {
	at := time.Date(2026, 6, 7, 19, 30, 0, 0, time.UTC)
	got := CapturePath(".sightmap", "home", at)
	want := ".sightmap/snapshots/home/20260607T193000Z.snap"
	if got != want {
		t.Fatalf("CapturePath = %q, want %q", got, want)
	}
}

func TestGroupSnapshotsByView(t *testing.T) {
	files := []string{
		// A view set, given out of order + with a redundant .tree.json sibling.
		".sightmap/snapshots/home/20260607T193000Z.snap",
		".sightmap/snapshots/home/20260607T090000Z.snap",
		".sightmap/snapshots/home/20260607T193000Z.snap.tree.json",
		// An old 3-segment (state) capture under the same view collapses into it.
		".sightmap/snapshots/home/apron/20260607T120000Z.snap",
		// A different view.
		".sightmap/snapshots/plp/20260601T120000Z.snap",
		// Noise that must be skipped.
		"notes.txt",
	}
	sets := GroupByView(files)

	home := sets["home"]
	if len(home) != 3 {
		t.Fatalf("home set size = %d, want 3 (tree.json sibling collapses; old state folds in)", len(home))
	}
	// Ordered oldest → newest by stamp.
	if home[0].Stamp != "20260607T090000Z" || home[2].Stamp != "20260607T193000Z" {
		t.Errorf("home order = [%q, %q, %q], want [09:00, 12:00, 19:30]", home[0].Stamp, home[1].Stamp, home[2].Stamp)
	}
	if home[2].Path != ".sightmap/snapshots/home/20260607T193000Z.snap" {
		t.Errorf("home newest path = %q (should be the .snap, not .tree.json)", home[2].Path)
	}
	if got := sets["plp"]; len(got) != 1 {
		t.Errorf("plp set size = %d, want 1", len(got))
	}
	if _, bad := sets["notes"]; bad {
		t.Errorf("non-snapshot path leaked into sets")
	}
}

func TestUnionPresence(t *testing.T) {
	// A 2-capture set where each load omits a different section (the HD-home
	// non-determinism case): Endcap renders only in the older snap, Recs only in
	// the newer one, Header in both, and Footer in neither.
	caps := []MatchCounts{
		{Stamp: "20260607T090000Z", Counts: map[string]int{"Header": 1, "Endcap": 4, "GlobalNav": 7}},
		{Stamp: "20260607T193000Z", Counts: map[string]int{"Header": 1, "Recs": 6}},
	}
	inventory := []string{"Header", "Endcap", "Recs", "Footer"}

	pres := UnionPresence(inventory, caps)

	// Sorted by name: Endcap, Footer, Header, Recs.
	by := map[string]ComponentPresence{}
	for _, p := range pres {
		by[p.Name] = p
	}
	if len(pres) != 4 {
		t.Fatalf("got %d presences, want 4: %+v", len(pres), pres)
	}
	if pres[0].Name != "Endcap" || pres[3].Name != "Recs" {
		t.Errorf("result not sorted by name: %v", []string{pres[0].Name, pres[1].Name, pres[2].Name, pres[3].Name})
	}

	// Header: alive in both — not dead, not partial.
	if h := by["Header"]; h.MatchedIn != 2 || h.Captures != 2 || h.TotalNodes != 2 || h.LastMatchedStamp != "20260607T193000Z" {
		t.Errorf("Header = %+v, want MatchedIn=2 Captures=2 TotalNodes=2 Last=19:30", h)
	}
	// Endcap: present only in the older snap — partial, NOT dead. Recency = its snap.
	if e := by["Endcap"]; e.MatchedIn != 1 || e.TotalNodes != 4 || e.LastMatchedStamp != "20260607T090000Z" {
		t.Errorf("Endcap = %+v, want MatchedIn=1 TotalNodes=4 Last=09:00", e)
	}
	// Recs: present only in the newer snap — partial, NOT dead.
	if r := by["Recs"]; r.MatchedIn != 1 || r.TotalNodes != 6 || r.LastMatchedStamp != "20260607T193000Z" {
		t.Errorf("Recs = %+v, want MatchedIn=1 TotalNodes=6 Last=19:30", r)
	}
	// Footer: matched in neither — union-dead.
	if f := by["Footer"]; f.MatchedIn != 0 || f.TotalNodes != 0 || f.LastMatchedStamp != "" {
		t.Errorf("Footer = %+v, want MatchedIn=0 TotalNodes=0 Last=\"\"", f)
	}
}

func TestUnionPresenceSingleUntimestamped(t *testing.T) {
	// A single legacy (untimestamped) capture behaves like the old per-snap check:
	// present components are alive (no recency stamp), absent ones are dead.
	caps := []MatchCounts{
		{Stamp: "", Counts: map[string]int{"Header": 2}},
	}
	pres := UnionPresence([]string{"Header", "Footer"}, caps)
	by := map[string]ComponentPresence{}
	for _, p := range pres {
		by[p.Name] = p
	}
	if h := by["Header"]; h.MatchedIn != 1 || h.Captures != 1 || h.LastMatchedStamp != "" {
		t.Errorf("Header = %+v, want MatchedIn=1 Captures=1 Last=\"\"", h)
	}
	if f := by["Footer"]; f.MatchedIn != 0 {
		t.Errorf("Footer = %+v, want MatchedIn=0 (dead)", f)
	}
}

func TestComputeNovelty(t *testing.T) {
	slots := func(matched []string, orphans map[string]int) Slots {
		m := map[string]bool{}
		for _, c := range matched {
			m[c] = true
		}
		if orphans == nil {
			orphans = map[string]int{}
		}
		return Slots{Matched: m, Orphans: orphans}
	}

	// Candidate uniquely renders DigitalEndcap (a new component type) and has a
	// new orphan slot; Header + the existing orphan are already in the union.
	cand := slots(
		[]string{"Header", "DigitalEndcap", "ProductPod"},
		map[string]int{`button @ div[data-testid="endcap"]`: 3, `link @ (no stable ancestor)`: 1},
	)
	others := []Slots{
		slots([]string{"Header", "ProductPod"}, map[string]int{`link @ (no stable ancestor)`: 1}),
		slots([]string{"Header"}, nil),
	}

	res := ComputeNovelty(cand, others)
	if !res.IsNovel() {
		t.Fatalf("expected novel, got %+v", res)
	}
	if res.ComparedTo != 2 {
		t.Errorf("ComparedTo = %d, want 2", res.ComparedTo)
	}
	if len(res.NovelComponents) != 1 || res.NovelComponents[0] != "DigitalEndcap" {
		t.Errorf("NovelComponents = %v, want [DigitalEndcap]", res.NovelComponents)
	}
	if got, ok := res.NovelOrphans[`button @ div[data-testid="endcap"]`]; !ok || got != 3 {
		t.Errorf("novel orphan endcap button = (%d,%v), want (3,true)", got, ok)
	}
	if _, ok := res.NovelOrphans[`link @ (no stable ancestor)`]; ok {
		t.Errorf("the existing orphan slot must not be reported as novel")
	}

	// A redundant re-capture (subset of the union) is not novel.
	dup := slots([]string{"Header", "ProductPod"}, map[string]int{`link @ (no stable ancestor)`: 5})
	if r := ComputeNovelty(dup, others); r.IsNovel() {
		t.Errorf("redundant capture flagged novel: %+v", r)
	}

	// First capture (no others): everything is novel vs the empty union, and
	// ComparedTo==0 lets the caller treat it as the keep-by-default first capture.
	if r := ComputeNovelty(cand, nil); r.ComparedTo != 0 || len(r.NovelComponents) != 3 || len(r.NovelOrphans) != 2 {
		t.Errorf("first capture: got %+v, want ComparedTo=0 with all slots novel", r)
	}
}

func TestPlanPrune(t *testing.T) {
	slots := func(matched ...string) Slots {
		m := map[string]bool{}
		for _, c := range matched {
			m[c] = true
		}
		return Slots{Matched: m, Orphans: map[string]int{}}
	}

	// [0] reduced (subset of [1]), [1] full, [2] duplicate of [1].
	// Expect: [0] subsumed by [1]; one of the [1]/[2] duplicates dropped; the
	// union (Header, Hero, Endcap) preserved by the survivor.
	caps := []Slots{
		slots("Header", "Hero"),
		slots("Header", "Hero", "Endcap"),
		slots("Header", "Hero", "Endcap"),
	}
	prune := PlanPrune(caps)
	if len(prune) != 2 {
		t.Fatalf("PlanPrune dropped %d, want 2 (reduced + one duplicate): %v", len(prune), prune)
	}
	// The survivor must still cover the full union.
	kept := map[int]bool{0: true, 1: true, 2: true}
	for _, i := range prune {
		delete(kept, i)
	}
	if len(kept) != 1 {
		t.Fatalf("expected exactly 1 survivor, kept=%v", kept)
	}
	for i := range kept {
		if !caps[i].Matched["Endcap"] {
			t.Errorf("survivor %d must retain Endcap (the union must not shrink)", i)
		}
	}

	// Two captures each contributing a unique component — nothing prunable.
	distinct := []Slots{slots("A", "B"), slots("A", "C")}
	if p := PlanPrune(distinct); len(p) != 0 {
		t.Errorf("PlanPrune pruned %v from a non-redundant set, want none", p)
	}

	// A single capture is never pruned.
	if p := PlanPrune([]Slots{slots("A")}); len(p) != 0 {
		t.Errorf("PlanPrune dropped the sole capture: %v", p)
	}
}

func TestFormatStampDisplay(t *testing.T) {
	if got := FormatStamp(""); got != "" {
		t.Errorf("empty stamp = %q, want \"\"", got)
	}
	if got, want := FormatStamp("20260607T193000Z"), "2026-06-07 19:30Z"; got != want {
		t.Errorf("FormatStamp = %q, want %q", got, want)
	}
	if got, want := FormatStamp("not-a-stamp"), "not-a-stamp"; got != want {
		t.Errorf("unparseable stamp = %q, want passthrough %q", got, want)
	}
}
