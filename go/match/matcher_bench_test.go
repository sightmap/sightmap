package match_test

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// Matching a corpus of a few hundred components against a full-page DOM is
// dominated by FindAllMatches. These benchmarks reproduce that shape: a seeded
// synthetic DOM and a corpus that is mostly [data-component="X"] selectors plus
// flattened child chains, the way authored corpora look after flattening.

type benchShape struct {
	nodes, comps int
}

var benchShapes = []benchShape{
	{nodes: 2_000, comps: 100},
	{nodes: 10_000, comps: 600},
	{nodes: 40_000, comps: 600},
}

var benchTags = []string{"div", "div", "div", "div", "span", "span", "a", "button", "li", "ul", "p", "svg", "path", "input", "img"}

func benchClass(r *rand.Rand) string { return fmt.Sprintf("c%03d", r.IntN(300)) }

func benchComp(r *rand.Rand, comps int) string { return fmt.Sprintf("Comp%d", r.IntN(comps)) }

// benchTree builds a DOM-like tree breadth-first: most nodes are single-child
// wrappers (which is what makes real DOMs deep), the rest fan out into lists.
// exotic adds element ids, uppercase and non-ASCII tags, repeated classes, and
// id/class carried in Attrs, to reach every index path in equivalence tests.
func benchTree(seed uint64, nodes, comps int, exotic bool) *sightmap.ComponentNode {
	r := rand.New(rand.NewPCG(seed, 1))
	count := 0
	newNode := func() *sightmap.ComponentNode {
		count++
		el := &sightmap.Element{Tag: benchTags[r.IntN(len(benchTags))]}
		for range r.IntN(4) {
			el.Classes = append(el.Classes, benchClass(r))
		}
		if r.IntN(100) < 12 {
			el.Attrs = map[string]string{"data-component": benchComp(r, comps)}
		}
		if r.IntN(100) < 5 {
			if el.Attrs == nil {
				el.Attrs = map[string]string{}
			}
			el.Attrs["data-testid"] = fmt.Sprintf("t%d", r.IntN(50))
		}
		if exotic {
			switch p := r.IntN(100); {
			case p < 8:
				el.Id = fmt.Sprintf("id%d", r.IntN(30))
			case p < 14:
				el.Tag = strings.ToUpper(el.Tag)
			case p < 15:
				el.Tag = "K" // Kelvin sign: EqualFolds "k"
			case p < 19:
				if len(el.Classes) > 0 {
					el.Classes = append(el.Classes, el.Classes[0])
				}
			case p < 23:
				if el.Attrs == nil {
					el.Attrs = map[string]string{}
				}
				el.Attrs["id"] = fmt.Sprintf("id%d", r.IntN(30))
				el.Attrs["class"] = benchClass(r)
			}
		}
		return &sightmap.ComponentNode{Id: fmt.Sprint(count), Element: el}
	}
	root := newNode()
	queue := []*sightmap.ComponentNode{root}
	for len(queue) > 0 && count < nodes {
		n := queue[0]
		queue = queue[1:]
		var k int
		switch p := r.IntN(10); {
		case p == 0:
			k = 0
		case p <= 6:
			k = 1
		default:
			k = 2 + r.IntN(7)
		}
		if k == 0 && len(queue) == 0 {
			k = 1 // keep growing until the node budget is spent
		}
		for range k {
			if count >= nodes {
				break
			}
			c := newNode()
			n.Children = append(n.Children, c)
			queue = append(queue, c)
		}
	}
	return root
}

// benchExoticSelectors are first parts that land in the less common index
// buckets (id, attribute operators, tag, universal), plus combinations that
// put a :has() or :not() argument on an indexed part.
var benchExoticSelectors = []string{
	`#id%d`,
	`#id%d span`,
	`[id="id%d"]`,
	`[id^="id%d"]`,
	`[class~="c%03d"]`,
	`[class*="c%03d"] > a`,
	`[class^="c%03d"]`,
	`[class$="c%03d"]`,
	`[class|="c%03d"]`,
	`[class="c%03d"]`,
	`[class*="c0"]`,
	`[data-component^="Comp%d"]`,
	`k`,
	`DIV.c%03d`,
	`*`,
	`* > a.c%03d`,
	`:is(a, button) span`,
	`:not(.c%03d) > li`,
	`li:not([data-testid]) a`,
	`div:has([data-testid="t%d"]) > span`,
	`[data-testid]`,
}

// benchCorpus builds comps global component defs covering the selector shapes
// the matcher handles: attribute equality (the common case), descendant and
// child chains, classes, tags, attribute presence, :has() and :not(). exotic
// mixes in benchExoticSelectors.
func benchCorpus(seed uint64, comps int, exotic bool) *sightmap.Corpus {
	r := rand.New(rand.NewPCG(seed, 2))
	defs := make([]sightmap.ComponentDef, 0, comps)
	for k := range comps {
		if exotic && r.IntN(100) < 40 {
			sel := benchExoticSelectors[r.IntN(len(benchExoticSelectors))]
			if strings.Contains(sel, "%") {
				sel = fmt.Sprintf(sel, r.IntN(30))
			}
			defs = append(defs, sightmap.ComponentDef{Name: fmt.Sprintf("Def%d", k), Selectors: []string{sel}})
			continue
		}
		self := fmt.Sprintf(`[data-component="Comp%d"]`, k)
		var sel string
		switch p := r.IntN(100); {
		case p < 65:
			sel = self
		case p < 80:
			sel = fmt.Sprintf(`[data-component="%s"] %s`, benchComp(r, comps), self)
		case p < 85:
			sel = fmt.Sprintf(`[data-component="%s"] [data-component="%s"] %s`, benchComp(r, comps), benchComp(r, comps), self)
		case p < 89:
			sel = fmt.Sprintf(`[data-component="%s"] > %s`, benchComp(r, comps), benchTags[r.IntN(len(benchTags))])
		case p < 93:
			sel = "." + benchClass(r)
		case p < 96:
			sel = benchTags[r.IntN(len(benchTags))] + "." + benchClass(r)
		case p < 97:
			sel = fmt.Sprintf(`[data-testid="t%d"]`, r.IntN(50))
		case p < 98:
			sel = fmt.Sprintf(`ul:has(> li.%s)`, benchClass(r))
		case p < 99:
			sel = fmt.Sprintf(`a:not(.%s)`, benchClass(r))
		default:
			sel = `[data-testid] span`
		}
		defs = append(defs, sightmap.ComponentDef{Name: fmt.Sprintf("Def%d", k), Selectors: []string{sel}})
	}
	return &sightmap.Corpus{GlobalComponents: defs}
}

func BenchmarkMatcherMatch(b *testing.B) {
	for _, s := range benchShapes {
		root := benchTree(1, s.nodes, s.comps, false)
		m := match.NewMatcher(benchCorpus(1, s.comps, false))
		m.Match(root, "") // compile and cache the queries outside the timed loop
		b.Run(fmt.Sprintf("nodes=%d/comps=%d", s.nodes, s.comps), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				m.Match(root, "")
			}
		})
	}
}

// benchCaptureAttrs gives every node the attribute load of a real capture:
// class mirrored into Attrs (as extract does), a handful of common attributes,
// and camelCase viewBox/preserveAspectRatio on svg. benchTree alone leaves most
// nodes without Attrs, which hides the cost of attribute lookups that miss.
func benchCaptureAttrs(n *sightmap.ComponentNode) {
	el := n.Element
	if el.Attrs == nil {
		el.Attrs = map[string]string{}
	}
	if len(el.Classes) > 0 {
		el.Attrs["class"] = strings.Join(el.Classes, " ")
	}
	el.Attrs["role"] = "presentation"
	el.Attrs["aria-hidden"] = "false"
	el.Attrs["style"] = "display:block"
	el.Attrs["data-reactid"] = n.Id
	el.Attrs["tabindex"] = "-1"
	if el.Tag == "svg" {
		el.Attrs["viewBox"] = "0 0 24 24"
		el.Attrs["preserveAspectRatio"] = "xMidYMid"
	}
	for _, c := range n.Children {
		benchCaptureAttrs(c)
	}
}

func BenchmarkMatcherMatchCaptureAttrs(b *testing.B) {
	for _, s := range benchShapes {
		root := benchTree(1, s.nodes, s.comps, false)
		benchCaptureAttrs(root)
		m := match.NewMatcher(benchCorpus(1, s.comps, false))
		m.Match(root, "") // compile and cache the queries outside the timed loop
		b.Run(fmt.Sprintf("nodes=%d/comps=%d", s.nodes, s.comps), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				m.Match(root, "")
			}
		})
	}
}

// BenchmarkFindAllMatches compares the matcher against referenceFindAllMatches,
// the pre-index implementation, on identical inputs.
func BenchmarkFindAllMatches(b *testing.B) {
	impls := []struct {
		name string
		fn   func(*sightmap.ComponentNode, []match.MatchQuery, func(*sightmap.ComponentNode, *match.MatchQuery))
	}{
		{"reference", referenceFindAllMatches},
		{"indexed", match.FindAllMatches},
	}
	for _, s := range benchShapes {
		root := benchTree(1, s.nodes, s.comps, false)
		queries, errs := match.ParseQueries(benchCorpus(1, s.comps, false).GlobalComponents)
		if len(errs) > 0 {
			b.Fatal(errs)
		}
		for _, impl := range impls {
			b.Run(fmt.Sprintf("%s/nodes=%d/comps=%d", impl.name, s.nodes, s.comps), func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					impl.fn(root, queries, func(*sightmap.ComponentNode, *match.MatchQuery) {})
				}
			})
		}
	}
}

type matchCall struct {
	node *sightmap.ComponentNode
	q    *match.MatchQuery
}

func collectCalls(
	fn func(*sightmap.ComponentNode, []match.MatchQuery, func(*sightmap.ComponentNode, *match.MatchQuery)),
	root *sightmap.ComponentNode,
	queries []match.MatchQuery,
) []matchCall {
	var calls []matchCall
	fn(root, queries, func(n *sightmap.ComponentNode, q *match.MatchQuery) {
		calls = append(calls, matchCall{n, q})
	})
	return calls
}

// TestFindAllMatchesEquivalentToReference checks that FindAllMatches makes the
// exact same onMatch calls, in the same order, as the reference implementation.
// Order matters: Match is first-match-wins per node.
func TestFindAllMatchesEquivalentToReference(t *testing.T) {
	type tcase struct {
		seed         uint64
		nodes, comps int
		exotic       bool
	}
	var cases []tcase
	for seed := uint64(1); seed <= 40; seed++ {
		cases = append(cases, tcase{seed, 300, 10 + int(seed)*3, false}, tcase{seed, 300, 10 + int(seed)*3, true})
	}
	for _, s := range benchShapes[:2] {
		cases = append(cases, tcase{7, s.nodes, s.comps, false}, tcase{7, s.nodes, s.comps, true})
	}
	for _, c := range cases {
		root := benchTree(c.seed, c.nodes, c.comps, c.exotic)
		queries, errs := match.ParseQueries(benchCorpus(c.seed, c.comps, c.exotic).GlobalComponents)
		if len(errs) > 0 {
			t.Fatal(errs)
		}
		want := collectCalls(referenceFindAllMatches, root, queries)
		got := collectCalls(match.FindAllMatches, root, queries)
		if len(want) == 0 {
			t.Fatalf("%+v: reference produced no matches; the generator is not exercising the matcher", c)
		}
		if len(got) != len(want) {
			t.Fatalf("%+v: got %d calls, want %d", c, len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%+v call %d: got (%s, %s), want (%s, %s)",
					c, i, got[i].node.Id, got[i].q.Name, want[i].node.Id, want[i].q.Name)
			}
		}
	}
}

// referenceFindAllMatches is the pre-index FindAllMatches, kept verbatim as the
// behavioral oracle and the "before" side of BenchmarkFindAllMatches.
func referenceFindAllMatches(
	root *sightmap.ComponentNode,
	queries []match.MatchQuery,
	onMatch func(*sightmap.ComponentNode, *match.MatchQuery),
) {
	if root == nil || len(queries) == 0 {
		return
	}
	refFindAllMatchesNFA([]*sightmap.ComponentNode{root}, nil, nil, queries, onMatch)
}

type refState struct {
	q   *match.MatchQuery
	idx int
}

func refFindAllMatchesNFA(
	chain []*sightmap.ComponentNode,
	descendant, direct []refState,
	queries []match.MatchQuery,
	onMatch func(*sightmap.ComponentNode, *match.MatchQuery),
) {
	node := chain[len(chain)-1]
	seen := make(map[refState]bool, len(queries)+len(descendant)+len(direct))
	var toCheck []refState
	add := func(s refState) {
		if !seen[s] {
			seen[s] = true
			toCheck = append(toCheck, s)
		}
	}
	for _, s := range descendant {
		add(s)
	}
	for _, s := range direct {
		add(s)
	}
	for i := range queries {
		add(refState{&queries[i], 0})
	}

	nextDescendant := make([]refState, len(descendant), len(descendant)+8)
	copy(nextDescendant, descendant)
	var nextDirect []refState
	matchedQueries := make(map[*match.MatchQuery]bool, 4)

	for _, state := range toCheck {
		if !sightmap.MatchesNodeChain(chain, state.q.Parts[state.idx]) {
			continue
		}
		nextIdx := state.idx + 1
		if nextIdx == len(state.q.Parts) {
			if !matchedQueries[state.q] {
				matchedQueries[state.q] = true
				onMatch(node, state.q)
			}
			continue
		}
		ns := refState{state.q, nextIdx}
		if state.q.Combinators[nextIdx] == ">" {
			nextDirect = refAppendIfNew(nextDirect, ns)
		} else {
			nextDescendant = refAppendIfNew(nextDescendant, ns)
		}
	}

	for _, child := range node.Children {
		childChain := append(chain[:len(chain):len(chain)], child)
		refFindAllMatchesNFA(childChain, nextDescendant, nextDirect, queries, onMatch)
	}
}

func refAppendIfNew(ss []refState, s refState) []refState {
	if slices.Contains(ss, s) {
		return ss
	}
	return append(ss, s)
}
