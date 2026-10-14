package match_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// checkChainCapture resolves every root-to-node chain under root and checks it,
// at every depth, against the tree-walk spine resolution and against full-tree
// Privacy and Watched on the same nodes. Full-tree Watched reports one name per
// matching definition while chain resolution reports each name once, so the
// tree's names are compacted before comparing. It returns the chains checked.
func checkChainCapture(t *testing.T, label string, m *match.Matcher, root *sightmap.ComponentNode, pageURL string) int {
	t.Helper()
	privacy, watched := m.Privacy(root, pageURL), m.Watched(root, pageURL)
	checked := 0
	var walk func(n *sightmap.ComponentNode, path []*sightmap.ComponentNode, chain []sightmap.Element)
	walk = func(n *sightmap.ComponentNode, path []*sightmap.ComponentNode, chain []sightmap.Element) {
		path = append(slices.Clip(path), n)
		chain = append(slices.Clip(chain), *n.Element)
		gotP, gotW := m.PrivacyForChain(chain, pageURL), m.WatchedForChain(chain, pageURL)
		spineP, spineW := m.ChainCaptureSpine(chain, pageURL)
		if len(gotP) != len(chain) || len(gotW) != len(chain) {
			t.Fatalf("%s: chain of %d gave %d privacy and %d watched entries", label, len(chain), len(gotP), len(gotW))
		}
		for d, node := range path {
			if gotP[d] != spineP[d] || !slices.Equal(gotW[d], spineW[d]) {
				t.Fatalf("%s: depth %d of %d: chain %q %v, spine %q %v", label, d, len(chain), gotP[d], gotW[d], spineP[d], spineW[d])
			}
			if gotP[d] != privacy[node] {
				t.Fatalf("%s: depth %d of %d: chain privacy %q, tree %q", label, d, len(chain), gotP[d], privacy[node])
			}
			if want := slices.Compact(slices.Clone(watched[node])); !slices.Equal(gotW[d], want) {
				t.Fatalf("%s: depth %d of %d: chain watched %v, tree %v", label, d, len(chain), gotW[d], want)
			}
		}
		checked++
		for _, c := range n.Children {
			walk(c, path, chain)
		}
	}
	if root != nil {
		walk(root, nil, nil)
	}
	return checked
}

// TestChainCapture_EqualsTree: subject-indexed chain resolution equals the
// tree-walk spine and full-tree resolution, on every hand-written resolution
// case and on seeded generated corpora, trees and consumer rules.
func TestChainCapture_EqualsTree(t *testing.T) {
	cases := 0
	for _, set := range [][]resolutionCase{privacyCases, profileCases} {
		for _, c := range set {
			m := match.NewMatcher(loadCorpus(t, c.yaml))
			cases += checkChainCapture(t, c.name, m, c.tree, c.url)
		}
	}
	consumer := []match.PrivacyRule{
		{Selector: ".c001", Privacy: "block"},
		{Selector: `[data-component="Comp3"] .c010`, Privacy: "mask"},
		{Selector: "li > a", Privacy: "mask"},
	}
	generated, privacy, watched := 0, map[string]int{}, 0
	for seed := uint64(1); seed <= 40; seed++ {
		for _, exotic := range []bool{false, true} {
			corpus := randomCaptureCorpus(seed, 10+int(seed)*3, exotic)
			root := benchTree(seed, 300, 10+int(seed)*3, exotic)
			opts := []match.Option(nil)
			if seed%4 == 0 {
				opts = append(opts, match.WithPrivacyRules(consumer...))
			}
			m := match.NewMatcher(corpus, opts...)
			generated += checkChainCapture(t, fmt.Sprintf("seed %d exotic %v", seed, exotic), m, root, "")
			for _, p := range m.Privacy(root, "") {
				privacy[p]++
			}
			watched += len(m.Watched(root, ""))
		}
	}
	// The benchmark corpus and chains: deep chains with many rules.
	for _, rules := range []int{100, 1000} {
		m := match.NewMatcher(benchCaptureCorpus(1, rules))
		for seed := uint64(1); seed <= 20; seed++ {
			chain := benchPrivacyChain(seed, 8+int(seed)*6)
			var root *sightmap.ComponentNode
			for i := len(chain) - 1; i >= 0; i-- {
				node := &sightmap.ComponentNode{Element: &chain[i]}
				if root != nil {
					node.Children = []*sightmap.ComponentNode{root}
				}
				root = node
			}
			generated += checkChainCapture(t, fmt.Sprintf("bench rules %d seed %d", rules, seed), m, root, "")
		}
	}
	for _, p := range []string{"unmask", "mask", "block"} {
		if privacy[p] == 0 {
			t.Errorf("no generated node resolved to %q", p)
		}
	}
	if watched == 0 {
		t.Error("no generated node was watched")
	}
	t.Logf("%d hand-written and %d generated chains agree at every depth (%d watched nodes); privacy %v", cases, generated, watched, privacy)
}

// TestChainCapture_SubjectKeys: rules keyed on each kind of subject
// requirement (id, attribute value folded for case, attribute presence, class,
// tag) all reach the element, and the strictest wins.
func TestChainCapture_SubjectKeys(t *testing.T) {
	m := match.NewMatcher(loadCorpus(t, `components:
  - name: ById
    selector: 'form #card'
    privacy: mask
  - name: ByAttr
    selector: 'input[type=PASSWORD]'
    privacy: block
  - name: ByPresence
    selector: 'section > [data-secret]'
    privacy: mask
  - name: ByClass
    selector: '.note'
    privacy: unmask
  - name: ByTag
    selector: 'section textarea'
    privacy: mask
  - name: Watched
    selector: 'section [data-secret]:not(.quiet)'
    watch: true
`))
	in := func(tag string, mut func(*sightmap.Element)) sightmap.Element {
		e := sightmap.Element{Tag: tag}
		mut(&e)
		return e
	}
	chain := []sightmap.Element{
		{Tag: "body"},
		{Tag: "form"},
		{Tag: "section"},
		in("input", func(e *sightmap.Element) {
			e.Id = "card"
			e.Attrs = map[string]string{"type": "password", "data-secret": ""}
		}),
	}
	if got, want := m.PrivacyForChain(chain, ""), []string{"", "", "", "block"}; !slices.Equal(got, want) {
		t.Errorf("PrivacyForChain = %q, want %q", got, want)
	}
	if got := m.WatchedForChain(chain, ""); !slices.Equal(got[3], []string{"Watched"}) || got[2] != nil {
		t.Errorf("WatchedForChain = %v, want Watched at the leaf only", got)
	}
	chain[3] = in("textarea", func(e *sightmap.Element) { e.Classes = []string{"note", "quiet"} })
	if got, want := m.PrivacyForChain(chain, ""), []string{"", "", "", "mask"}; !slices.Equal(got, want) {
		t.Errorf("PrivacyForChain = %q, want %q", got, want)
	}
	if got := m.WatchedForChain(chain, ""); got[3] != nil {
		t.Errorf("WatchedForChain = %v, want nothing watched", got)
	}
}

// TestChainCapture_ZeroAllocs: past the returned slice, resolving privacy for
// a chain allocates nothing, and resolving watch allocates only for elements a
// watched component matches.
func TestChainCapture_ZeroAllocs(t *testing.T) {
	for _, rules := range []int{100, 1000} {
		m := match.NewMatcher(benchCaptureCorpus(1, rules))
		for _, depth := range []int{8, 32, 128} {
			chain := benchPrivacyChain(1, depth)
			m.PrivacyForChain(chain, "")
			if a := testing.AllocsPerRun(50, func() { m.PrivacyForChain(chain, "") }); a > 1 {
				t.Errorf("rules %d depth %d: PrivacyForChain allocates %.0f times, want 1 (the result)", rules, depth, a)
			}
			hits := 0
			for _, names := range m.WatchedForChain(chain, "") {
				if names != nil {
					hits++
				}
			}
			if a := testing.AllocsPerRun(50, func() { m.WatchedForChain(chain, "") }); a > float64(1+hits) {
				t.Errorf("rules %d depth %d: WatchedForChain allocates %.0f times, want at most %d", rules, depth, a, 1+hits)
			}
		}
	}
}
