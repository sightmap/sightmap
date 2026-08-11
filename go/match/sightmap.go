package match

import (
	"fmt"

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
			ps, err := sightmap.ParseSightmapSelector(selStr)
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

// (ApplySightmap and FindConflicts were folded into the Matcher API —
// Matcher.Match and Matcher.Conflicts — so the corpus's cached, per-URL queries
// are the single path into the NFA. See corpus_matcher.go.)
