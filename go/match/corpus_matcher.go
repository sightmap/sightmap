package match

import (
	"sync"

	"github.com/sightmap/sightmap/go/comps"
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

// MatchTree applies the corpus to a pre-built component tree for pageURL: it
// selects the matching view (falling back to global components), compiles the
// queries if not already cached, then runs the NFA matcher over root. Returns
// nil when root is nil or no queries apply.
func (m *Matcher) MatchTree(root *comps.ComponentNode, pageURL string) map[*comps.ComponentNode]*sightmap.ComponentMatch {
	entry := m.entryFor(pageURL)
	if root == nil || len(entry.queries) == 0 {
		return nil
	}

	// Map component name → definition for memory resolution at match time.
	byName := make(map[string]*sightmap.ComponentDef, len(entry.components))
	for i := range entry.components {
		byName[entry.components[i].Name] = &entry.components[i]
	}

	result := make(map[*comps.ComponentNode]*sightmap.ComponentMatch)
	FindAllMatches(root, entry.queries, func(node *comps.ComponentNode, q *MatchQuery) {
		if _, already := result[node]; already {
			return // first-match-wins
		}
		var memory []string
		if def := byName[q.Name]; def != nil {
			memory = def.Memory
		}
		result[node] = &sightmap.ComponentMatch{Name: q.Name, Memory: memory}
	})
	return result
}

// Components returns the merged component list for pageURL (view components plus
// non-colliding globals) — the compiled inventory, for tools that need the
// definitions without a tree to match against. Cached alongside the queries.
func (m *Matcher) Components(pageURL string) []sightmap.ComponentDef {
	return m.entryFor(pageURL).components
}
