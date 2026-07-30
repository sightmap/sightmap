package match

import (
	"fmt"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/sel"
)

// SightmapComponent is a single flattened sightmap component definition.
// Hierarchical YAML selectors should be pre-flattened by the caller into
// compound descendant selectors before calling ParseQueries.
type SightmapComponent struct {
	Name        string     `json:"name"`
	Selectors   []string   `json:"selectors"`
	Source      string     `json:"source,omitempty"`
	Memory      []string   `json:"memory,omitempty"`
	Tags        []string   `json:"tags,omitempty"`
	Properties  []Property `json:"properties,omitempty"`
	ParentChain []string   `json:"parentChain,omitempty"` // ancestor component names, root-first
	Stability   string     `json:"stability,omitempty"`   // "" (default), "uncertain", or "unstable"
}

// Property describes a value to extract from a matched DOM element.
type Property struct {
	Name      string `json:"name"`
	Extract   string `json:"extract"`   // see extract modes: text, inner_text, text_only, attr=NAME, exists:SEL, CSS selector
	Transform string `json:"transform"` // optional post-processing
}

// SightmapMatch records which component definition matched a node.
type SightmapMatch struct {
	Name   string
	Memory []string
	Tags   []string
}

// ParseQueries parses SightmapComponent definitions into MatchQuery values.
// Each selector string in each component becomes a separate MatchQuery.
// Invalid selectors are skipped; their errors are returned alongside the
// valid queries (non-fatal).
func ParseQueries(defs []SightmapComponent) ([]MatchQuery, []error) {
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
// ComponentNode to the SightmapMatch that claimed it. First-match-wins: when
// multiple definitions match the same node, the one earliest in defs order
// (and earliest selector within that definition) takes precedence.
//
// Returns nil if defs is empty or root is nil.
func ApplySightmap(root *comps.ComponentNode, defs []SightmapComponent) map[*comps.ComponentNode]*SightmapMatch {
	if root == nil || len(defs) == 0 {
		return nil
	}

	queries, _ := ParseQueries(defs)
	if len(queries) == 0 {
		return nil
	}

	// Build a lookup from query name → SightmapMatch so we can resolve the
	// component definition at match time.
	defByName := make(map[string]*SightmapComponent, len(defs))
	for i := range defs {
		defByName[defs[i].Name] = &defs[i]
	}

	result := make(map[*comps.ComponentNode]*SightmapMatch)

	FindAllMatches(root, queries, func(node *comps.ComponentNode, q *MatchQuery) {
		// First-match-wins: skip if already claimed.
		if _, already := result[node]; already {
			return
		}
		def := defByName[q.Name]
		result[node] = &SightmapMatch{
			Name:   q.Name,
			Memory: def.Memory,
			Tags:   def.Tags,
		}
	})

	return result
}

// Conflict records a DOM node directly matched by more than one DISTINCT
// component name. ApplySightmap is first-match-wins, so only Names[0] actually
// claims the node; the rest are silently dropped. Names are in first-seen
// (definition) order.
type Conflict struct {
	Node  *comps.ComponentNode
	Names []string
}

// FindConflicts returns the nodes that more than one distinct component name
// matches directly. A single name matching many nodes (e.g. a list of cards) is
// normal and never reported; a single node claimed by several names is the
// ambiguity, since ApplySightmap keeps only the first. It reuses the same
// traversal as ApplySightmap, so it sees exactly the same matches.
func FindConflicts(root *comps.ComponentNode, defs []SightmapComponent) []Conflict {
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

	var out []Conflict
	for _, node := range order {
		if names := namesByNode[node]; len(names) >= 2 {
			out = append(out, Conflict{Node: node, Names: names})
		}
	}
	return out
}
