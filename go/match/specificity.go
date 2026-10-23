package match

import (
	"sync"

	"github.com/sightmap/sightmap/go/sightmap"
)

// specificity memoises ParsedSelector.MoreSpecific over the queries of one
// compiled query set, so a page with a thousand nodes each claimed by the same
// two definitions compares their selectors once.
type specificity struct {
	mu   sync.Mutex
	memo map[[2]*MatchQuery]bool
}

func (n *specificity) moreSpecific(a, b *MatchQuery) bool {
	key := [2]*MatchQuery{a, b}
	n.mu.Lock()
	defer n.mu.Unlock()
	if v, ok := n.memo[key]; ok {
		return v
	}
	if n.memo == nil {
		n.memo = make(map[[2]*MatchQuery]bool)
	}
	v := selectorOf(a).MoreSpecific(selectorOf(b))
	n.memo[key] = v
	return v
}

func selectorOf(q *MatchQuery) sightmap.ParsedSelector {
	return sightmap.ParsedSelector{Parts: q.Parts, Combinators: q.Combinators}
}

// mostSpecific drops every claim on one node that another claim is strictly
// more specific than (SEP-0023): `span.label` sees off `span`, so the node is
// `Label`, not contested. What remains keeps its order, so a caller that
// takes the first remaining claim applies the same tie-break as before among
// definitions the selector text cannot order.
func (n *specificity) mostSpecific(claims []*MatchQuery) []*MatchQuery {
	if len(claims) < 2 {
		return claims
	}
	out := make([]*MatchQuery, 0, len(claims))
	for i, c := range claims {
		beaten := false
		for j, d := range claims {
			if i != j && n.moreSpecific(d, c) {
				beaten = true
				break
			}
		}
		if !beaten {
			out = append(out, c)
		}
	}
	return out
}
