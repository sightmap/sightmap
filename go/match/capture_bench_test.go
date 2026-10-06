package match_test

import (
	"fmt"
	"testing"

	"github.com/sightmap/sightmap/go/match"
)

// BenchmarkMatcherMatchWithCapture is BenchmarkMatcherMatch where every tenth
// definition declares privacy and every twentieth watch, so Match also
// resolves them.
func BenchmarkMatcherMatchWithCapture(b *testing.B) {
	for _, s := range benchShapes {
		root := benchTree(1, s.nodes, s.comps, false)
		corpus := benchCorpus(1, s.comps, false)
		for i := range corpus.GlobalComponents {
			if i%10 == 0 {
				corpus.GlobalComponents[i].Privacy = []string{"block", "mask", "unmask"}[i/10%3]
			}
			corpus.GlobalComponents[i].Watch = i%20 == 5
		}
		m := match.NewMatcher(corpus)
		m.Match(root, "") // compile and cache the queries outside the timed loop
		b.Run(fmt.Sprintf("nodes=%d/comps=%d", s.nodes, s.comps), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				m.Match(root, "")
			}
		})
	}
}
