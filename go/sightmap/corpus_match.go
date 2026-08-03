package sightmap

import (
	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/match"
)

// Load parses the .sightmap/ corpus at dir and returns an operational Corpus.
// It is shorthand for DirLoader(dir).Load(). Callers hold the returned *Corpus
// for the life of a run; reloading is just another Load (the old Corpus is
// discarded). For a long-lived process that needs to pick up on-disk edits, call
// Load again and swap the handle.
func Load(dir string) (*Corpus, error) {
	return DirLoader(dir).Load()
}

// queryCacheEntry stores the merged component list and compiled queries for one
// page URL. Compilation is the expensive step, so it is cached per URL.
type queryCacheEntry struct {
	components []match.ComponentDef
	queries    []match.MatchQuery
}

// entryFor returns the cached (or freshly compiled) queries for pageURL.
func (c *Corpus) entryFor(pageURL string) *queryCacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cache == nil {
		c.cache = make(map[string]*queryCacheEntry)
	}
	if e, ok := c.cache[pageURL]; ok {
		return e
	}
	compList := c.ComponentsForURL(pageURL)
	queries, _ := match.ParseQueries(compList)
	e := &queryCacheEntry{components: compList, queries: queries}
	c.cache[pageURL] = e
	return e
}

// MatchTree applies the corpus to a pre-built component tree for pageURL: it
// selects the matching view (falling back to global components), compiles the
// queries if not already cached, then runs the NFA matcher over root. Returns
// nil when root is nil or no queries apply.
func (c *Corpus) MatchTree(root *comps.ComponentNode, pageURL string) map[*comps.ComponentNode]*match.ComponentMatch {
	entry := c.entryFor(pageURL)
	if root == nil || len(entry.queries) == 0 {
		return nil
	}

	// Map component name → definition for memory resolution at match time.
	byName := make(map[string]*match.ComponentDef, len(entry.components))
	for i := range entry.components {
		byName[entry.components[i].Name] = &entry.components[i]
	}

	result := make(map[*comps.ComponentNode]*match.ComponentMatch)
	match.FindAllMatches(root, entry.queries, func(node *comps.ComponentNode, q *match.MatchQuery) {
		if _, already := result[node]; already {
			return // first-match-wins
		}
		var memory []string
		if def := byName[q.Name]; def != nil {
			memory = def.Memory
		}
		result[node] = &match.ComponentMatch{Name: q.Name, Memory: memory}
	})
	return result
}

// Components returns the merged component list for pageURL (view components plus
// non-colliding globals) — the compiled inventory, for tools that need the
// definitions without a tree to match against. Cached alongside the queries.
func (c *Corpus) Components(pageURL string) []match.ComponentDef {
	return c.entryFor(pageURL).components
}

// GlobalComponentNames returns the set of component names declared at corpus
// scope (components.yaml). Callers use it to exclude globally-shared components
// (Header, Footer, Nav, …) from page-specific checks.
func (c *Corpus) GlobalComponentNames() map[string]bool {
	names := make(map[string]bool, len(c.GlobalComponents))
	for _, gc := range c.GlobalComponents {
		if gc.Name != "" {
			names[gc.Name] = true
		}
	}
	return names
}
