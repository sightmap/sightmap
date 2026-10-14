package match

import (
	"slices"

	"github.com/sightmap/sightmap/go/sightmap"
)

// ChainCaptureSpine resolves a chain the way chain resolution did before the
// subject index: build the chain as a one-child-per-node tree and run the tree
// walk over it. Tests use it as an oracle for PrivacyForChain and
// WatchedForChain.
func (m *Matcher) ChainCaptureSpine(chain []sightmap.Element, pageURL string) ([]string, [][]string) {
	privacy := make([]string, len(chain))
	watched := make([][]string, len(chain))
	cs := m.entryFor(pageURL).capture
	if len(chain) == 0 || (len(cs.queries) == 0 && cs.floor == rankNone) {
		return privacy, watched
	}
	nodes := make([]sightmap.ComponentNode, len(chain))
	for i := range chain {
		nodes[i].Element = &chain[i]
		if i > 0 {
			nodes[i-1].Children = []*sightmap.ComponentNode{&nodes[i]}
		}
	}
	local, parent := rankNone, rankNone
	var names []string
	walkMatches(&nodes[0], cs.queries, cs.index,
		func(_ *sightmap.ComponentNode, q *MatchQuery) {
			local = max(local, q.privacyRank)
			if q.watch && !slices.Contains(names, q.Name) {
				names = append(names, q.Name)
			}
		},
		func(_ *sightmap.ComponentNode, depth int) {
			parent = max(foldRank(parent, local), cs.floor)
			privacy[depth] = rankNames[parent]
			if len(names) > 0 {
				slices.Sort(names)
				watched[depth] = names
			}
			local, names = rankNone, nil
		})
	return privacy, watched
}
