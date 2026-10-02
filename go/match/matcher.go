package match

import (
	"slices"

	"github.com/sightmap/sightmap/go/sightmap"
)

// MatchQuery pairs a semantic name with a parsed, root-first selector chain.
// Combinators[i] is the combinator that precedes Parts[i]:
//   - Combinators[0] is always ""
//   - Subsequent entries are " " (descendant) or ">" (direct child)
type MatchQuery struct {
	Name        string
	Parts       []*sightmap.SelectorPart
	Combinators []string
	// Def is the component definition this query was compiled from — the precise
	// def for this match (component names are unique only per parent, so a name
	// lookup would collide). Consumers read Memory/Tags/Properties from it.
	Def *sightmap.ComponentDef
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
	root *sightmap.ComponentNode,
	queries []MatchQuery,
	onMatch func(*sightmap.ComponentNode, *MatchQuery),
) {
	findAllMatches(root, queries, newFirstPartIndex(queries), onMatch)
}

// findAllMatches is FindAllMatches with a prebuilt index, so a Matcher can
// build the index once per compiled query set rather than once per call.
func findAllMatches(
	root *sightmap.ComponentNode,
	queries []MatchQuery,
	index *firstPartIndex,
	onMatch func(*sightmap.ComponentNode, *MatchQuery),
) {
	if root == nil || len(queries) == 0 {
		return
	}
	w := &nfaWalker{
		queries: queries,
		index:   index,
		onMatch: onMatch,
		chain:   []*sightmap.ComponentNode{root},
	}
	w.visit(nil, nil)
}

// ---- NFA internals ----------------------------------------------------------

// selectorState tracks a single in-flight position in a MatchQuery's selector
// chain: we are looking for Parts[idx] to match the next candidate node.
type selectorState struct {
	q   *MatchQuery
	idx int
}

// nfaWalker holds the state shared across one FindAllMatches traversal.
type nfaWalker struct {
	queries []MatchQuery
	index   *firstPartIndex
	onMatch func(*sightmap.ComponentNode, *MatchQuery)
	// chain is the root-first path to the node being visited, pushed and popped
	// as the walk descends. MatchesNodeChain needs the ancestor path to evaluate a
	// :not() argument that carries a combinator.
	chain []*sightmap.ComponentNode
	// fresh is scratch space for the current node's fresh-start candidates. It is
	// fully consumed before the walk recurses, so one buffer serves every node.
	fresh []int
}

// visit is the recursive DFS core for the last node of w.chain.
//
//	descendant – states that must be checked at this node AND all descendants.
//	direct     – states that must be checked at this node ONLY (direct-child
//	             combinator consumed one level).
//
// Neither slice is modified: siblings share them.
func (w *nfaWalker) visit(descendant, direct []selectorState) {
	chain := w.chain
	node := chain[len(chain)-1]

	// nextDescendant starts as the inherited descendant set — those states keep
	// propagating through the subtree whether or not they match here. The capped
	// capacity makes the first append copy, so the parent's slice is never written.
	nextDescendant := descendant[:len(descendant):len(descendant)]
	var nextDirect []selectorState

	// Inherited states first: these represent in-progress multi-part selector
	// matches that have already matched ancestor nodes. Giving them priority
	// over fresh starts ensures that a scoped child definition (e.g.
	// "[data-component^=X] a") beats a broad global definition (e.g. just
	// "a") when both could claim the same node — regardless of query order.
	for _, s := range descendant {
		nextDescendant, nextDirect = w.step(chain, s, nextDescendant, nextDirect)
	}
	for _, s := range direct {
		if !slices.Contains(descendant, s) {
			nextDescendant, nextDirect = w.step(chain, s, nextDescendant, nextDirect)
		}
	}
	// Fresh starts after, in query order: any node can begin a match chain. Only
	// queries whose first part this node could satisfy are tried. Inherited states
	// are never at index 0, so a fresh start never duplicates one.
	w.fresh = w.index.candidates(node.Element, w.fresh[:0])
	for _, qi := range w.fresh {
		nextDescendant, nextDirect = w.step(chain, selectorState{&w.queries[qi], 0}, nextDescendant, nextDirect)
	}

	// Recurse into children. Direct children receive both nextDescendant and
	// nextDirect; deeper descendants receive only nextDescendant (nextDirect
	// is consumed after one hop because those children won't re-include it in
	// their own nextDescendant when recursing further).
	for _, child := range node.Children {
		w.chain = append(w.chain, child)
		w.visit(nextDescendant, nextDirect)
		w.chain = w.chain[:len(w.chain)-1]
	}
}

// step evaluates one state at the last node of chain. A completed selector is
// reported through onMatch; a partially matched one advances into the
// descendant or direct set for the node's children. States are unique per
// (query, index) and each query has a single terminal index, so a query is
// reported at most once per node.
func (w *nfaWalker) step(chain []*sightmap.ComponentNode, s selectorState, descendant, direct []selectorState) ([]selectorState, []selectorState) {
	// MatchesNodeChain honors :has() (node's subtree) and a combinator-bearing
	// :not() (node's ancestor chain); falls back to the flat checks otherwise.
	if !sightmap.MatchesNodeChain(chain, s.q.Parts[s.idx]) {
		return descendant, direct
	}
	nextIdx := s.idx + 1
	if nextIdx == len(s.q.Parts) {
		w.onMatch(chain[len(chain)-1], s.q)
		return descendant, direct
	}
	// The combinator before Parts[nextIdx] determines whether the next match
	// must be a direct child (">") or any descendant (" ").
	ns := selectorState{s.q, nextIdx}
	if s.q.Combinators[nextIdx] == ">" {
		direct = appendStateIfNew(direct, ns)
	} else {
		descendant = appendStateIfNew(descendant, ns)
	}
	return descendant, direct
}

// appendStateIfNew appends s to ss only if no existing entry has the same
// (query pointer, part index). Linear scan — acceptable for the small state
// counts typical of sightmap rule sets.
func appendStateIfNew(ss []selectorState, s selectorState) []selectorState {
	if slices.Contains(ss, s) {
		return ss
	}
	return append(ss, s)
}
