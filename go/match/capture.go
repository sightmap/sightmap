package match

import (
	"slices"

	"github.com/sightmap/sightmap/go/sightmap"
)

// This file resolves privacy (SEP-0009) and watch (SEP-0015). Both come from
// every component whose selector matches an element, never from whichever
// component happens to name it.

// Privacy ranks: a stricter directive has a higher rank.
const (
	rankNone = iota
	rankUnmask
	rankMask
	rankBlock
)

var rankNames = [...]string{"", "unmask", "mask", "block"}

// rankOf ranks a declared privacy value. An unrecognized value is a block.
func rankOf(privacy string) int {
	switch privacy {
	case "unmask":
		return rankUnmask
	case "mask":
		return rankMask
	}
	return rankBlock
}

// foldRank resolves an element from its parent's effective rank and the
// strictest declaration on the element itself: block is absolute, otherwise the
// element's own declaration wins over what it inherits.
func foldRank(parent, local int) int {
	switch {
	case parent == rankBlock || local == rankBlock:
		return rankBlock
	case local != rankNone:
		return local
	}
	return parent
}

// captureSet is a page's compiled privacy and watch rules: one query per
// selector of every component that declares privacy or watch.
type captureSet struct {
	queries []MatchQuery // each with privacyRank and/or watch set
	index   *firstPartIndex
	// floor is the rank every element resolves to at least: the strictest block
	// or mask rule that could not be evaluated, which so covers the document.
	floor int
}

// compileCapture compiles the privacy and watch rules of defs (SEP-0021). A rule outside the capture-baseline profile
// never matches loosely: a block or mask raises the floor, an unmask is
// dropped, and a watch is not reported. A privacy rule whose subject matches
// nearly every element is treated the same way.
func compileCapture(defs []sightmap.ComponentDef) *captureSet {
	cs := &captureSet{}
	add := func(name, sel string, rank int, watch bool, def *sightmap.ComponentDef) {
		parsed, err := sightmap.ParseSightmapSelector(sel)
		prof, perr := sightmap.ParseProfileSelector(sel, sightmap.ProfileCaptureBaseline)
		valid := err == nil && perr == nil
		if rank != rankNone && (!valid || prof.Subject().Constraint == sightmap.ConstraintNone) {
			if rank != rankUnmask {
				cs.floor = max(cs.floor, rank)
			}
			rank = rankNone
		}
		if !valid {
			watch = false
		}
		if rank == rankNone && !watch {
			return
		}
		cs.queries = append(cs.queries, MatchQuery{
			Name: name, Parts: parsed.Parts, Combinators: parsed.Combinators, Def: def,
			privacyRank: rank, watch: watch,
		})
	}
	for i := range defs {
		d := &defs[i]
		if d.Privacy == "" && !d.Watch {
			continue
		}
		rank := rankNone
		if d.Privacy != "" {
			rank = rankOf(d.Privacy)
		}
		for _, sel := range d.Selectors {
			add(d.Name, sel, rank, d.Watch, d)
		}
	}
	cs.index = newFirstPartIndex(cs.queries)
	return cs
}

// captureDefsForURL is every component that can declare privacy or watch on
// pageURL: the matching view's components and all globals. A view component
// never hides a global of the same name here, since privacy and watch do not
// depend on which component names an element.
func captureDefsForURL(c *sightmap.Corpus, pageURL string) []sightmap.ComponentDef {
	v := c.ViewForURL(pageURL)
	if v == nil {
		return c.GlobalComponents
	}
	return append(slices.Clip(v.Components), c.GlobalComponents...)
}

// captured accumulates the watched components matching each node.
type captured struct {
	watched map[*sightmap.ComponentNode][]string
	// seen keys a node's already-recorded definitions, so a component with
	// several selectors is recorded once while two distinct components sharing
	// a name are both recorded (names are unique only per parent).
	seen map[watchKey]bool
}

type watchKey struct {
	node *sightmap.ComponentNode
	def  *sightmap.ComponentDef
}

func newCaptured() captured {
	return captured{watched: map[*sightmap.ComponentNode][]string{}, seen: map[watchKey]bool{}}
}

// add records a watch rule's match.
func (c captured) add(node *sightmap.ComponentNode, q *MatchQuery) {
	if !q.watch {
		return
	}
	k := watchKey{node, q.Def}
	if c.seen[k] {
		return
	}
	c.seen[k] = true
	c.watched[node] = append(c.watched[node], q.Name)
}

func (cs *captureSet) collect(root *sightmap.ComponentNode) captured {
	c := newCaptured()
	if root != nil && len(cs.queries) > 0 {
		findAllMatches(root, cs.queries, cs.index, func(node *sightmap.ComponentNode, q *MatchQuery) { c.add(node, q) })
	}
	c.sortWatched()
	return c
}

// sortWatched orders each node's watched names, so results do not depend on
// traversal order.
func (c captured) sortWatched() {
	for _, names := range c.watched {
		slices.Sort(names)
	}
}

// Privacy resolves the effective privacy of every node under root for pageURL:
// "block", "mask" or "unmask", absent when nothing applies. Unlike Match, it
// covers nodes no component names.
func (m *Matcher) Privacy(root *sightmap.ComponentNode, pageURL string) map[*sightmap.ComponentNode]string {
	out := map[*sightmap.ComponentNode]string{}
	cs := m.entryFor(pageURL).capture
	if root == nil || (len(cs.queries) == 0 && cs.floor == rankNone) {
		return out
	}
	local, eff := rankNone, []int(nil)
	walkMatches(root, cs.queries, cs.index,
		func(_ *sightmap.ComponentNode, q *MatchQuery) { local = max(local, q.privacyRank) },
		func(n *sightmap.ComponentNode, depth int) {
			parent := rankNone
			if depth > 0 {
				parent = eff[depth-1]
			}
			e := max(foldRank(parent, local), cs.floor)
			eff = append(eff[:depth], e)
			if e != rankNone {
				out[n] = rankNames[e]
			}
			local = rankNone
		})
	return out
}

// Watched returns, for every node under root that a watched component matches
// on pageURL, those components' names, sorted. One entry per matching
// component definition, so a name appears twice when two distinct components
// share it (component names are unique only per parent). Unlike Match, it
// covers nodes no component names.
func (m *Matcher) Watched(root *sightmap.ComponentNode, pageURL string) map[*sightmap.ComponentNode][]string {
	return m.entryFor(pageURL).capture.collect(root).watched
}
