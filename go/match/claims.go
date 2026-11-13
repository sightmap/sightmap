package match

import (
	"encoding/binary"
	"slices"
	"sync"

	"github.com/sightmap/sightmap/go/sightmap"
)

// ClaimCache remembers which nodes of a tree each selector matches, so a tool
// that matches the same trees against a long run of slightly different
// corpora -- a generator checking its own output after every change -- walks
// each tree only for the selectors it has not met before.
//
// A query's matches depend on its selector and the tree alone, not on its
// name, its definition or the corpus around it, so they are shared by
// MatchQuery.Key across Matchers and across corpora. Which claim names a node
// is still decided as Match decides it, over the remembered claims: the
// cached calls return exactly what the uncached ones do.
//
// Trees must not change while they are cached. A ClaimCache is safe for
// concurrent use; calls on different trees run in parallel.
type ClaimCache struct {
	mu    sync.Mutex
	keys  map[string]int32
	trees map[*sightmap.ComponentNode]*treeClaims
	// sets keeps the index of each missing query set met, by its key ids
	// (missingSet), so the trees of one view, which after a change usually
	// miss the same queries, share one.
	sets map[string]*missingSet
}

// maxMissingSets bounds ClaimCache.sets.
const maxMissingSets = 16

// missingSet is a set of queries walked together and its index, which reports
// matches as pointers into queries; at maps each of those pointers back to its
// position in queries.
type missingSet struct {
	queries []MatchQuery
	ids     []int32
	index   *queryIndex
	at      map[*MatchQuery]int
}

// newMissingSet readies queries (with their key ids) to be walked together,
// building the index unless one is given.
func newMissingSet(queries []MatchQuery, ids []int32, index *queryIndex) *missingSet {
	if index == nil {
		index = newQueryIndex(queries)
	}
	return &missingSet{queries: queries, ids: ids, index: index, at: positions(queries)}
}

// NewClaimCache returns an empty cache.
func NewClaimCache() *ClaimCache {
	return &ClaimCache{keys: map[string]int32{}, trees: map[*sightmap.ComponentNode]*treeClaims{}, sets: map[string]*missingSet{}}
}

// treeClaims is one tree's remembered matches: seen marks the keys already
// walked for (a bitset over key ids), claims holds the nodes each matched key
// matched, in document order.
type treeClaims struct {
	mu     sync.Mutex
	seen   []uint64
	claims map[int32][]*sightmap.ComponentNode
}

func (t *treeClaims) has(id int32) bool {
	w := int(id >> 6)
	return w < len(t.seen) && t.seen[w]&(1<<(id&63)) != 0
}

func (t *treeClaims) mark(id int32) {
	w := int(id >> 6)
	for len(t.seen) <= w {
		t.seen = append(t.seen, 0)
	}
	t.seen[w] |= 1 << (id & 63)
}

func (c *ClaimCache) tree(root *sightmap.ComponentNode) *treeClaims {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.trees[root]
	if t == nil {
		t = &treeClaims{claims: map[int32][]*sightmap.ComponentNode{}}
		c.trees[root] = t
	}
	return t
}

// keyIDs interns the Keys of queries (-1 for an empty Key, never cached).
func (c *ClaimCache) keyIDs(queries []MatchQuery) []int32 {
	ids := make([]int32, len(queries))
	c.mu.Lock()
	for i := range queries {
		k := queries[i].Key
		if k == "" {
			ids[i] = -1
			continue
		}
		id, ok := c.keys[k]
		if !ok {
			id = int32(len(c.keys))
			c.keys[k] = id
		}
		ids[i] = id
	}
	c.mu.Unlock()
	return ids
}

// claims returns, for each query, the nodes of root it matches in document
// order, walking root once for every query whose key has not been walked for
// (and for every query without a key). The tree's lock is held by the caller.
func (c *ClaimCache) claims(t *treeClaims, root *sightmap.ComponentNode, queries []MatchQuery, ids []int32, full func() *missingSet) [][]*sightmap.ComponentNode {
	out := make([][]*sightmap.ComponentNode, len(queries))
	var missing []int // indices into queries, one per missing key, then every keyless one
	queued := map[int32]bool{}
	keyless := false
	for i, id := range ids {
		switch {
		case id < 0:
			keyless = true
			missing = append(missing, i)
		case !t.has(id) && !queued[id]:
			queued[id] = true
			missing = append(missing, i)
		}
	}
	if len(missing) > 0 {
		var set *missingSet
		switch {
		case len(missing) == len(queries) && full != nil:
			// A tree met for the first time misses everything when every key is
			// distinct: walk the whole set with its own index, built once for
			// every tree.
			set = full()
		case keyless:
			sub := make([]MatchQuery, len(missing))
			for j, i := range missing {
				sub[j] = queries[i]
			}
			subIDs := make([]int32, len(missing))
			for j, i := range missing {
				subIDs[j] = ids[i]
			}
			set = newMissingSet(sub, subIDs, nil)
		default:
			set = c.setFor(queries, ids, missing)
		}
		found := make([][]*sightmap.ComponentNode, len(set.queries))
		walkMatches(root, set.queries, set.index, func(n *sightmap.ComponentNode, q *MatchQuery) {
			j := set.at[q]
			found[j] = append(found[j], n)
		}, nil)
		keylessAt := 0
		for j, id := range set.ids {
			if id >= 0 {
				if !t.has(id) {
					t.mark(id)
					if len(found[j]) > 0 {
						t.claims[id] = found[j]
					}
				}
				continue
			}
			// Keyless queries are not cached; they come back in the order missing listed them.
			for keylessAt < len(missing) && ids[missing[keylessAt]] >= 0 {
				keylessAt++
			}
			out[missing[keylessAt]] = found[j]
			keylessAt++
		}
	}
	for i, id := range ids {
		if id >= 0 {
			out[i] = t.claims[id]
		}
	}
	return out
}

// setFor is the missing set of the queries at missing (all keyed), its index
// built once per distinct set of keys. Queries with one key match the same
// nodes, so a set built from another call's queries serves this one.
func (c *ClaimCache) setFor(queries []MatchQuery, ids []int32, missing []int) *missingSet {
	key := make([]byte, 0, 4*len(missing))
	for _, i := range missing {
		id := ids[i]
		key = binary.LittleEndian.AppendUint32(key, uint32(id))
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if set := c.sets[string(key)]; set != nil {
		return set
	}
	// A missing set recurs across the trees of one view within one check,
	// rarely after, and each index pools walkers sized to its queries: keep
	// a bounded number rather than every set met.
	if len(c.sets) >= maxMissingSets {
		clear(c.sets)
	}
	subQueries, subIDs := make([]MatchQuery, len(missing)), make([]int32, len(missing))
	for j, i := range missing {
		subQueries[j], subIDs[j] = queries[i], ids[i]
	}
	set := newMissingSet(subQueries, subIDs, nil)
	c.sets[string(key)] = set
	return set
}

// CachedQueries is a query set prepared for matching through a ClaimCache,
// as Prepare readies one for FindAll. The slice must not change afterwards.
type CachedQueries struct {
	cache   *ClaimCache
	queries []MatchQuery
	ids     []int32
	once    sync.Once
	set     *missingSet
}

// fullSet is the whole query set, indexed on first use.
func (q *CachedQueries) fullSet() *missingSet {
	q.once.Do(func() { q.set = newMissingSet(q.queries, q.ids, nil) })
	return q.set
}

// Prepare readies queries for matching through c.
func (c *ClaimCache) Prepare(queries []MatchQuery) *CachedQueries {
	return &CachedQueries{cache: c, queries: queries, ids: c.keyIDs(queries)}
}

// Claims returns, for each query (aligned with the slice given to Prepare),
// the nodes of root it matches in document order, from the cache where it
// can. The lists are shared with the cache: read them, never change them.
func (q *CachedQueries) Claims(root *sightmap.ComponentNode) [][]*sightmap.ComponentNode {
	if root == nil || len(q.queries) == 0 {
		return make([][]*sightmap.ComponentNode, len(q.queries))
	}
	t := q.cache.tree(root)
	t.mu.Lock()
	defer t.mu.Unlock()
	return q.cache.claims(t, root, q.queries, q.ids, q.fullSet)
}

// FindAll calls onMatch for every node of root each query matches, as
// FindAllMatches does, from the cache where it can. Matches arrive query by
// query, each query's in document order, not interleaved by node.
func (q *CachedQueries) FindAll(root *sightmap.ComponentNode, onMatch func(*sightmap.ComponentNode, *MatchQuery)) {
	for i, ns := range q.Claims(root) {
		for _, n := range ns {
			onMatch(n, &q.queries[i])
		}
	}
}

// MatchConflictsCached is MatchConflicts over cache: the same names,
// privacy, watch, properties and conflicts, in the same order.
func (m *Matcher) MatchConflictsCached(cache *ClaimCache, root *sightmap.ComponentNode, pageURL string) (map[*sightmap.ComponentNode]*sightmap.ComponentMatch, []sightmap.Conflict) {
	return m.matchCached(cache, root, pageURL, true)
}

// ConflictsCached is Conflicts over cache.
func (m *Matcher) ConflictsCached(cache *ClaimCache, root *sightmap.ComponentNode, pageURL string) []sightmap.Conflict {
	_, out := m.matchCached(cache, root, pageURL, false)
	return out
}

// matchCached is MatchConflictsCached; without names it skips resolving
// properties, which only names need.
func (m *Matcher) matchCached(cache *ClaimCache, root *sightmap.ComponentNode, pageURL string, names bool) (map[*sightmap.ComponentNode]*sightmap.ComponentMatch, []sightmap.Conflict) {
	entry := m.entryFor(pageURL)
	if root == nil || len(entry.queries) == 0 {
		return nil, nil
	}
	qs := entry.combined
	full := m.entrySet(entry, cache)
	t := cache.tree(root)
	t.mu.Lock()
	per := cache.claims(t, root, qs, full.ids, func() *missingSet { return full })
	t.mu.Unlock()

	// Each node's claims, as indices into qs in query order.
	byNode := map[*sightmap.ComponentNode][]int{}
	for i, ns := range per {
		for _, n := range ns {
			byNode[n] = append(byNode[n], i)
		}
	}
	// The most specific naming claims of each node. Where two or more remain,
	// the first in the walk's match order names the node (inherited chains
	// before fresh starts: a child definition beats a bare global), which
	// query order does not give, so those nodes are walked again over just
	// the claims that remain there.
	naming := map[*sightmap.ComponentNode][]*MatchQuery{}
	ambiguous := map[*sightmap.ComponentNode]bool{}
	var again []int
	at := map[*MatchQuery]int{} // qs positions of the claims of ambiguous nodes
	for n, is := range byNode {
		var cl []*MatchQuery
		for _, i := range is {
			if q := &qs[i]; q.privacyRank == rankNone && !q.watch {
				cl = append(cl, q)
			}
		}
		if cl = entry.narrow.mostSpecific(cl); len(cl) > 0 {
			naming[n] = cl
		}
		if len(cl) >= 2 {
			ambiguous[n] = true
			for _, i := range is {
				at[&qs[i]] = i
			}
			for _, q := range cl {
				again = append(again, at[q])
			}
		}
	}
	if len(again) > 0 {
		slices.Sort(again)
		again = slices.Compact(again)
		sub := make([]MatchQuery, len(again))
		for j, i := range again {
			sub[j] = qs[i]
		}
		order := map[*sightmap.ComponentNode][]*MatchQuery{}
		subAt := positions(sub)
		walkMatches(root, sub, newQueryIndex(sub), func(n *sightmap.ComponentNode, q *MatchQuery) {
			if ambiguous[n] {
				order[n] = append(order[n], &qs[again[subAt[q]]])
			}
		}, nil)
		for n := range ambiguous {
			keep := naming[n]
			var cl []*MatchQuery
			for _, q := range order[n] {
				if slices.Contains(keep, q) {
					cl = append(cl, q)
				}
			}
			naming[n] = cl
		}
	}

	// Then the walk Match makes, node by node in document order, from the
	// claims instead of the NFA.
	var out []sightmap.Conflict
	result := make(map[*sightmap.ComponentNode]*sightmap.ComponentMatch)
	defByNode := make(map[*sightmap.ComponentNode]*sightmap.ComponentDef)
	var eff []int
	var declared map[*sightmap.ComponentNode]int
	sightmap.Walk(root, func(node *sightmap.ComponentNode, depth int) bool {
		local := rankNone
		var watched []string
		var watchDef []*sightmap.ComponentDef
		for _, i := range byNode[node] {
			q := &qs[i]
			if q.privacyRank == rankNone && !q.watch {
				continue
			}
			local = max(local, q.privacyRank)
			if q.watch && !slices.Contains(watchDef, q.Def) {
				watchDef = append(watchDef, q.Def)
				watched = append(watched, q.Name)
			}
		}
		parent := rankNone
		if depth > 0 {
			parent = eff[depth-1]
		}
		e := foldRank(parent, local)
		eff = append(eff[:depth], e)
		if local != rankNone {
			if declared == nil {
				declared = map[*sightmap.ComponentNode]int{}
			}
			declared[node] = local
		}
		claims := naming[node]
		if len(claims) >= 2 {
			if c, ok := conflictAt(node, claims); ok {
				out = append(out, c)
			}
		}
		if len(claims) > 0 {
			q := claims[0]
			named := &sightmap.ComponentMatch{Name: q.Name}
			if q.Def != nil {
				named.Memory = q.Def.Memory
				named.Tags = q.Def.Tags
				defByNode[node] = q.Def
			}
			result[node] = named
			named.Privacy = rankNames[e]
			if len(watched) > 0 {
				slices.Sort(watched)
				named.Watched, named.Watch = watched, true
			}
		}
		return true
	})
	if !names {
		return nil, out
	}
	resolveComponentProperties(&propResolver{root: root, result: result, defByNode: defByNode, declared: declared})
	return result, out
}

// entrySet is entry.combined as a missingSet, its key ids interned once per
// entry. It lives on the Matcher's entry, so a cache that outlives a run of
// corpora keeps none of their compiled queries alive; the entry holds the
// cache only while the Matcher lives.
func (m *Matcher) entrySet(entry *queryCacheEntry, cache *ClaimCache) *missingSet {
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry.setFor != cache {
		entry.set, entry.setFor = newMissingSet(entry.combined, cache.keyIDs(entry.combined), entry.combinedIndex), cache
	}
	return entry.set
}

// positions maps each query's address in qs to its index.
func positions(qs []MatchQuery) map[*MatchQuery]int {
	m := make(map[*MatchQuery]int, len(qs))
	for i := range qs {
		m[&qs[i]] = i
	}
	return m
}

// ClaimStats describes what a ClaimCache holds.
type ClaimStats struct {
	Trees, Keys, Lists, Nodes int
}

// Stats counts the trees, distinct keys, remembered match lists and the nodes
// in them.
func (c *ClaimCache) Stats() ClaimStats {
	c.mu.Lock()
	st := ClaimStats{Trees: len(c.trees), Keys: len(c.keys)}
	trees := make([]*treeClaims, 0, len(c.trees))
	for _, t := range c.trees {
		trees = append(trees, t)
	}
	c.mu.Unlock()
	for _, t := range trees {
		t.mu.Lock()
		st.Lists += len(t.claims)
		for _, ns := range t.claims {
			st.Nodes += len(ns)
		}
		t.mu.Unlock()
	}
	return st
}
