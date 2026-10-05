package match_test

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// randomPrivacyCorpus is benchCorpus with a random privacy on about half the
// definitions, including an unrecognized value that must fail closed to block.
// keepHas false drops every definition whose selector uses :has().
func randomPrivacyCorpus(seed uint64, comps int, exotic, keepHas bool) *sightmap.Corpus {
	c := benchCorpus(seed, comps, exotic)
	r := rand.New(rand.NewPCG(seed, 3))
	choices := []string{"", "", "", "block", "mask", "unmask", "unmask", "bogus"}
	defs := c.GlobalComponents[:0]
	for _, d := range c.GlobalComponents {
		if !keepHas && slices.ContainsFunc(d.Selectors, func(s string) bool { return strings.Contains(s, ":has(") }) {
			continue
		}
		d.Privacy = choices[r.IntN(len(choices))]
		defs = append(defs, d)
	}
	c.GlobalComponents = defs
	return c
}

// visitChains calls fn for every node with its root-first ancestor chain.
func visitChains(root *sightmap.ComponentNode, fn func(nodes []*sightmap.ComponentNode, chain []sightmap.Element)) {
	var walk func(n *sightmap.ComponentNode, nodes []*sightmap.ComponentNode, chain []sightmap.Element)
	walk = func(n *sightmap.ComponentNode, nodes []*sightmap.ComponentNode, chain []sightmap.Element) {
		nodes, chain = append(nodes, n), append(chain, *n.Element)
		fn(nodes, chain)
		for _, c := range n.Children {
			walk(c, nodes, chain)
		}
	}
	walk(root, nil, nil)
}

// TestMatchChain_PrivacyEquivalentToMatch is the differential proof: for every
// element of many seeded pages, the privacy MatchChain resolves from the
// element's ancestor chain equals the privacy Match resolves on the full tree,
// as does which definition matches first. Selectors other than :has() depend
// only on an element and its ancestors, so with :has() excluded the two must
// agree exactly, including nearest-enclosing inheritance, unmask re-opening,
// first-match-wins between definitions on one node, and fail-closed handling of
// an unknown value.
func TestMatchChain_PrivacyEquivalentToMatch(t *testing.T) {
	seen := map[string]int{}
	compared := 0
	for seed := uint64(1); seed <= 60; seed++ {
		for _, exotic := range []bool{false, true} {
			root := benchTree(seed, 400, 10+int(seed)*3, exotic)
			m := match.NewMatcher(randomPrivacyCorpus(seed, 10+int(seed)*3, exotic, false))
			full := m.Match(root, "")
			visitChains(root, func(nodes []*sightmap.ComponentNode, chain []sightmap.Element) {
				depth := len(chain) - 1
				var at []match.ChainMatch
				for _, cm := range m.MatchChain(chain, "") {
					if cm.Depth == depth {
						at = append(at, cm)
					}
				}
				want := full[nodes[depth]]
				if want == nil {
					if len(at) > 0 {
						t.Fatalf("seed %d exotic %v: chain matched %q where Match matched nothing", seed, exotic, at[0].Name)
					}
					return
				}
				if len(at) == 0 {
					t.Fatalf("seed %d exotic %v: Match matched %q, chain matched nothing", seed, exotic, want.Name)
				}
				if at[0].Name != want.Name {
					t.Fatalf("seed %d exotic %v: first match %q, Match %q", seed, exotic, at[0].Name, want.Name)
				}
				for _, cm := range at {
					if cm.Privacy != want.Privacy {
						t.Fatalf("seed %d exotic %v: %s privacy %q on the chain, %q in Match", seed, exotic, cm.Name, cm.Privacy, want.Privacy)
					}
				}
				compared++
				seen[want.Privacy]++
			})
		}
	}
	// The comparison only proves something if every outcome actually occurred.
	for _, p := range []string{"", "block", "mask", "unmask"} {
		if seen[p] == 0 {
			t.Errorf("no matched element resolved to privacy %q; the corpus does not exercise it", p)
		}
	}
	t.Logf("compared %d matched elements; resolved privacy counts %v", compared, seen)
}

// TestMatchChain_PrivacyDivergesOnlyThroughHas runs the same comparison with
// :has() selectors kept, and requires every disagreement to involve a :has()
// definition on the element's path. That bounds the one known gap: nothing
// else makes the chain disagree with the full tree.
func TestMatchChain_PrivacyDivergesOnlyThroughHas(t *testing.T) {
	divergent := 0
	for seed := uint64(1); seed <= 60; seed++ {
		for _, exotic := range []bool{false, true} {
			corpus := randomPrivacyCorpus(seed, 10+int(seed)*3, exotic, true)
			usesHas := map[string]bool{}
			for _, d := range corpus.GlobalComponents {
				usesHas[d.Name] = slices.ContainsFunc(d.Selectors, func(s string) bool { return strings.Contains(s, ":has(") })
			}
			root := benchTree(seed, 400, 10+int(seed)*3, exotic)
			m := match.NewMatcher(corpus)
			full := m.Match(root, "")
			visitChains(root, func(nodes []*sightmap.ComponentNode, chain []sightmap.Element) {
				depth := len(chain) - 1
				cms := m.MatchChain(chain, "")
				var got string
				for _, cm := range cms {
					if cm.Depth == depth {
						got = cm.Privacy
					}
				}
				want := ""
				if w := full[nodes[depth]]; w != nil {
					want = w.Privacy
				} else {
					return // unmatched leaf: no ChainMatch to compare
				}
				if got == want {
					return
				}
				divergent++
				for i, n := range nodes {
					if w := full[n]; w != nil && usesHas[w.Name] {
						return
					}
					for _, cm := range cms {
						if cm.Depth == i && usesHas[cm.Name] {
							return
						}
					}
				}
				t.Fatalf("seed %d exotic %v: chain privacy %q, Match %q, and no :has() definition is on the path", seed, exotic, got, want)
			})
		}
	}
	t.Logf("%d elements diverged, each attributable to a :has() definition", divergent)
}

// TestMatchChain_PrivacyDeclaredOnlyByANestedChild guards the shortcut that
// skips privacy resolution when no definition declares privacy: a declaration
// that exists only on a child, flattened by the loader, still counts.
func TestMatchChain_PrivacyDeclaredOnlyByANestedChild(t *testing.T) {
	dir := t.TempDir()
	yaml := `components:
  - name: Checkout
    selector: 'form.checkout'
    children:
      - name: CardNumber
        selector: 'input.cc'
        privacy: block
`
	if err := os.WriteFile(filepath.Join(dir, "checkout.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	corpus, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := match.NewMatcher(corpus)
	for _, cm := range m.MatchChain([]sightmap.Element{el("form", "checkout"), el("input", "cc")}, "") {
		want := ""
		if cm.Name == "CardNumber" {
			want = "block"
		}
		if cm.Privacy != want {
			t.Errorf("%s: privacy %q, want %q", cm.Name, cm.Privacy, want)
		}
	}
}

// TestMatchChain_PrivacyDeclaredOnlyInAView guards the same shortcut for a
// declaration that exists only on a view-scoped component: on its route the
// privacy applies, off its route the component does not match at all.
func TestMatchChain_PrivacyDeclaredOnlyInAView(t *testing.T) {
	m := match.NewMatcher(&sightmap.Corpus{
		GlobalComponents: []sightmap.ComponentDef{{Name: "Form", Selectors: []string{"form"}}},
		Views: []sightmap.ViewDef{{Name: "Checkout", Route: "/checkout", Components: []sightmap.ComponentDef{
			{Name: "CardNumber", Selectors: []string{"input.cc"}, Privacy: "block"},
		}}},
	})
	chain := []sightmap.Element{el("form"), el("input", "cc")}
	got := map[string]string{}
	for _, cm := range m.MatchChain(chain, "/checkout") {
		got[cm.Name] = cm.Privacy
	}
	if want := map[string]string{"Form": "", "CardNumber": "block"}; !reflect.DeepEqual(got, want) {
		t.Errorf("on route: %v, want %v", got, want)
	}
	for _, cm := range m.MatchChain(chain, "/account") {
		if cm.Name == "CardNumber" {
			t.Errorf("view-scoped CardNumber matched off its route")
		}
	}
}
