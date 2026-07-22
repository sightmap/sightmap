package match

import (
	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/sel"
)

// MatchQuery pairs a semantic name with a parsed, root-first selector chain.
// Combinators[i] is the combinator that precedes Parts[i]:
//   - Combinators[0] is always ""
//   - Subsequent entries are " " (descendant) or ">" (direct child)
type MatchQuery struct {
	Name        string
	Parts       []*comps.SelectorPart
	Combinators []string
}

// FindAllMatches traverses root depth-first, invoking onMatch for every node
// that satisfies any query in queries. Each node is visited once (O(M) pass
// for M nodes). The onMatch callback may be called more than once per node if
// multiple queries match; call order follows queries slice order.
//
// Frame subtrees (nodes whose IDs carry a frame prefix) are traversed without
// special handling — component IDs are globally unique and require no frame
// boundary logic here.
func FindAllMatches(
	root *comps.ComponentNode,
	queries []MatchQuery,
	onMatch func(*comps.ComponentNode, *MatchQuery),
) {
	if root == nil || len(queries) == 0 {
		return
	}
	findAllMatchesNFA(root, nil, nil, queries, onMatch)
}

// ---- NFA internals ----------------------------------------------------------

// selectorState tracks a single in-flight position in a MatchQuery's selector
// chain: we are looking for Parts[idx] to match the next candidate node.
type selectorState struct {
	q   *MatchQuery
	idx int
}

// findAllMatchesNFA is the recursive DFS core.
//
//	descendant – states that must be checked at this node AND all descendants.
//	direct     – states that must be checked at this node ONLY (direct-child
//	             combinator consumed one level).
func findAllMatchesNFA(
	node *comps.ComponentNode,
	descendant, direct []selectorState,
	queries []MatchQuery,
	onMatch func(*comps.ComponentNode, *MatchQuery),
) {
	// Build de-duplicated set of states to evaluate at this node.
	// Fresh starts ({q, 0}) are added for every query at every node — this is
	// what makes "descendant" semantics work without explicit propagation of
	// the initial part: any node can be the start of a match chain.
	type stateKey struct {
		q   *MatchQuery
		idx int
	}
	seen := make(map[stateKey]bool, len(queries)+len(descendant)+len(direct))
	var toCheck []selectorState

	add := func(s selectorState) {
		k := stateKey{s.q, s.idx}
		if !seen[k] {
			seen[k] = true
			toCheck = append(toCheck, s)
		}
	}

	// Inherited states first: these represent in-progress multi-part selector
	// matches that have already matched ancestor nodes. Giving them priority
	// over fresh starts ensures that a scoped child definition (e.g.
	// "[data-component^=X] a") beats a broad global definition (e.g. just
	// "a") when both could claim the same node — regardless of query order.
	for _, s := range descendant {
		add(s)
	}
	for _, s := range direct {
		add(s)
	}
	// Fresh starts after: try each query from scratch at this node.
	// Listed last so contextually-scoped inherited matches take precedence.
	for i := range queries {
		add(selectorState{&queries[i], 0})
	}

	// nextDescendant begins as a copy of the current descendant set — those
	// states must continue propagating through the subtree regardless of
	// whether they match here. Newly advanced descendant states are appended.
	nextDescendant := make([]selectorState, len(descendant), len(descendant)+8)
	copy(nextDescendant, descendant)

	var nextDirect []selectorState
	matchedQueries := make(map[*MatchQuery]bool, 4)

	for _, state := range toCheck {
		rule := state.q.Parts[state.idx]
		// MatchesNode honors :has() (needs node's subtree); falls back to the
		// flat checks for everything else.
		if !sel.MatchesNode(node, rule) {
			continue
		}

		nextIdx := state.idx + 1
		if nextIdx == len(state.q.Parts) {
			// Terminal — this node fully satisfies the query's selector.
			if !matchedQueries[state.q] {
				matchedQueries[state.q] = true
				onMatch(node, state.q)
			}
			continue
		}

		// Advance to the next part. The combinator before Parts[nextIdx]
		// determines whether the next match must be a direct child (">")
		// or any descendant (" ").
		ns := selectorState{state.q, nextIdx}
		if state.q.Combinators[nextIdx] == ">" {
			nextDirect = appendStateIfNew(nextDirect, ns)
		} else {
			nextDescendant = appendStateIfNew(nextDescendant, ns)
		}
	}

	// Recurse into children. Direct children receive both nextDescendant and
	// nextDirect; deeper descendants receive only nextDescendant (nextDirect
	// is consumed after one hop because those children won't re-include it in
	// their own nextDescendant when recursing further).
	for _, child := range node.Children {
		findAllMatchesNFA(child, nextDescendant, nextDirect, queries, onMatch)
	}
}

// appendStateIfNew appends s to ss only if no existing entry has the same
// (query pointer, part index). Linear scan — acceptable for the small state
// counts typical of sightmap rule sets.
func appendStateIfNew(ss []selectorState, s selectorState) []selectorState {
	for _, existing := range ss {
		if existing.q == s.q && existing.idx == s.idx {
			return ss
		}
	}
	return append(ss, s)
}
