package sightmap

import (
	"sync"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/match"
)

// Session holds a loaded Corpus and caches compiled MatchQuery slices.
// Safe for concurrent use.
type Session struct {
	loader Loader

	// mu guards corpus and generation.
	mu         sync.RWMutex
	corpus     *Corpus
	generation uint64

	// cacheMu guards the query cache.
	cacheMu sync.RWMutex
	cache   map[cacheKey]*cacheEntry
}

// cacheKey uniquely identifies a (corpus version, page URL) pair.
type cacheKey struct {
	generation uint64
	url        string
}

// cacheEntry stores the precomputed component list and compiled queries for
// one (corpus generation, page URL) pair.
type cacheEntry struct {
	components []match.SightmapComponent
	queries    []match.MatchQuery
}

// NewSession creates a Session backed by loader. The corpus is loaded lazily
// on first use.
func NewSession(loader Loader) *Session {
	return &Session{
		loader: loader,
		cache:  make(map[cacheKey]*cacheEntry),
	}
}

// Refresh reloads the corpus and invalidates the query cache.
func (s *Session) Refresh() error {
	corpus, err := s.loader.Load()
	if err != nil {
		return err
	}

	// Bump the generation counter atomically with the corpus swap so that any
	// in-flight MatchTree calls using the old generation will populate orphaned
	// cache entries that are never looked up again.
	s.mu.Lock()
	s.corpus = corpus
	s.generation++
	s.mu.Unlock()

	// Clear the cache to reclaim memory; orphaned (old-generation) entries
	// would never be hit anyway, but clearing keeps memory bounded.
	s.cacheMu.Lock()
	s.cache = make(map[cacheKey]*cacheEntry)
	s.cacheMu.Unlock()

	return nil
}

// MatchTree applies the sightmap to a pre-built component tree for pageURL.
// It selects the matching view (falling back to global components), compiles
// queries if not already cached, then runs the NFA matcher over root.
func (s *Session) MatchTree(
	root *comps.ComponentNode,
	pageURL string,
) (map[*comps.ComponentNode]*match.SightmapMatch, error) {
	entry, err := s.getCacheEntry(pageURL)
	if err != nil {
		return nil, err
	}

	if root == nil || len(entry.queries) == 0 {
		return nil, nil
	}

	// Map component name → definition for memory resolution at match time.
	byName := make(map[string]*match.SightmapComponent, len(entry.components))
	for i := range entry.components {
		byName[entry.components[i].Name] = &entry.components[i]
	}

	result := make(map[*comps.ComponentNode]*match.SightmapMatch)
	match.FindAllMatches(root, entry.queries, func(node *comps.ComponentNode, q *match.MatchQuery) {
		if _, already := result[node]; already {
			return // first-match-wins
		}
		var memory []string
		if def := byName[q.Name]; def != nil {
			memory = def.Memory
		}
		result[node] = &match.SightmapMatch{
			Name:   q.Name,
			Memory: memory,
		}
	})

	return result, nil
}

// Components returns the merged component list for pageURL (for tools that
// need the component definitions without a tree to match against).
func (s *Session) Components(pageURL string) ([]match.SightmapComponent, error) {
	entry, err := s.getCacheEntry(pageURL)
	if err != nil {
		return nil, err
	}
	return entry.components, nil
}

// GlobalComponentNames returns the set of component names declared in the
// corpus-level components.yaml (global scope). Callers use this to exclude
// globally-shared components (Header, Footer, Nav, etc.) from page-specific
// checks that should only consider view-local components.
func (s *Session) GlobalComponentNames() (map[string]bool, error) {
	corpus, _, err := s.getCorpus()
	if err != nil {
		return nil, err
	}
	names := make(map[string]bool, len(corpus.GlobalComponents))
	for _, gc := range corpus.GlobalComponents {
		if gc.Name != "" {
			names[gc.Name] = true
		}
	}
	return names, nil
}

// ViewForURL returns the matching View for pageURL, or nil if only globals match.
func (s *Session) ViewForURL(pageURL string) (*View, error) {
	corpus, _, err := s.getCorpus()
	if err != nil {
		return nil, err
	}
	return corpus.ViewForURL(pageURL), nil
}

// ---- internal helpers -------------------------------------------------------

// getCorpus returns the current corpus and generation, loading it lazily on
// first call using the double-checked locking pattern.
func (s *Session) getCorpus() (*Corpus, uint64, error) {
	// Fast path: already loaded.
	s.mu.RLock()
	corpus, gen := s.corpus, s.generation
	s.mu.RUnlock()
	if corpus != nil {
		return corpus, gen, nil
	}

	// Slow path: first load.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.corpus != nil { // double-check
		return s.corpus, s.generation, nil
	}
	c, err := s.loader.Load()
	if err != nil {
		return nil, 0, err
	}
	s.corpus = c
	return c, s.generation, nil
}

// getCacheEntry returns the cached (or freshly computed) entry for pageURL at
// the current corpus generation.
func (s *Session) getCacheEntry(pageURL string) (*cacheEntry, error) {
	corpus, gen, err := s.getCorpus()
	if err != nil {
		return nil, err
	}

	key := cacheKey{generation: gen, url: pageURL}

	// Fast path: cache hit.
	s.cacheMu.RLock()
	entry, ok := s.cache[key]
	s.cacheMu.RUnlock()
	if ok {
		return entry, nil
	}

	// Cache miss: build the entry outside of any lock (pure computation).
	compList := corpus.ComponentsForURL(pageURL)
	queries, _ := match.ParseQueries(compList)
	newEntry := &cacheEntry{components: compList, queries: queries}

	s.cacheMu.Lock()
	if existing, ok := s.cache[key]; ok {
		// Another goroutine beat us — use its entry for consistency.
		newEntry = existing
	} else {
		s.cache[key] = newEntry
	}
	s.cacheMu.Unlock()

	return newEntry, nil
}
