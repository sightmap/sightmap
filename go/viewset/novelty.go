package viewset

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/sightmap/sightmap/go/coverage"
	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// SlotsFromMatch builds a capture's structural fingerprint from an already-matched
// tree: the component TYPES matched plus the orphan SLOTS left uncovered. Shared
// by the offline path (SlotsForCapture) and the live capture path (the capture
// command) so both gate on the same notion of "new".
func SlotsFromMatch(
	matches map[*sightmap.ComponentNode]*sightmap.ComponentMatch,
	orphans []*sightmap.ComponentNode,
	parentMap coverage.ParentMap,
) Slots {
	cs := Slots{Matched: map[string]bool{}, Orphans: map[string]int{}}
	for _, m := range matches {
		if m != nil && m.Name != "" {
			cs.Matched[m.Name] = true
		}
	}
	for _, n := range orphans {
		cs.Orphans[coverage.OrphanSlotKey(n, parentMap)]++
	}
	return cs
}

// SlotsForCapture re-matches a saved capture against the current corpus and
// returns its component-type / orphan-slot fingerprint. visibleOnly selects
// the coverage filter the live capture was scored under, so the offline
// fingerprint matches the live one: pass true for the default visible-only
// capture path, false for --include-hidden. ok is false when the capture's
// .tree.json cannot be read or parsed.
func SlotsForCapture(snapPath string, corpus *sightmap.Corpus, visibleOnly bool) (Slots, bool) {
	data, err := os.ReadFile(snapPath + ".tree.json")
	if err != nil {
		return Slots{}, false
	}
	var root sightmap.ComponentNode
	if json.Unmarshal(data, &root) != nil {
		return Slots{}, false
	}
	matches := match.NewMatcher(corpus).Match(&root, RouteOf(snapPath))
	cov := coverage.Score(&root, matches, coverage.Options{VisibleOnly: visibleOnly})
	return SlotsFromMatch(matches, cov.Orphans, cov.ParentMap), true
}

// ViewSlots re-matches every capture currently in the view's set against the
// current corpus, optionally excluding one path. visibleOnly is applied to
// every capture's fingerprint; pass the live capture's filter so the gate
// compares fingerprints built under the same visibility.
func ViewSlots(sightmapDir, viewBasename string, corpus *sightmap.Corpus, excludePath string, visibleOnly bool) []Slots {
	all, _ := Find(sightmapDir, nil)
	entries := GroupByView(all)[viewBasename]
	excl := filepath.ToSlash(excludePath)
	var out []Slots
	for _, e := range entries {
		if excl != "" && filepath.ToSlash(e.Path) == excl {
			continue
		}
		if cs, ok := SlotsForCapture(e.Path, corpus, visibleOnly); ok {
			cs.Stamp = e.Stamp
			out = append(out, cs)
		}
	}
	return out
}

// Gate decides whether a freshly extracted capture should be written to its
// view's set. The first capture of a view always writes; force bypasses the
// gate. visibleOnly is the filter the live candidate was scored under; it is
// threaded into every on-disk fingerprint so a --include-hidden candidate is
// compared against an on-disk union that also admits hidden orphan slots —
// otherwise a hidden-only orphan slot would never appear in the union and read
// as endlessly novel, letting a structurally-duplicate capture through. Returns
// the novelty result and the write decision.
func Gate(corpus *sightmap.Corpus, sightmapDir, viewBasename string, cand Slots, visibleOnly bool, force bool) (Novelty, bool) {
	others := ViewSlots(sightmapDir, viewBasename, corpus, "", visibleOnly)
	res := ComputeNovelty(cand, others)
	return res, force || len(others) == 0 || res.IsNovel()
}
