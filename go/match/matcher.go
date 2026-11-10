package match

import (
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
	// privacyRank and watch mark a capture rule (see capture.go), which declares
	// privacy or watch for the elements it matches and never names one.
	privacyRank int
	watch       bool
}

// FindAllMatches traverses root depth-first, invoking onMatch for every node
// that satisfies any query in queries. Each node is visited once (O(M) pass
// for M nodes). The onMatch callback may be called more than once per node if
// multiple queries match; call order follows queries slice order.
//
// Frame subtrees (nodes whose IDs carry a frame prefix) are traversed without
// special handling — component IDs are globally unique and require no frame
// boundary logic here.
//
// It indexes queries on every call; to walk many trees with one query set,
// prepare it once with Prepare and call Queries.FindAll.
func FindAllMatches(
	root *sightmap.ComponentNode,
	queries []MatchQuery,
	onMatch func(*sightmap.ComponentNode, *MatchQuery),
) {
	findAllMatches(root, queries, newQueryIndex(queries), onMatch)
}

// Queries is a query set indexed once for any number of walks: FindAll is
// FindAllMatches without rebuilding the index, which on a large set costs
// more than walking a page. Safe for concurrent use.
type Queries struct {
	queries []MatchQuery
	index   *queryIndex
}

// Prepare indexes queries for FindAll. The slice must not change afterwards.
func Prepare(queries []MatchQuery) *Queries {
	return &Queries{queries: queries, index: newQueryIndex(queries)}
}

// FindAll is FindAllMatches over the prepared set; onMatch receives pointers
// into the slice given to Prepare.
func (q *Queries) FindAll(root *sightmap.ComponentNode, onMatch func(*sightmap.ComponentNode, *MatchQuery)) {
	findAllMatches(root, q.queries, q.index, onMatch)
}

// findAllMatches is FindAllMatches with a prebuilt index, so a Matcher can
// build the index once per compiled query set rather than once per call.
func findAllMatches(
	root *sightmap.ComponentNode,
	queries []MatchQuery,
	index *queryIndex,
	onMatch func(*sightmap.ComponentNode, *MatchQuery),
) {
	if root == nil || len(queries) == 0 {
		return
	}
	walkMatches(root, queries, index, onMatch, nil)
}

// walkMatches is findAllMatches that also calls onNode, when set, for every
// node in depth-first order, after all of that node's matches and before any
// of its descendants', with the node's depth (root 0). It lets a caller fold
// state down the tree in the same traversal.
func walkMatches(
	root *sightmap.ComponentNode,
	queries []MatchQuery,
	index *queryIndex,
	onMatch func(*sightmap.ComponentNode, *MatchQuery),
	onNode func(*sightmap.ComponentNode, int),
) {
	// The walker's tables are sized to the whole query set, which on a large
	// corpus dwarfs a small tree, so they are reused across walks: a walk
	// leaves inDesc all false, and hasKey and inDir are stamped with a serial
	// that keeps counting up across walks.
	w, _ := index.walkers.Get().(*nfaWalker)
	if w == nil {
		w = &nfaWalker{
			hasKey: make([]uint32, index.keys.n),
			inDesc: make([]bool, len(index.partKey)),
			inDir:  make([]uint32, len(index.partKey)),
		}
	}
	w.queries, w.index, w.onMatch, w.onNode = queries, index, onMatch, onNode
	w.chain = append(w.chain[:0], root)
	w.visit(0, nil)
	w.queries, w.onMatch, w.onNode = nil, nil, nil
	clear(w.chain)
	index.walkers.Put(w)
}

// ---- NFA internals ----------------------------------------------------------

// selectorState tracks a single in-flight position in a MatchQuery's selector
// chain: we are looking for Parts[idx] to match the next candidate node. part is
// the dense id of Parts[idx] in the walk's queryIndex (partBase[query]+idx),
// which keys the walker's per-part tables.
type selectorState struct {
	q    *MatchQuery
	idx  int
	part int32
}

// nfaWalker holds the state shared across one FindAllMatches traversal.
//
// The descendant set of a node is its parent's plus whatever advanced at the
// parent, so the sets along the current path form a stack: desc[:top] is the
// set a node inherits, and states that advance at it are pushed above top for
// its subtree and popped when the walk leaves it. This keeps the order the
// sets always had (inherited first, then new states in discovery order) without
// copying the inherited set at each node.
type nfaWalker struct {
	queries []MatchQuery
	index   *queryIndex
	onMatch func(*sightmap.ComponentNode, *MatchQuery)
	onNode  func(*sightmap.ComponentNode, int)
	// chain is the root-first path to the node being visited, pushed and popped
	// as the walk descends. MatchesNodeChain needs the ancestor path to evaluate a
	// :not() argument that carries a combinator.
	chain []*sightmap.ComponentNode
	// desc is the descendant-state stack (see above).
	desc []selectorState
	// inDesc[part] is set when the state is on the desc stack, so a state
	// that advances again in a subtree where it is already live is not pushed
	// twice.
	inDesc []bool
	// dir holds the direct-child sets of the nodes on the path, each a slice of
	// it, the same stack discipline as desc. inDir[part] == serial marks a state
	// already in the current node's direct set.
	dir   []selectorState
	inDir []uint32
	// hasKey[key] == serial marks a part key the current node meets; serial
	// numbers the visited nodes from 1, so the table never needs clearing.
	hasKey []uint32
	serial uint32
	// keys and fresh are scratch for the current node, fully consumed before
	// the walk recurses, so one buffer serves every node.
	keys  []int32
	fresh []int
}

// visit is the recursive DFS core for the last node of w.chain.
//
//	w.desc[:top] – states that must be checked at this node AND all descendants.
//	direct       – states that must be checked at this node ONLY (direct-child
//	               combinator consumed one level).
//
// Neither is modified: siblings share them.
func (w *nfaWalker) visit(top int, direct []selectorState) {
	chain := w.chain
	node := chain[len(chain)-1]
	w.serial++
	if w.serial == 0 {
		// Wrapped: old stamps could collide with new ones.
		clear(w.hasKey)
		clear(w.inDir)
		w.serial = 1
	}
	serial := w.serial
	w.keys = w.index.keys.nodeKeys(node.Element, w.keys[:0])
	for _, k := range w.keys {
		w.hasKey[k] = serial
	}
	dirBase := len(w.dir)

	// Inherited states first: these represent in-progress multi-part selector
	// matches that have already matched ancestor nodes. Giving them priority
	// over fresh starts ensures that a scoped child definition (e.g.
	// "[data-component^=X] a") beats a broad global definition (e.g. just
	// "a") when both could claim the same node — regardless of query order.
	//
	// A state whose pending part requires a key this node lacks cannot match
	// here, so it is skipped without testing; it stays in the set either way.
	// w.desc may grow while this runs, but only above top.
	for i := 0; i < top; i++ {
		if s := w.desc[i]; w.mayMatch(s, serial) {
			w.step(chain, s, serial)
		}
	}
	// A state is never in both sets: whether Parts[idx] is pending as a direct
	// or a descendant state depends only on Combinators[idx].
	for _, s := range direct {
		if w.mayMatch(s, serial) {
			w.step(chain, s, serial)
		}
	}
	// Fresh starts after, in query order: any node can begin a match chain. Only
	// queries whose first part this node could satisfy are tried. Inherited states
	// are never at index 0, so a fresh start never duplicates one.
	w.fresh = w.index.fresh(w.keys, w.fresh[:0])
	for _, qi := range w.fresh {
		w.step(chain, selectorState{&w.queries[qi], 0, w.index.partBase[qi]}, serial)
	}

	if w.onNode != nil {
		w.onNode(node, len(chain)-1)
	}

	// Recurse into children. Direct children receive both the grown descendant
	// set and this node's direct set; deeper descendants receive only the
	// descendant set (a direct state is consumed after one hop because those
	// children won't re-include it in their own descendant set).
	nextTop := len(w.desc)
	nextDirect := w.dir[dirBase:len(w.dir):len(w.dir)]
	for _, child := range node.Children {
		w.chain = append(w.chain, child)
		w.visit(nextTop, nextDirect)
		w.chain = w.chain[:len(w.chain)-1]
	}

	for _, s := range w.desc[top:] {
		w.inDesc[s.part] = false
	}
	w.desc = w.desc[:top]
	w.dir = w.dir[:dirBase]
}

// mayMatch reports whether s's pending part could match the current node:
// false only when the part requires a key the node lacks.
func (w *nfaWalker) mayMatch(s selectorState, serial uint32) bool {
	for _, k := range w.index.partNeeds[s.part] {
		if w.hasKey[k] != serial {
			return false
		}
	}
	return true
}

// step evaluates one state at the last node of chain. A completed selector is
// reported through onMatch; a partially matched one advances into the
// descendant set (pushed on w.desc) or the node's direct set (on w.dir) for
// its children. States are unique per (query, index) and each query has a
// single terminal index, so a query is reported at most once per node.
func (w *nfaWalker) step(chain []*sightmap.ComponentNode, s selectorState, serial uint32) {
	// MatchesNodeChain honors :has() (node's subtree) and a combinator-bearing
	// :not() (node's ancestor chain); falls back to the flat checks otherwise.
	if pm := w.index.parts[s.part]; pm != nil && !pm.MatchesNodeChain(chain) {
		return
	}
	nextIdx := s.idx + 1
	if nextIdx == len(s.q.Parts) {
		w.onMatch(chain[len(chain)-1], s.q)
		return
	}
	// The combinator before Parts[nextIdx] determines whether the next match
	// must be a direct child (">") or any descendant (" ").
	ns := selectorState{s.q, nextIdx, s.part + 1}
	if s.q.Combinators[nextIdx] == ">" {
		if w.inDir[ns.part] != serial {
			w.inDir[ns.part] = serial
			w.dir = append(w.dir, ns)
		}
	} else if !w.inDesc[ns.part] {
		w.desc = append(w.desc, ns)
		w.inDesc[ns.part] = true
	}
}
