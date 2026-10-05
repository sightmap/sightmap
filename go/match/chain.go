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
	// Privacy is the chain node's effective capture directive (SEP-0009),
	// resolved as Match resolves ComponentMatch.Privacy: the declaration of the
	// nearest enclosing matched component, this node included, with the first
	// matching definition per node deciding. "" when none declares one. Every
	// ChainMatch at one depth carries the same value. Pass it to Withholds to
	// decide whether a value read from this node may be surfaced.
	//
	// A chain carries a node's ancestors but not its subtree, so a definition
	// whose selector uses :has() matches only when the :has() argument lies on
	// the chain. When it does not, that definition's privacy is not applied and
	// Privacy can be less restrictive than Match would resolve on the full tree.
	Privacy string
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
	entry := m.entryFor(pageURL)
	if len(chain) == 0 || len(entry.queries) == 0 {
		return nil
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

	// Privacy needs only the first matching definition per depth (first-match
	// wins, as in Match), and only when some definition declares privacy.
	anyPrivacy := entry.anyPrivacy
	// declared[d] is "" until depth d's first match decides it, then that
	// definition's privacy, or noDeclaration when it declares none.
	const noDeclaration = "\x00"
	var declared []string
	if anyPrivacy {
		declared = make([]string, len(nodes))
	}
	var out []ChainMatch
	findAllMatches(nodes[0], entry.queries, entry.index, func(node *sightmap.ComponentNode, q *MatchQuery) {
		d := depthOf[node]
		cm := ChainMatch{Depth: d, Name: q.Name}
		if q.Def != nil {
			cm.Tags = q.Def.Tags
			cm.Memory = q.Def.Memory
			if anyPrivacy && declared[d] == "" {
				declared[d] = noDeclaration
				if q.Def.Privacy != "" {
					declared[d] = q.Def.Privacy
				}
			}
		}
		out = append(out, cm)
	})
	if anyPrivacy {
		// Nearest enclosing declaration wins down the spine.
		resolved, inherited := declared, ""
		for d, p := range declared {
			if p != "" && p != noDeclaration {
				inherited = knownPrivacy(p)
			}
			resolved[d] = inherited
		}
		for i := range out {
			out[i].Privacy = resolved[out[i].Depth]
		}
	}
	// FindAllMatches visits the linear spine depth-first, so out already runs
	// root -> leaf; a defensive stable sort keeps the contract explicit without
	// disturbing within-depth (definition) order.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Depth < out[j].Depth })
	return out
}

// NamesForChain returns the component name(s) that identify the observed leaf
// element, applying the spec's nearest-enclosing rule: the name resolved from
// the deepest chain level that matched. It is almost always a single name;
// more than one is returned only when several definitions match that same
// deepest level (a genuine identity conflict the caller may want to see, rather
// than a silently-picked winner). Names are deduplicated and returned in
// component-definition order. Returns nil when nothing along the chain matched.
func (m *Matcher) NamesForChain(chain []sightmap.Element, pageURL string) []string {
	matches := m.MatchChain(chain, pageURL)
	if len(matches) == 0 {
		return nil
	}
	deepest := matches[len(matches)-1].Depth // sorted ascending by depth
	var names []string
	seen := make(map[string]bool)
	for _, cm := range matches {
		if cm.Depth == deepest && !seen[cm.Name] {
			seen[cm.Name] = true
			names = append(names, cm.Name)
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
