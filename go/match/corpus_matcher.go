package match

import (
	"slices"
	"strconv"
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
	index      *firstPartIndex
	capture    *captureSet
	// combined is queries followed by the capture rules, so Match resolves
	// names, privacy and watch in one traversal.
	combined      []MatchQuery
	combinedIndex *firstPartIndex
	// narrow orders this entry's queries by selector specificity, so a node
	// claimed by several definitions goes to the most specific (SEP-0023).
	narrow specificity
}

// entryFor returns the cached (or freshly compiled) queries for pageURL.
func (m *Matcher) entryFor(pageURL string) *queryCacheEntry {
	return m.cached(pageURL, func() *sightmap.ViewDef { return m.corpus.ViewForURL(pageURL) })
}

// entryForView returns the cached (or freshly compiled) queries for a view
// chosen without a URL. Its cache key cannot collide with a URL's, which never
// starts with a NUL byte. View names need not be unique, so a view of this
// corpus is keyed by its position; any other view is not cached.
func (m *Matcher) entryForView(view *sightmap.ViewDef) *queryCacheEntry {
	key := "\x00"
	if view != nil {
		i := -1
		for j := range m.corpus.Views {
			if &m.corpus.Views[j] == view {
				i = j
				break
			}
		}
		if i < 0 {
			return m.compile(view)
		}
		key += strconv.Itoa(i)
	}
	return m.cached(key, func() *sightmap.ViewDef { return view })
}

// cached returns the entry cached under key, compiling it for the view that
// resolve names on a miss.
func (m *Matcher) cached(key string, resolve func() *sightmap.ViewDef) *queryCacheEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cache == nil {
		m.cache = make(map[string]*queryCacheEntry)
	}
	if e, ok := m.cache[key]; ok {
		return e
	}
	e := m.compile(resolve())
	m.cache[key] = e
	return e
}

// compile builds the queries for view's components (plus globals).
func (m *Matcher) compile(view *sightmap.ViewDef) *queryCacheEntry {
	compList := m.corpus.ComponentsForView(view)
	queries, _ := ParseQueries(compList)
	e := &queryCacheEntry{
		components: compList,
		queries:    queries,
		index:      newFirstPartIndex(queries),
		capture:    compileCapture(captureDefsForView(m.corpus, view)),
	}
	e.combined = append(append([]MatchQuery(nil), queries...), e.capture.queries...)
	e.combinedIndex = newFirstPartIndex(e.combined)
	return e
}

// Match applies the corpus to a pre-built component tree for pageURL: it
// selects the matching view (falling back to global components), compiles the
// queries if not already cached, then runs the NFA matcher over root. Returns
// nil when root is nil or no queries apply.
func (m *Matcher) Match(root *sightmap.ComponentNode, pageURL string) map[*sightmap.ComponentNode]*sightmap.ComponentMatch {
	return matchEntry(root, m.entryFor(pageURL))
}

// MatchView is Match for a tree that has no URL to select a view by, such as a
// native app screen: it applies view's components (plus non-colliding globals),
// or the globals alone when view is nil. The caller chooses the view.
func (m *Matcher) MatchView(root *sightmap.ComponentNode, view *sightmap.ViewDef) map[*sightmap.ComponentNode]*sightmap.ComponentMatch {
	return matchEntry(root, m.entryForView(view))
}

func matchEntry(root *sightmap.ComponentNode, entry *queryCacheEntry) map[*sightmap.ComponentNode]*sightmap.ComponentMatch {
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
		claims   []*MatchQuery            // the node's naming matches, in match order
		named    *sightmap.ComponentMatch // the node's name, resolved in onNode
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
		claims = append(claims, q)
	}
	onNode := func(node *sightmap.ComponentNode, depth int) {
		parent := rankNone
		if depth > 0 {
			parent = eff[depth-1]
		}
		e := foldRank(parent, local)
		eff = append(eff[:depth], e)
		// The most specific claim names the node; among claims the selector text
		// cannot order, the first in match order does, as before.
		if claims = entry.narrow.mostSpecific(claims); len(claims) > 0 {
			q := claims[0]
			named = &sightmap.ComponentMatch{Name: q.Name}
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
		claims, named, local, watched, watchDef = claims[:0], nil, rankNone, nil, nil
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
// definitions is the ambiguity, since Match keeps only one. A claim that
// another claim is strictly more specific than is not an ambiguity (SEP-0023):
// the more specific definition names it, as Match resolves it. It reuses the same
// cached queries as Match, so it sees exactly the same matches.
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
	return conflictsEntry(root, m.entryFor(pageURL))
}

// ConflictsView is Conflicts for a view chosen without a URL; see MatchView.
func (m *Matcher) ConflictsView(root *sightmap.ComponentNode, view *sightmap.ViewDef) []sightmap.Conflict {
	return conflictsEntry(root, m.entryForView(view))
}

func conflictsEntry(root *sightmap.ComponentNode, entry *queryCacheEntry) []sightmap.Conflict {
	if root == nil || len(entry.queries) == 0 {
		return nil
	}

	byNode := make(map[*sightmap.ComponentNode][]*MatchQuery)
	var order []*sightmap.ComponentNode
	findAllMatches(root, entry.queries, entry.index, func(node *sightmap.ComponentNode, q *MatchQuery) {
		if _, ok := byNode[node]; !ok {
			order = append(order, node)
		}
		byNode[node] = append(byNode[node], q)
	})

	var out []sightmap.Conflict
	for _, node := range order {
		var c sightmap.Conflict
		for _, q := range entry.narrow.mostSpecific(byNode[node]) {
			dup := false
			for i, d := range c.Defs {
				if sameClaimant(d, q.Def, c.Names[i], q.Name) {
					dup = true
					break
				}
			}
			if !dup {
				c.Defs = append(c.Defs, q.Def)
				c.Names = append(c.Names, q.Name)
			}
		}
		if len(c.Defs) >= 2 {
			c.Node = node
			out = append(out, c)
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
