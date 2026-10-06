package match

import (
	"slices"
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
	corpus        *sightmap.Corpus
	consumerRules []PrivacyRule
	mu            sync.Mutex
	cache         map[string]*queryCacheEntry
}

// NewMatcher returns a Matcher bound to corpus.
func NewMatcher(corpus *sightmap.Corpus, opts ...Option) *Matcher {
	m := &Matcher{corpus: corpus}
	for _, o := range opts {
		o(m)
	}
	return m
}

// queryCacheEntry stores the merged component list and compiled queries for one
// page URL. Compilation is the expensive step, so it is cached per URL.
type queryCacheEntry struct {
	components []sightmap.ComponentDef
	queries    []MatchQuery
	index      *firstPartIndex
	capture    *captureSet
	// combined is queries followed by the capture rules, so Match resolves
	// names, privacy and watch in one traversal.
	combined      []MatchQuery
	combinedIndex *firstPartIndex
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
	e := &queryCacheEntry{
		components: compList,
		queries:    queries,
		index:      newFirstPartIndex(queries),
		capture:    compileCapture(captureDefsForURL(m.corpus, pageURL), m.consumerRules),
	}
	e.combined = append(append([]MatchQuery(nil), queries...), e.capture.queries...)
	e.combinedIndex = newFirstPartIndex(e.combined)
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

	// Names, privacy and watch resolve in one traversal. A node's matches all
	// arrive before onNode fires for it, so per-node state needs no maps, and
	// privacy folds down a depth-indexed stack (SEP-0009). A capture rule never
	// names a node, whatever its order.
	var (
		named    *sightmap.ComponentMatch // the node's first naming match
		local    int                      // strictest privacy declared on the node
		watched  []string                 // watched components matching the node
		watchDef []*sightmap.ComponentDef // their definitions, for identity dedup
		eff      []int                    // effective privacy by depth
	)
	onMatch := func(node *sightmap.ComponentNode, q *MatchQuery) {
		if q.privacyRank != rankNone || q.watch {
			local = max(local, q.privacyRank)
			// Dedupe by definition, not name: a component with several
			// selectors is recorded once, and two distinct components sharing a
			// name are both recorded.
			if q.watch && !slices.Contains(watchDef, q.Def) {
				watchDef = append(watchDef, q.Def)
				watched = append(watched, q.Name)
			}
			return
		}
		if named != nil {
			return // first-match-wins
		}
		named = &sightmap.ComponentMatch{Name: q.Name}
		if q.Def != nil {
			named.Memory = q.Def.Memory
			named.Tags = q.Def.Tags
			defByNode[node] = q.Def
		}
		result[node] = named
	}
	onNode := func(_ *sightmap.ComponentNode, depth int) {
		parent := rankNone
		if depth > 0 {
			parent = eff[depth-1]
		}
		e := max(foldRank(parent, local), entry.capture.floor)
		eff = append(eff[:depth], e)
		if named != nil {
			named.Privacy = rankNames[e]
			if len(watched) > 0 {
				slices.Sort(watched)
				named.Watched, named.Watch = watched, true
			}
		}
		named, local, watched, watchDef = nil, rankNone, nil, nil
	}
	walkMatches(root, entry.combined, entry.combinedIndex, onMatch, onNode)

	// Declared component properties resolve over the matched tree (SEP-0010):
	// dom.* sources read the node itself; component and component.exists resolve
	// a descendant matched component. No live DOM is required.
	privacy := func(n *sightmap.ComponentNode) string {
		if cm := result[n]; cm != nil {
			return cm.Privacy
		}
		return ""
	}
	resolveComponentProperties(result, defByNode, privacy)
	return result
}

// Components returns the merged component list for pageURL (view components plus
// non-colliding globals) — the compiled inventory, for tools that need the
// definitions without a tree to match against. Cached alongside the queries.
func (m *Matcher) Components(pageURL string) []sightmap.ComponentDef {
	return m.entryFor(pageURL).components
}

// Conflicts returns the nodes in root directly matched by more than one distinct
// component DEFINITION for pageURL. One definition matching many nodes (e.g. a
// list of cards) is normal and never reported; one node claimed by several
// definitions is the ambiguity, since Match is first-match-wins and keeps only
// the first. It reuses the same cached queries as Match, so it sees exactly the
// same matches.
//
// Claims are deduplicated by DEFINITION, not by name. A component name is unique
// only within its parent, so deduplicating by name silently discarded every
// conflict between two same-named definitions — which is not a corner case: a
// production sign-in corpus had five nodes each claimed by two definitions that
// happened to share a name, and this reported none of them. MatchQuery carries
// Def for exactly this reason. A definition with several alternative selectors
// still counts once, since they are one definition, and so do a global and its
// $ref expansions, which are copies of one definition.
func (m *Matcher) Conflicts(root *sightmap.ComponentNode, pageURL string) []sightmap.Conflict {
	entry := m.entryFor(pageURL)
	if root == nil || len(entry.queries) == 0 {
		return nil
	}

	type claims struct {
		defs  []*sightmap.ComponentDef
		names []string
	}
	byNode := make(map[*sightmap.ComponentNode]*claims)
	var order []*sightmap.ComponentNode
	findAllMatches(root, entry.queries, entry.index, func(node *sightmap.ComponentNode, q *MatchQuery) {
		c := byNode[node]
		if c == nil {
			c = &claims{}
			byNode[node] = c
			order = append(order, node)
		}
		for i, d := range c.defs {
			if sameClaimant(d, q.Def, c.names[i], q.Name) {
				return
			}
		}
		c.defs = append(c.defs, q.Def)
		c.names = append(c.names, q.Name)
	})

	var out []sightmap.Conflict
	for _, node := range order {
		if c := byNode[node]; len(c.defs) >= 2 {
			out = append(out, sightmap.Conflict{Node: node, Names: c.names, Defs: c.defs})
		}
	}
	return out
}

// sameClaimant reports whether two claims on a node come from one definition.
// Defs instantiated from the same global (the global itself and each $ref
// expansion of it) share an Origin; any other def is identified by its pointer.
// Queries built without a Def (never the case via ParseQueries) fall back to
// comparing names.
func sameClaimant(a, b *sightmap.ComponentDef, aName, bName string) bool {
	switch {
	case a == nil || b == nil:
		return a == nil && b == nil && aName == bName
	case a == b:
		return true
	default:
		return a.Origin != "" && a.Origin == b.Origin
	}
}
