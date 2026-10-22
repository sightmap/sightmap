package match

import (
	"sort"

	"github.com/sightmap/sightmap/go/sightmap"
)

// ChainMatch is a single component definition matched at a specific depth along
// an observed element's ancestor chain. Depth is the 0-based index into the
// chain passed to MatchChain: 0 is the root, len(chain)-1 is the observed leaf.
// A chain node may appear in more than one ChainMatch when several component
// definitions match it.
type ChainMatch struct {
	Depth  int
	Name   string
	Tags   []string
	Memory []string
}

// MatchChain resolves the component definitions that apply along a single
// observed element's ancestor chain, root-first (chain[0] is the root, the last
// entry is the observed leaf). It is the streaming-element analogue of Match:
// where Match walks a full component tree, MatchChain evaluates one branch in
// isolation, which is what a consumer classifying a stream of individual
// observed elements has on hand.
//
// Matching is route-aware: pageURL selects the same view-scoped-plus-global
// component set Match would use, so a view-scoped definition applies on the
// chain exactly as it would in a full-tree match. Returns nil when chain is
// empty or no component definitions apply for pageURL.
//
// Each returned ChainMatch is annotated with the Depth of the chain node that
// completed the selector. Results are ordered by ascending depth (root toward
// leaf); within a depth they follow component-definition order. Callers wanting
// the spec's two resolution policies directly should reach for NamesForChain
// (nearest-enclosing) and TagsForChain (union) rather than re-deriving them.
//
// Because the caller supplies only the ancestor chain and not the leaf's own
// subtree, a relational selector that inspects descendants (:has()) on the leaf
// cannot be satisfied here — the same inherent limitation any ancestor-only
// view carries.
func (m *Matcher) MatchChain(chain []sightmap.Element, pageURL string) []ChainMatch {
	out, _ := m.matchChain(chain, pageURL)
	return out
}

// matchChain is MatchChain that also returns the query behind each match,
// index-aligned, for callers that resolve among them.
func (m *Matcher) matchChain(chain []sightmap.Element, pageURL string) ([]ChainMatch, []*MatchQuery) {
	entry := m.entryFor(pageURL)
	if len(chain) == 0 || len(entry.queries) == 0 {
		return nil, nil
	}

	// Build a single-branch spine (root -> leaf) of ComponentNodes so the shared
	// NFA matcher can run over it unchanged. Each node aliases the caller's
	// Element identity; matching only reads it.
	nodes := make([]*sightmap.ComponentNode, len(chain))
	for i := range chain {
		nodes[i] = &sightmap.ComponentNode{Element: &chain[i]}
	}
	depthOf := make(map[*sightmap.ComponentNode]int, len(nodes))
	for i, n := range nodes {
		depthOf[n] = i
		if i+1 < len(nodes) {
			n.Children = []*sightmap.ComponentNode{nodes[i+1]}
		}
	}

	var out []ChainMatch
	var qs []*MatchQuery
	findAllMatches(nodes[0], entry.queries, entry.index, func(node *sightmap.ComponentNode, q *MatchQuery) {
		cm := ChainMatch{Depth: depthOf[node], Name: q.Name}
		if q.Def != nil {
			cm.Tags = q.Def.Tags
			cm.Memory = q.Def.Memory
		}
		out = append(out, cm)
		qs = append(qs, q)
	})
	if len(out) == 0 {
		return nil, nil
	}
	// FindAllMatches visits the linear spine depth-first, so out already runs
	// root -> leaf; a defensive stable sort keeps the contract explicit without
	// disturbing within-depth (definition) order.
	idx := make([]int, len(out))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool { return out[idx[i]].Depth < out[idx[j]].Depth })
	sorted, sortedQs := make([]ChainMatch, len(out)), make([]*MatchQuery, len(out))
	for i, k := range idx {
		sorted[i], sortedQs[i] = out[k], qs[k]
	}
	return sorted, sortedQs
}

// NamesForChain returns the component name(s) that identify the observed leaf
// element, applying the spec's nearest-enclosing rule: the name resolved from
// the deepest chain level that matched. Among several definitions matching
// that level, one strictly more specific than another sees it off (SEP-0023), so
// the result is almost always a single name; more than one is returned only
// when definitions the selector text cannot order match the same deepest
// level (a genuine identity conflict the caller may want to see, rather than a
// silently-picked winner). Names are deduplicated and returned in
// component-definition order. Returns nil when nothing along the chain matched.
func (m *Matcher) NamesForChain(chain []sightmap.Element, pageURL string) []string {
	matches, qs := m.matchChain(chain, pageURL)
	if len(matches) == 0 {
		return nil
	}
	deepest := matches[len(matches)-1].Depth // sorted ascending by depth
	var claims []*MatchQuery
	for i, cm := range matches {
		if cm.Depth == deepest {
			claims = append(claims, qs[i])
		}
	}
	var names []string
	seen := make(map[string]bool)
	for _, q := range m.entryFor(pageURL).narrow.mostSpecific(claims) {
		if !seen[q.Name] {
			seen[q.Name] = true
			names = append(names, q.Name)
		}
	}
	return names
}

// TagsForChain returns the tags that apply to the observed leaf element,
// applying the spec's tag-union rule: the union of tags across every matching
// level of the chain, never narrowed by the nearest-enclosing identity rule. The
// result is deduplicated and lexicographically sorted, per the spec's Tags
// resolution requirement. Returns nil when no matching level carries a tag.
func (m *Matcher) TagsForChain(chain []sightmap.Element, pageURL string) []string {
	matches := m.MatchChain(chain, pageURL)
	set := make(map[string]bool)
	for _, cm := range matches {
		for _, t := range cm.Tags {
			set[t] = true
		}
	}
	if len(set) == 0 {
		return nil
	}
	tags := make([]string, 0, len(set))
	for t := range set {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return tags
}
