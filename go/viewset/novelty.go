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
// returns its component-type / orphan-slot fingerprint. Visible nodes only,
// matching the coverage default. ok is false when the capture's .tree.json
// cannot be read or parsed.
func SlotsForCapture(snapPath string, corpus *sightmap.Corpus) (Slots, bool) {
	data, err := os.ReadFile(snapPath + ".tree.json")
	if err != nil {
		return Slots{}, false
	}
	var root sightmap.ComponentNode
	if json.Unmarshal(data, &root) != nil {
		return Slots{}, false
	}
	matches := match.NewMatcher(corpus).Match(&root, RouteOf(snapPath))
	cov := coverage.Score(&root, matches, coverage.Options{VisibleOnly: true})
	return SlotsFromMatch(matches, cov.Orphans, cov.ParentMap), true
}

// ViewSlots re-matches every capture currently in the view's set against the
// current corpus, optionally excluding one path.
//
// excludePath is excluded by IDENTITY, not lexical spelling: both sides are
// normalized to a canonical absolute, cleaned, slash form before comparing, so
// the operator's verbatim candidate argument (absolute vs relative, or with a
// "./" prefix) excludes the same on-disk capture that discovery (Find) emits.
// filepath.Abs does not resolve symlinks, so two captures reached via distinct
// symlink aliases still compare unequal; that is intentional and matches the
// single-file identity the rest of the code assumes.
func ViewSlots(sightmapDir, viewBasename string, corpus *sightmap.Corpus, excludePath string) []Slots {
	all, _ := Find(sightmapDir, nil)
	entries := GroupByView(all)[viewBasename]
	excl := ""
	if excludePath != "" {
		excl = canonicalCapturePath(excludePath)
	}
	var out []Slots
	for _, e := range entries {
		if excl != "" && canonicalCapturePath(e.Path) == excl {
			continue
		}
		if cs, ok := SlotsForCapture(e.Path, corpus); ok {
			cs.Stamp = e.Stamp
			out = append(out, cs)
		}
	}
	return out
}

// canonicalCapturePath returns a canonical absolute, cleaned, slash-separator
// form of path for identity comparison: filepath.Abs resolves a relative path
// against the process working directory (and both e.Path and the operator's
// argument are relative to the same cwd), Clean collapses "./" and "..", and
// ToSlash normalizes OS separators. Returns "" when the path cannot be made
// absolute, in which case ViewSlots leaves it unexcluded rather than risk a
// false-positive exclusion.
func canonicalCapturePath(path string) string {
	a, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(a))
}

// Gate decides whether a freshly extracted capture should be written to its
// view's set. The first capture of a view always writes; force bypasses the
// gate. Returns the novelty result and the write decision.
func Gate(corpus *sightmap.Corpus, sightmapDir, viewBasename string, cand Slots, force bool) (Novelty, bool) {
	others := ViewSlots(sightmapDir, viewBasename, corpus, "")
	res := ComputeNovelty(cand, others)
	return res, force || len(others) == 0 || res.IsNovel()
}
