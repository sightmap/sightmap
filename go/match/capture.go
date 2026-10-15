package match

import (
	"slices"
	"sync"

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

// PrivacyRule is a privacy rule a consumer supplies itself, such as a built-in
// default or a rule configured outside the corpus.
type PrivacyRule struct {
	Selector string
	// Privacy is "block" or "mask"; anything else unrecognized ranks as block.
	// A consumer rule may only withhold, so "unmask" is ignored.
	Privacy string
}

// Option configures a Matcher.
type Option func(*Matcher)

// WithPrivacyRules adds consumer privacy rules to every page's resolution.
func WithPrivacyRules(rules ...PrivacyRule) Option {
	return func(m *Matcher) { m.consumerRules = append(m.consumerRules, rules...) }
}

// captureSet is a page's compiled privacy and watch rules: one query per
// selector of every component that declares privacy or watch, plus the
// consumer's privacy rules.
type captureSet struct {
	queries []MatchQuery // each with privacyRank and/or watch set
	index   *partIndex
	// subjects keys the same queries on their subject, for chain resolution.
	subjects *partIndex
	// partBase[qi] is where query qi's parts start in keys and in a
	// chainScratch's memo slots, one per part.
	partBase []int
	// keys holds each part's index key, a cheap necessary condition checked
	// before matching the part in full.
	keys    []partKey
	scratch sync.Pool // *chainScratch
	// floor is the rank every element resolves to at least: the strictest block
	// or mask rule that could not be evaluated, which so covers the document.
	floor int
	// attrs are the attribute names the rules test, sorted.
	attrs []string
}

// compileCapture compiles the privacy and watch rules of defs and the
// consumer's rules. A rule outside the capture-baseline profile (SEP-0021)
// never matches loosely: a block or mask raises the floor, an unmask is
// dropped, and a watch is not reported. A privacy rule whose subject matches
// nearly every element is treated the same way.
func compileCapture(defs []sightmap.ComponentDef, consumer []PrivacyRule) *captureSet {
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
		for _, c := range prof.Compounds {
			for _, a := range c.Attrs {
				if !slices.Contains(cs.attrs, a) {
					cs.attrs = append(cs.attrs, a)
				}
			}
		}
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
	for _, r := range consumer {
		if r.Privacy == "unmask" {
			continue
		}
		add("", r.Selector, rankOf(r.Privacy), false, nil)
	}
	cs.index = newFirstPartIndex(cs.queries)
	cs.subjects = newSubjectIndex(cs.queries)
	for i := range cs.queries {
		cs.partBase = append(cs.partBase, len(cs.keys))
		for _, p := range cs.queries[i].Parts {
			cs.keys = append(cs.keys, keyOf(p))
		}
	}
	slots := len(cs.keys)
	cs.scratch.New = func() any { return &chainScratch{slots: make([]memoSlot, slots)} }
	slices.Sort(cs.attrs)
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

// PrivacyForChain resolves the effective privacy of each element on an
// observed element's ancestor chain (root first, leaf last) for pageURL. Every
// rule in the capture-baseline profile depends only on an element and its
// ancestors, so this equals what Privacy gives the same elements in the full
// tree. The result has one entry per chain element: "block",
// "mask", "unmask" or "".
func (m *Matcher) PrivacyForChain(chain []sightmap.Element, pageURL string) []string {
	return m.entryFor(pageURL).capture.chainPrivacy(chain)
}

// WatchedForChain returns, for each element on an ancestor chain, the names of
// the watched components matching it, sorted; nil where none does.
func (m *Matcher) WatchedForChain(chain []sightmap.Element, pageURL string) [][]string {
	return m.entryFor(pageURL).capture.chainWatched(chain)
}

// candidateBuf sizes the stack buffer for one element's candidate queries;
// only an element hitting more buckets than this allocates.
const candidateBuf = 64

// chainPrivacy resolves each element of chain from the queries whose subject
// the element could match, each verified against the element's ancestors.
// It allocates only the result.
func (cs *captureSet) chainPrivacy(chain []sightmap.Element) []string {
	privacy := make([]string, len(chain))
	if len(cs.queries) == 0 && cs.floor == rankNone {
		return privacy
	}
	v := cs.verifier(chain)
	defer cs.scratch.Put(v.sc)
	var buf [candidateBuf]int
	parent := rankNone
	for i := range chain {
		local := rankNone
		for _, qi := range cs.subjects.candidates(&chain[i], buf[:0]) {
			q := &cs.queries[qi]
			if q.privacyRank > local && v.matches(qi, i) {
				if local = q.privacyRank; local == rankBlock {
					break
				}
			}
		}
		parent = max(foldRank(parent, local), cs.floor)
		privacy[i] = rankNames[parent]
		if parent == rankBlock {
			// Block is absolute: everything below the element is blocked too.
			for j := i + 1; j < len(chain); j++ {
				privacy[j] = rankNames[rankBlock]
			}
			break
		}
	}
	return privacy
}

// chainWatched collects, for each element of chain, the names of the watch
// queries matching it, sorted and deduplicated. It allocates the result and a
// name slice for each element some watch query matches.
func (cs *captureSet) chainWatched(chain []sightmap.Element) [][]string {
	watched := make([][]string, len(chain))
	if len(cs.queries) == 0 {
		return watched
	}
	v := cs.verifier(chain)
	defer cs.scratch.Put(v.sc)
	var buf [candidateBuf]int
	var nameBuf [8]string
	for i := range chain {
		names := nameBuf[:0]
		for _, qi := range cs.subjects.candidates(&chain[i], buf[:0]) {
			q := &cs.queries[qi]
			if q.watch && !slices.Contains(names, q.Name) && v.matches(qi, i) {
				names = append(names, q.Name)
			}
		}
		if len(names) > 0 {
			slices.Sort(names)
			watched[i] = slices.Clone(names)
		}
	}
	return watched
}

// chainScratch is the per-call memo of a chain verification, pooled so a call
// does not allocate it in the steady state. A slot is live only when its gen is the scratch's, so
// starting a call is one increment rather than a clear.
type chainScratch struct {
	gen   uint32
	slots []memoSlot
}

// memoSlot records, for one query and one part k, the lowest chain index where
// the selector prefix Parts[:k+1] matches with Parts[k] as its subject: hit,
// once found, else -1 with chain[:next] known not to match.
type memoSlot struct {
	gen       uint32
	hit, next int32
}

// chainVerifier checks capture queries against one ancestor chain.
type chainVerifier struct {
	cs    *captureSet
	chain []sightmap.Element
	sc    *chainScratch
}

func (cs *captureSet) verifier(chain []sightmap.Element) chainVerifier {
	sc := cs.scratch.Get().(*chainScratch)
	if sc.gen++; sc.gen == 0 {
		clear(sc.slots)
		sc.gen = 1
	}
	return chainVerifier{cs, chain, sc}
}

// matches reports whether query qi matches chain[i].
func (v chainVerifier) matches(qi, i int) bool {
	return v.at(qi, len(v.cs.queries[qi].Parts)-1, i)
}

// at reports whether the selector prefix Parts[:k+1] of query qi matches with
// Parts[k] anchored at chain[i], right to left: a child combinator takes
// exactly one step up, a descendant combinator asks whether any ancestor
// matches the rest. Every capture query is in the capture-baseline profile,
// whose parts need no subtree (no :has()) and whose :not() takes a single
// simple selector, so the element-only sightmap.Matches decides each part
// exactly as the tree walk's MatchesNodeChain does.
func (v chainVerifier) at(qi, k, i int) bool {
	q := &v.cs.queries[qi]
	el := &v.chain[i]
	if !v.cs.keys[v.cs.partBase[qi]+k].admits(el) || !sightmap.Matches(el, q.Parts[k]) {
		return false
	}
	switch {
	case k == 0:
		return true
	case q.Combinators[k] == ">":
		return i > 0 && v.at(qi, k-1, i-1)
	}
	return v.above(qi, k-1, i)
}

// above reports whether Parts[:k+1] of query qi matches some chain[j], j < i.
// Whether it matches at j depends only on chain[:j+1], the same for every
// element below, so the lowest such j is found once per call and each element
// asking again costs a comparison: a chain resolves in time linear in its
// depth per query part, not quadratic.
func (v chainVerifier) above(qi, k, i int) bool {
	s := &v.sc.slots[v.cs.partBase[qi]+k]
	if s.gen != v.sc.gen {
		*s = memoSlot{gen: v.sc.gen, hit: -1}
	}
	if s.hit >= 0 {
		return int(s.hit) < i
	}
	for ; int(s.next) < i; s.next++ {
		if v.at(qi, k, int(s.next)) {
			s.hit = s.next
			return true
		}
	}
	return false
}

// PrivacyAttributes returns the attribute names pageURL's privacy and watch
// rules test, sorted. A consumer that records an element's ancestor chain for
// PrivacyForChain or WatchedForChain must record at least these attributes,
// plus each element's tag, id and classes.
func (m *Matcher) PrivacyAttributes(pageURL string) []string {
	return slices.Clone(m.entryFor(pageURL).capture.attrs)
}
