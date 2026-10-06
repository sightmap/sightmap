package match_test

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// benchPrivacyCorpus returns rules portable privacy definitions in the shapes
// authored corpora use: a class, a scoped class, an attribute on a tag.
func benchPrivacyCorpus(seed uint64, rules int) *sightmap.Corpus {
	r := rand.New(rand.NewPCG(seed, 4))
	kinds := []string{"block", "mask", "unmask"}
	defs := make([]sightmap.ComponentDef, 0, rules)
	for k := range rules {
		var sel string
		switch r.IntN(3) {
		case 0:
			sel = "." + benchClass(r)
		case 1:
			sel = fmt.Sprintf(`[data-component="%s"] .%s`, benchComp(r, 100), benchClass(r))
		default:
			sel = fmt.Sprintf(`input[data-testid="t%d"]`, r.IntN(50))
		}
		defs = append(defs, sightmap.ComponentDef{Name: fmt.Sprintf("P%d", k), Selectors: []string{sel}, Privacy: kinds[r.IntN(3)]})
	}
	return &sightmap.Corpus{GlobalComponents: defs}
}

// benchPrivacyChain is a depth-element ancestor chain with capture-like classes
// and attributes.
func benchPrivacyChain(seed uint64, depth int) []sightmap.Element {
	r := rand.New(rand.NewPCG(seed, 5))
	chain := make([]sightmap.Element, depth)
	for i := range chain {
		el := sightmap.Element{Tag: benchTags[r.IntN(len(benchTags))]}
		for range r.IntN(4) {
			el.Classes = append(el.Classes, benchClass(r))
		}
		if r.IntN(100) < 15 {
			el.Attrs = map[string]string{"data-component": benchComp(r, 100)}
		}
		chain[i] = el
	}
	return chain
}

// BenchmarkPrivacyForChain is the per-click cost: resolving privacy for one
// observed element from its ancestor chain.
func BenchmarkPrivacyForChain(b *testing.B) {
	for _, depth := range []int{8, 32, 128} {
		for _, rules := range []int{10, 100, 1000} {
			m := match.NewMatcher(benchPrivacyCorpus(1, rules))
			chain := benchPrivacyChain(1, depth)
			m.PrivacyForChain(chain, "") // compile outside the timed loop
			b.Run(fmt.Sprintf("depth=%d/rules=%d", depth, rules), func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					m.PrivacyForChain(chain, "")
				}
			})
		}
	}
}

// BenchmarkMatcherPrivacy is full-tree resolution, as a snapshot runs it.
func BenchmarkMatcherPrivacy(b *testing.B) {
	for _, s := range benchShapes[:2] {
		root := benchTree(1, s.nodes, s.comps, false)
		m := match.NewMatcher(benchPrivacyCorpus(1, 100))
		m.Privacy(root, "")
		b.Run(fmt.Sprintf("nodes=%d/rules=100", s.nodes), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				m.Privacy(root, "")
			}
		})
	}
}

// TestPrivacyForChain_AllocsBounded guards the per-click allocation count.
func TestPrivacyForChain_AllocsBounded(t *testing.T) {
	m := match.NewMatcher(benchPrivacyCorpus(1, 100))
	chain := benchPrivacyChain(1, 32)
	m.PrivacyForChain(chain, "")
	allocs := testing.AllocsPerRun(100, func() { m.PrivacyForChain(chain, "") })
	if allocs > 24 {
		t.Errorf("PrivacyForChain allocates %.0f times per call at depth 32, want at most 24", allocs)
	}
	t.Logf("PrivacyForChain: %.0f allocs per call at depth 32, 100 rules", allocs)
}
