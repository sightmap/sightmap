package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Multi-snapshot view layout
// ---------------------------------------------------
// A VIEW is a SET of timestamped captures, not a single file:
//
//	snapshots/<view>/<stamp>.snap        (+ .snap.tree.json)
//
// where <stamp> is a filesystem-safe UTC timestamp (snapStampLayout). Real pages
// are non-deterministic (lazy carousels, personalization, rotating promos), so no
// single capture exercises a view's full component structure. Tooling operates
// over the UNION of the view's set. We removed the named
// per-view "state" segment: re-snapping just appends a capture and the novelty
// gate drops redundant ones, so curated state names aren't needed.

// snapStampLayout formats capture timestamps as filesystem-safe UTC basic
// ISO-8601, e.g. 20260607T193000Z. Lexical order == chronological order.
const snapStampLayout = "20060102T150405Z"

// snapshotStamp renders t as a capture-set timestamp segment.
func snapshotStamp(t time.Time) string { return t.UTC().Format(snapStampLayout) }

// viewSnapshotPath returns the path of one timestamped capture in a view's set:
// snapshots/<view>/<stamp>.snap.
func viewSnapshotPath(sightmapDir, viewName string, t time.Time) string {
	return filepath.Join(sightmapDir, "snapshots", viewName, snapshotStamp(t)+".snap")
}

// snapEntry is one capture within a view's set. Stamp is "" for an untimestamped
// legacy capture (e.g. the flat <view>.snap form).
type snapEntry struct {
	Path  string
	Stamp string
}

// formatStampDisplay renders a capture stamp (snapStampLayout) for human output,
// e.g. "2026-06-07 19:30Z". Returns "" for an empty (untimestamped) stamp and
// the raw value if it does not parse.
func formatStampDisplay(stamp string) string {
	if stamp == "" {
		return ""
	}
	t, err := time.Parse(snapStampLayout, stamp)
	if err != nil {
		return stamp
	}
	return t.UTC().Format("2006-01-02 15:04Z")
}

// latestViewSnapshot returns the path of the most recent .snap capture for a view
// — the greatest stamp under snapshots/<view>/ — falling back to the legacy flat
// <view>.snap. ok=false when no capture exists. Used by readers that need ONE
// representative file for a view (report, pick-time hints); union-aware readers
// walk the whole set instead.
func latestViewSnapshot(sightmapDir, viewBasename string) (string, bool) {
	dir := filepath.Join(sightmapDir, "snapshots", viewBasename)
	if entries, err := os.ReadDir(dir); err == nil {
		newest := ""
		for _, e := range entries {
			// ".snap.tree.json" ends in ".json", so this keeps only the .snap files;
			// directories (any stale state subdirs) are skipped.
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".snap") {
				continue
			}
			if e.Name() > newest { // lexical == chronological for snapStampLayout
				newest = e.Name()
			}
		}
		if newest != "" {
			return filepath.Join(dir, newest), true
		}
	}
	if flat := viewBasename + ".snap"; fileExists(flat) {
		return flat, true
	}
	return "", false
}

// viewSnapshotSet returns every .snap capture for a view, ordered oldest→newest by
// stamp — the whole set under snapshots/<viewBasename>/ — falling back to the
// legacy flat <viewBasename>.snap when no set dir exists. Empty when the view has
// no capture. This is the set-aware analogue of latestViewSnapshot, used by readers
// that aggregate over a view's union (report).
func viewSnapshotSet(sightmapDir, viewBasename string) []snapEntry {
	dir := filepath.Join(sightmapDir, "snapshots", viewBasename)
	var entries []snapEntry
	if des, err := os.ReadDir(dir); err == nil {
		for _, e := range des {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".snap") {
				continue
			}
			_, stamp, ok := parseSnapshotPath("snapshots/" + viewBasename + "/" + e.Name())
			if !ok {
				continue
			}
			entries = append(entries, snapEntry{Path: filepath.Join(dir, e.Name()), Stamp: stamp})
		}
	}
	if len(entries) == 0 {
		if flat := viewBasename + ".snap"; fileExists(flat) {
			entries = append(entries, snapEntry{Path: flat, Stamp: ""})
		}
		return entries
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Stamp != entries[j].Stamp {
			return entries[i].Stamp < entries[j].Stamp
		}
		return entries[i].Path < entries[j].Path
	})
	return entries
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// captureMatchCounts is one capture's per-component matched-node counts plus its
// stamp within a view's set. Counts is keyed by component name.
// LeafCounts holds leaf-part-only match counts for components that got 0 full
// matches in this capture — used to distinguish broken selectors (leaf matches but
// full scoped selector doesn't) from genuinely-absent components.
type captureMatchCounts struct {
	Stamp      string
	Counts     map[string]int
	LeafCounts map[string]int // nil when no dead-component probe was performed
}

// componentPresence summarizes how often a view component matched across the
// UNION of a view's snapshot set. A component is "alive" if
// MatchedIn > 0 and "dead" only when MatchedIn == 0 across the whole set, so a
// component absent from a single reduced load is no longer flagged.
// LeafMatchAny is set (only meaningful when MatchedIn==0) when at least one
// capture had nodes matching the component's leaf selector part — indicating a
// broken scoped selector rather than a legitimately-absent component.
type componentPresence struct {
	Name             string // component name
	MatchedIn        int    // captures in which it matched >=1 node
	Captures         int    // size of the set (N)
	TotalNodes       int    // matched nodes summed across the set
	LastMatchedStamp string // stamp of the most-recent capture it matched ("" if none/untimestamped)
	LeafMatchAny     bool   // any capture had leaf-part matches (only set when MatchedIn==0)
}

// unionPresence aggregates per-capture match counts across a state's set into
// per-component presence. components is the view's declared component inventory;
// every listed component appears in the result (MatchedIn == 0 when union-dead)
// so callers can report dead components, while counts for names outside the
// inventory (e.g. globals) are ignored. Result is sorted by component name.
// Stamps are compared lexically, which equals chronological order for
// snapStampLayout.
func unionPresence(components []string, caps []captureMatchCounts) []componentPresence {
	ordered := make([]string, 0, len(components))
	byName := make(map[string]*componentPresence, len(components))
	for _, name := range components {
		if name == "" || byName[name] != nil {
			continue
		}
		p := &componentPresence{Name: name, Captures: len(caps)}
		byName[name] = p
		ordered = append(ordered, name)
	}
	for _, c := range caps {
		for name, cnt := range c.Counts {
			p := byName[name]
			if p == nil || cnt <= 0 {
				continue
			}
			p.MatchedIn++
			p.TotalNodes += cnt
			if c.Stamp >= p.LastMatchedStamp {
				p.LastMatchedStamp = c.Stamp
			}
		}
		// Propagate leaf-match evidence for dead components.
		for name, cnt := range c.LeafCounts {
			p := byName[name]
			if p == nil || cnt <= 0 {
				continue
			}
			p.LeafMatchAny = true
		}
	}
	sort.Strings(ordered)
	out := make([]componentPresence, 0, len(ordered))
	for _, name := range ordered {
		out = append(out, *byName[name])
	}
	return out
}

// captureSlots is the corpus-relative structural fingerprint of one capture used
// by snapshot novelty: the component TYPES it matched and the
// uncovered-interactive SLOTS it left orphaned. Both are computed by re-matching
// the capture against the CURRENT corpus, so a capture's novelty is re-evaluated
// as the corpus matures (which is what lets redundant captures be pruned later).
type captureSlots struct {
	Stamp   string
	Matched map[string]bool // component names matched (>=1 node)
	Orphans map[string]int  // orphan slot key (orphanSlotKey) -> count
}

// noveltyResult is what a candidate capture adds vs the UNION of the existing set
// (the other captures of its (view, state)). A capture is worth keeping iff it is
// novel — i.e. it expands the union with a new component type or a new orphan slot.
type noveltyResult struct {
	ComparedTo      int            // number of existing captures unioned against
	NovelComponents []string       // component types matched here, in no existing capture (sorted)
	NovelOrphans    map[string]int // orphan slot key -> count in candidate, new vs the union
}

// IsNovel reports whether the candidate expands the union at all.
func (r noveltyResult) IsNovel() bool {
	return len(r.NovelComponents) > 0 || len(r.NovelOrphans) > 0
}

// computeNovelty diffs a candidate capture's slots against the union of the other
// captures in its set. Additive-only: it reports what the candidate ADDS, never
// what it lacks (lost-component detection is a separate, deferred problem).
func computeNovelty(cand captureSlots, others []captureSlots) noveltyResult {
	unionMatched := map[string]bool{}
	unionOrphan := map[string]bool{}
	for _, o := range others {
		for c := range o.Matched {
			unionMatched[c] = true
		}
		for k := range o.Orphans {
			unionOrphan[k] = true
		}
	}
	res := noveltyResult{ComparedTo: len(others), NovelOrphans: map[string]int{}}
	for c := range cand.Matched {
		if !unionMatched[c] {
			res.NovelComponents = append(res.NovelComponents, c)
		}
	}
	sort.Strings(res.NovelComponents)
	for k, n := range cand.Orphans {
		if !unionOrphan[k] {
			res.NovelOrphans[k] = n
		}
	}
	return res
}

// planPrune returns the indices (into caps) of captures that are SUBSUMED and can
// be dropped without shrinking the view's union. A capture is
// subsumed when it adds no new component type or orphan slot vs the OTHERS — i.e.
// computeNovelty(c, others) is not novel. Pruning is iterative and conservative:
// each pass drops the first subsumed capture (scanning in the given order, so
// pass oldest-first to retire stale duplicates), then recomputes, so two
// identical captures collapse to one rather than both vanishing. The surviving
// set's union equals the original's.
func planPrune(caps []captureSlots) []int {
	alive := make([]int, len(caps))
	for i := range alive {
		alive[i] = i
	}
	var pruned []int
	for {
		victim := -1
		for pos := range alive {
			if len(alive) <= 1 {
				break // never prune the last capture
			}
			others := make([]captureSlots, 0, len(alive)-1)
			for p2, idx2 := range alive {
				if p2 != pos {
					others = append(others, caps[idx2])
				}
			}
			if !computeNovelty(caps[alive[pos]], others).IsNovel() {
				victim = pos
				break
			}
		}
		if victim < 0 {
			break
		}
		pruned = append(pruned, alive[victim])
		alive = append(alive[:victim], alive[victim+1:]...)
	}
	return pruned
}

// groupSnapshotsByView groups snapshot file paths into per-view sets, each ordered
// oldest→newest by capture stamp (untimestamped legacy captures, stamp "", sort
// first). Non-snapshot paths are skipped. Accepts either .snap or .snap.tree.json
// paths; the .snap path is canonical, so a .snap and its .tree.json sibling
// collapse to one entry.
func groupSnapshotsByView(files []string) map[string][]snapEntry {
	sets := make(map[string][]snapEntry)
	seen := make(map[string]bool)
	for _, f := range files {
		view, stamp, ok := parseSnapshotPath(f)
		if !ok {
			continue
		}
		// Normalize to the .snap path so a .snap + its .tree.json don't double-count.
		snapPath := strings.TrimSuffix(filepath.ToSlash(f), ".tree.json")
		if seen[snapPath] {
			continue
		}
		seen[snapPath] = true
		sets[view] = append(sets[view], snapEntry{Path: snapPath, Stamp: stamp})
	}
	for view, entries := range sets {
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Stamp != entries[j].Stamp {
				return entries[i].Stamp < entries[j].Stamp
			}
			return entries[i].Path < entries[j].Path
		})
		sets[view] = entries
	}
	return sets
}
