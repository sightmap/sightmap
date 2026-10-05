package match_test

import (
	"fmt"
	"testing"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// benchChains returns up to n root-to-leaf element paths from the synthetic DOM,
// the shape a streaming consumer classifies one observed element at a time.
func benchChains(root *sightmap.ComponentNode, n int) [][]sightmap.Element {
	var out [][]sightmap.Element
	var walk func(node *sightmap.ComponentNode, path []sightmap.Element)
	walk = func(node *sightmap.ComponentNode, path []sightmap.Element) {
		if len(out) >= n {
			return
		}
		path = append(path, *node.Element)
		if len(node.Children) == 0 {
			out = append(out, append([]sightmap.Element(nil), path...))
			return
		}
		for _, c := range node.Children {
			walk(c, path)
		}
	}
	walk(root, nil)
	return out
}

// BenchmarkMatcherMatchChain classifies 200 leaf chains per op, against a
// corpus with no privacy and one where every tenth definition declares it (so
// chain privacy resolution is on the timed path).
func BenchmarkMatcherMatchChain(b *testing.B) {
	for _, withPrivacy := range []bool{false, true} {
		for _, s := range benchShapes {
			root := benchTree(1, s.nodes, s.comps, false)
			corpus := benchCorpus(1, s.comps, false)
			if withPrivacy {
				for i := range corpus.GlobalComponents {
					if i%10 == 0 {
						corpus.GlobalComponents[i].Privacy = []string{"block", "mask", "unmask"}[i/10%3]
					}
				}
			}
			m := match.NewMatcher(corpus)
			chains := benchChains(root, 200)
			m.MatchChain(chains[0], "") // compile and cache the queries outside the timed loop
			b.Run(fmt.Sprintf("privacy=%v/nodes=%d/comps=%d", withPrivacy, s.nodes, s.comps), func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					for _, c := range chains {
						m.MatchChain(c, "")
					}
				}
			})
		}
	}
}
