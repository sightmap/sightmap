package match

import (
	"fmt"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/sel"
	"github.com/sightmap/sightmap/go/sightmap"
)

// ParseQueries parses ComponentDef definitions into MatchQuery values.
// Each selector string in each component becomes a separate MatchQuery.
// Invalid selectors are skipped; their errors are returned alongside the
// valid queries (non-fatal).
func ParseQueries(defs []sightmap.ComponentDef) ([]MatchQuery, []error) {
	var queries []MatchQuery
	var errs []error

	for _, def := range defs {
		for _, selStr := range def.Selectors {
			ps, err := sel.ParseSightmapSelector(selStr)
			if err != nil {
				errs = append(errs, fmt.Errorf("component %q selector %q: %w", def.Name, selStr, err))
				continue
			}
			queries = append(queries, MatchQuery{
				Name:        def.Name,
				Parts:       ps.Parts,
				Combinators: ps.Combinators,
			})
		}
	}

	return queries, errs
}

// ApplySightmap runs the NFA against root and returns a map from matched
// ComponentNode to the ComponentMatch that claimed it. First-match-wins: when
// multiple definitions match the same node, the one earliest in defs order
// (and earliest selector within that definition) takes precedence.
//
// Returns nil if defs is empty or root is nil.
func ApplySightmap(root *comps.ComponentNode, defs []sightmap.ComponentDef) map[*comps.ComponentNode]*sightmap.ComponentMatch {
	if root == nil || len(defs) == 0 {
		return nil
	}

	queries, _ := ParseQueries(defs)
	if len(queries) == 0 {
		return nil
	}

	// Build a lookup from query name → ComponentDef so we can resolve the
	// component definition at match time.
	defByName := make(map[string]*sightmap.ComponentDef, len(defs))
	for i := range defs {
		defByName[defs[i].Name] = &defs[i]
	}

	result := make(map[*comps.ComponentNode]*sightmap.ComponentMatch)

	FindAllMatches(root, queries, func(node *comps.ComponentNode, q *MatchQuery) {
		// First-match-wins: skip if already claimed.
		if _, already := result[node]; already {
			return
		}
		def := defByName[q.Name]
		result[node] = &sightmap.ComponentMatch{
			Name:   q.Name,
			Memory: def.Memory,
			Tags:   def.Tags,
		}
	})

	return result
}

// FindConflicts returns the nodes that more than one distinct component name
// matches directly. A single name matching many nodes (e.g. a list of cards) is
// normal and never reported; a single node claimed by several names is the
// ambiguity, since ApplySightmap keeps only the first. It reuses the same
// traversal as ApplySightmap, so it sees exactly the same matches.
func FindConflicts(root *comps.ComponentNode, defs []sightmap.ComponentDef) []sightmap.Conflict {
	if root == nil || len(defs) == 0 {
		return nil
	}
	queries, _ := ParseQueries(defs)
	if len(queries) == 0 {
		return nil
	}

	namesByNode := make(map[*comps.ComponentNode][]string)
	var order []*comps.ComponentNode
	FindAllMatches(root, queries, func(node *comps.ComponentNode, q *MatchQuery) {
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
