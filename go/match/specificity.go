package match

import (
	"sync"

	"github.com/sightmap/sightmap/go/sightmap"
)

// specificity memoises ParsedSelector.MoreSpecific over the queries of one
// compiled query set, so a page with a thousand nodes each claimed by the same
// two definitions compares their selectors once. A Matcher matches many pages
// at once, so the memo is a sync.Map: almost every call is a read of a pair
// already compared, which must not queue behind a lock.
type specificity struct {
	memo sync.Map // [2]*MatchQuery -> bool
}

func (n *specificity) moreSpecific(a, b *MatchQuery) bool {
	key := [2]*MatchQuery{a, b}
	if v, ok := n.memo.Load(key); ok {
		return v.(bool)
	}
	v := selectorOf(a).MoreSpecific(selectorOf(b))
	n.memo.Store(key, v)
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
