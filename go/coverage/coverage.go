// Package coverage computes T1/T2/T3 interactive-node coverage of a component
// tree against a set of sightmap matches. It is pure: it takes an already-matched
// comps tree and returns structured results (counts, orphan nodes, T2 scopes,
// annotation gaps) with no browser dependency and no text formatting. Callers —
// the snapshot/capture/coverage/report commands and external consumers like
// Subtext — render or act on the data.
//
// The three tiers:
//
//   - T1 (direct): the node itself has a sightmap match.
//   - T2 (scoped): an ancestor has a sightmap match, but the node does not.
//   - T3 (orphan): no sightmap context at any depth.
package coverage

import (
	"math"

	"github.com/sightmap/sightmap/go/sightmap"
)

// Matches maps a component node to the sightmap rule it matched.
type Matches = map[*sightmap.ComponentNode]*sightmap.ComponentMatch

// ParentMap maps each non-root node to its parent.
type ParentMap = map[*sightmap.ComponentNode]*sightmap.ComponentNode

// Options controls how coverage is scored.
type Options struct {
	// VisibleOnly skips hidden/ignored interactive nodes (the default for the
	// CLI). When false, every interactive node is counted.
	VisibleOnly bool
}

// Result is the outcome of scoring a tree. The T3 orphan nodes, T2 scopes, and
// parent map are retained so callers can render cluster traces or derive
// structural fingerprints (e.g. the view-set novelty gate) without re-walking.
type Result struct {
	T1, T2, T3, Total int
	VisibleOnly       bool

	// Orphans are the T3 nodes (interactive, no matched ancestor).
	Orphans []*sightmap.ComponentNode
	// Scopes maps a matched ancestor to its count of T2 children.
	Scopes map[*sightmap.ComponentNode]int
	// ScopeChildren maps a matched ancestor to its T2 children.
	ScopeChildren map[*sightmap.ComponentNode][]*sightmap.ComponentNode
	// ParentMap is the node→parent map built during scoring.
	ParentMap ParentMap
}

// Score classifies every interactive node in the tree into T1/T2/T3.
func Score(root *sightmap.ComponentNode, matches Matches, opts Options) Result {
	res := Result{
		VisibleOnly:   opts.VisibleOnly,
		Scopes:        make(map[*sightmap.ComponentNode]int),
		ScopeChildren: make(map[*sightmap.ComponentNode][]*sightmap.ComponentNode),
		ParentMap:     BuildParentMap(root),
	}
	if root == nil {
		return res
	}
	sightmap.Walk(root, func(n *sightmap.ComponentNode, _ int) bool {
		if !n.IsInteractive {
			return true
		}
		if opts.VisibleOnly && (!n.IsVisible || n.IsIgnored) {
			return true // skip hidden/ignored interactive nodes
		}
		res.Total++
		if matches[n] != nil {
			res.T1++
		} else if anc := nearestMatchedAncestor(n, res.ParentMap, matches); anc != nil {
			res.T2++
			res.Scopes[anc]++
			res.ScopeChildren[anc] = append(res.ScopeChildren[anc], n)
		} else {
			res.T3++
			res.Orphans = append(res.Orphans, n)
		}
		return true
	})
	return res
}

// CountInteractive returns the number of interactive nodes in the tree, applying
// the same visibility filter Score uses. It is the corpus-independent way to ask
// "does this page have any actionable content?" — useful for detecting a blank or
// still-loading page before a corpus is even involved.
func CountInteractive(root *sightmap.ComponentNode, visibleOnly bool) int {
	if root == nil {
		return 0
	}
	n := 0
	sightmap.Walk(root, func(node *sightmap.ComponentNode, _ int) bool {
		if !node.IsInteractive {
			return true
		}
		if visibleOnly && (!node.IsVisible || node.IsIgnored) {
			return true
		}
		n++
		return true
	})
	return n
}

// BuildParentMap returns a map from each non-root node to its parent.
func BuildParentMap(root *sightmap.ComponentNode) ParentMap {
	pm := make(ParentMap)
	if root == nil {
		return pm
	}
	sightmap.Walk(root, func(n *sightmap.ComponentNode, _ int) bool {
		for _, child := range n.Children {
			pm[child] = n
		}
		return true
	})
	return pm
}

// nearestMatchedAncestor returns the nearest ancestor of node that has a
// sightmap match, or nil if none exists.
func nearestMatchedAncestor(node *sightmap.ComponentNode, parentMap ParentMap, matches Matches) *sightmap.ComponentNode {
	curr := parentMap[node]
	for curr != nil {
		if matches[curr] != nil {
			return curr
		}
		curr = parentMap[curr]
	}
	return nil
}

// ViewScopedMatchCount returns the number of matched (T1) nodes whose component
// is NOT a global (chrome) component. Zero while nodes are still covered means
// the page is carried only by global backstops — a clean coverage pass that
// reflects chrome, not a modeled view. Shared by the snapshot/coverage advisory
// and `gap` so the two surfaces agree instead of one showing a false green.
func ViewScopedMatchCount(matches Matches, globalNames map[string]bool) int {
	n := 0
	for _, m := range matches {
		if m != nil && !globalNames[m.Name] {
			n++
		}
	}
	return n
}

// Pct returns a/b as a percentage rounded to the nearest integer (0 when b==0).
func Pct(a, b int) int {
	if b == 0 {
		return 0
	}
	return int(math.Round(float64(a) * 100 / float64(b)))
}

// Empty reports whether no interactive nodes were scored at all. An empty result
// means the page carried no coverage signal — typically a blank or still-loading
// page — and must not be treated as a pass.
func (r Result) Empty() bool { return r.Total == 0 }

// OK reports whether coverage is a clean pass: at least one interactive node was
// scored and none were orphaned (T3). A page with zero interactive nodes is NOT
// a pass — there is nothing to green-light.
func (r Result) OK() bool { return r.Total > 0 && r.T3 == 0 }

// Mark returns the status glyph for a coverage line: "✓" for a clean pass, "✗"
// when there are orphaned (T3) nodes, and "∅" when there were no interactive
// nodes at all (a blank or unrendered page, which is never a pass).
func (r Result) Mark() string {
	switch {
	case r.Empty():
		return "∅"
	case r.T3 == 0:
		return "✓"
	default:
		return "✗"
	}
}
