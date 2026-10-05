package match

import (
	"slices"
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
	// It is never less restrictive than Match. A definition whose selector
	// needs the node's subtree (:has()) cannot be located on a chain, so it
	// fails closed: a block or mask one applies to every chain node, and an
	// unmask one is never honored.
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
	cp := entry.chain
	// declared[d] is "" until depth d's first match decides it, then that
	// definition's privacy, or noDeclaration when it declares none.
	const noDeclaration = "\x00"
	var declared []string
	if cp.any {
		declared = make([]string, len(nodes))
	}
	var out []ChainMatch
	findAllMatches(nodes[0], entry.queries, entry.index, func(node *sightmap.ComponentNode, q *MatchQuery) {
		d := depthOf[node]
		cm := ChainMatch{Depth: d, Name: q.Name}
		if q.Def != nil {
			cm.Tags = q.Def.Tags
			cm.Memory = q.Def.Memory
			if cp.any && declared[d] == "" {
				declared[d] = noDeclaration
				if q.Def.Privacy != "" && !cp.unreliable[q.Def] {
					declared[d] = q.Def.Privacy
				}
			}
		}
		out = append(out, cm)
	})
	if cp.any {
		// Nearest enclosing declaration wins down the spine; the opaque
		// directive from subtree-dependent definitions is a floor on every node.
		resolved, inherited := declared, ""
		for d, p := range declared {
			if p != "" && p != noDeclaration {
				inherited = knownPrivacy(p)
			}
			resolved[d] = stricterPrivacy(inherited, cp.opaque)
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

// chainPrivacy is what MatchChain needs to resolve privacy, computed once per
// compiled query set. any is false when no definition declares privacy, which
// skips resolution entirely. A definition whose selector needs a subtree is
// unreliable on a chain: its own match is ignored, and if it is block or mask its
// directive becomes opaque, a floor applied to every chain node.
type chainPrivacy struct {
	any        bool
	opaque     string
	unreliable map[*sightmap.ComponentDef]bool
}

func newChainPrivacy(queries []MatchQuery) chainPrivacy {
	var cp chainPrivacy
	for i := range queries {
		q := &queries[i]
		if q.Def == nil || q.Def.Privacy == "" {
			continue
		}
		cp.any = true
		if !queryNeedsSubtree(q) {
			continue
		}
		if cp.unreliable == nil {
			cp.unreliable = map[*sightmap.ComponentDef]bool{}
		}
		cp.unreliable[q.Def] = true
		if p := knownPrivacy(q.Def.Privacy); p != "unmask" {
			cp.opaque = stricterPrivacy(cp.opaque, p)
		}
	}
	return cp
}

// queryNeedsSubtree reports whether q's selector uses :has() anywhere, including
// inside :is() or :not(). A chain carries a node's ancestors but not its
// subtree, so such a selector can neither be confirmed nor ruled out there.
func queryNeedsSubtree(q *MatchQuery) bool {
	return slices.ContainsFunc(q.Parts, partNeedsSubtree)
}

func partNeedsSubtree(p *sightmap.SelectorPart) bool {
	if p == nil {
		return false
	}
	if len(p.Has) > 0 || slices.ContainsFunc(p.Is, partNeedsSubtree) {
		return true
	}
	return slices.ContainsFunc(p.Not, func(n sightmap.ParsedSelector) bool {
		return slices.ContainsFunc(n.Parts, partNeedsSubtree)
	})
}

// stricterPrivacy returns the more restrictive of two resolved directives:
// block over mask over unmask or none.
func stricterPrivacy(a, b string) string {
	rank := func(p string) int {
		switch p {
		case "block":
			return 2
		case "mask":
			return 1
		}
		return 0
	}
	if rank(b) > rank(a) {
		return b
	}
	return a
}
