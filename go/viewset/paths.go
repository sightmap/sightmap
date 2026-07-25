// Package viewset manages a view's on-disk capture set: the snapshots/<view>/
// <stamp>.snap layout, capture discovery and grouping, the corpus-relative
// novelty gate and prune planner, and per-view presence aggregation. A view is a
// SET of timestamped captures (real pages are non-deterministic), and tooling
// operates over the union of that set.
package viewset

import (
	"path/filepath"
	"strings"
)

// TreePath returns the tree JSON path for a given snapshot path.
// Example: ".sightmap/snapshots/app-home/base.snap" → ".sightmap/snapshots/app-home/base.snap.tree.json"
func TreePath(snapPath string) string {
	return snapPath + ".tree.json"
}

// ParsePath extracts the view name and capture stamp from a snapshot path
// (or its .snap.tree.json sibling). Since we dropped the named-state
// segment, a capture lives at snapshots/<view>/<stamp>.snap: the view is the first
// segment under snapshots/ and the stamp is the last. Legacy forms still parse —
// a flat <view>.snap, and the old single-file / <view>/<state>/<stamp> layouts —
// by collapsing to (view, last-segment).
//
//	snapshots/app-home/20260607T193000Z.snap → ("app-home", "20260607T193000Z", true)
//	snapshots/app-home/base.snap              → ("app-home", "base", true)
//	app-home.snap (flat)                      → ("app-home", "", true)
func ParsePath(path string) (view, stamp string, ok bool) {
	p := filepath.ToSlash(path)
	p = strings.TrimSuffix(p, ".tree.json")
	if !strings.HasSuffix(p, ".snap") {
		return "", "", false
	}
	p = strings.TrimSuffix(p, ".snap")

	var remainder string
	switch {
	case strings.Contains(p, "/snapshots/"):
		remainder = p[strings.LastIndex(p, "/snapshots/")+len("/snapshots/"):]
	case strings.HasPrefix(p, "snapshots/"):
		remainder = strings.TrimPrefix(p, "snapshots/")
	default:
		return filepath.Base(p), "", true // legacy flat <view>.snap
	}

	segs := strings.Split(remainder, "/")
	if len(segs) == 0 || segs[0] == "" {
		return "", "", false
	}
	view = segs[0]
	if len(segs) >= 2 {
		stamp = segs[len(segs)-1] // last segment; any middle "state" collapses away
	}
	return view, stamp, true
}
