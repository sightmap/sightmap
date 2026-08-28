package match

import (
	"sync"

	"github.com/sightmap/sightmap/go/sightmap"
)

// Matcher is the matching engine for a Corpus. It compiles the corpus's
// component definitions into NFA queries — lazily, cached per page URL — and
// runs them against a live component tree. The Corpus itself is pure data; build
// a Matcher when you need to match against it.
//
// A Matcher holds a per-URL compiled-query cache and is safe for concurrent use.
// Create one per Corpus and reuse it so the cache is shared across calls.
type Matcher struct {
	corpus *sightmap.Corpus
	mu     sync.Mutex
	cache  map[string]*queryCacheEntry
}

// NewMatcher returns a Matcher bound to corpus.
func NewMatcher(corpus *sightmap.Corpus) *Matcher {
	return &Matcher{corpus: corpus}
}

// queryCacheEntry stores the merged component list and compiled queries for one
// page URL. Compilation is the expensive step, so it is cached per URL.
type queryCacheEntry struct {
	components []sightmap.ComponentDef
	queries    []MatchQuery
}

// entryFor returns the cached (or freshly compiled) queries for pageURL.
func (m *Matcher) entryFor(pageURL string) *queryCacheEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cache == nil {
		m.cache = make(map[string]*queryCacheEntry)
	}
	if e, ok := m.cache[pageURL]; ok {
		return e
	}
	compList := m.corpus.ComponentsForURL(pageURL)
	queries, _ := ParseQueries(compList)
	e := &queryCacheEntry{components: compList, queries: queries}
	m.cache[pageURL] = e
	return e
}

// Match applies the corpus to a pre-built component tree for pageURL: it
// selects the matching view (falling back to global components), compiles the
// queries if not already cached, then runs the NFA matcher over root. Returns
// nil when root is nil or no queries apply.
func (m *Matcher) Match(root *sightmap.ComponentNode, pageURL string) map[*sightmap.ComponentNode]*sightmap.ComponentMatch {
	entry := m.entryFor(pageURL)
	if root == nil || len(entry.queries) == 0 {
		return nil
	}

	result := make(map[*sightmap.ComponentNode]*sightmap.ComponentMatch)
	defByNode := make(map[*sightmap.ComponentNode]*sightmap.ComponentDef)
	FindAllMatches(root, entry.queries, func(node *sightmap.ComponentNode, q *MatchQuery) {
		if _, already := result[node]; already {
			return // first-match-wins
		}
		cm := &sightmap.ComponentMatch{Name: q.Name}
		if q.Def != nil {
			cm.Memory = q.Def.Memory
			cm.Tags = q.Def.Tags
			defByNode[node] = q.Def
		}
		result[node] = cm
	})

	// Resolve declared component properties over the matched tree (SEP-0010):
	// text/attr read the node itself; PATH.prop and exists:PATH resolve a
	// descendant matched component. No live DOM is required.
	resolveComponentProperties(result, defByNode)
	return result
}

// Components returns the merged component list for pageURL (view components plus
// non-colliding globals) — the compiled inventory, for tools that need the
// definitions without a tree to match against. Cached alongside the queries.
func (m *Matcher) Components(pageURL string) []sightmap.ComponentDef {
	return m.entryFor(pageURL).components
}

// Conflicts returns the nodes in root directly matched by more than one distinct
// component name for pageURL. A single name matching many nodes (e.g. a list of
// cards) is normal and never reported; a single node claimed by several names is
// the ambiguity, since Match is first-match-wins and keeps only the first. It
// reuses the same cached queries as Match, so it sees exactly the same matches.
func (m *Matcher) Conflicts(root *sightmap.ComponentNode, pageURL string) []sightmap.Conflict {
	entry := m.entryFor(pageURL)
	if root == nil || len(entry.queries) == 0 {
		return nil
	}

	namesByNode := make(map[*sightmap.ComponentNode][]string)
	var order []*sightmap.ComponentNode
	FindAllMatches(root, entry.queries, func(node *sightmap.ComponentNode, q *MatchQuery) {
		names := namesByNode[node]
		for _, n := range names {
			if n == q.Name {
				return // count distinct names only
			}
		}
		if len(names) == 0 {
			order = append(order, node)
		}
		namesByNode[node] = append(names, q.Name)
	})

	var out []sightmap.Conflict
	for _, node := range order {
		if names := namesByNode[node]; len(names) >= 2 {
			out = append(out, sightmap.Conflict{Node: node, Names: names})
		}
	}
	return out
}
