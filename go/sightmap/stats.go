package sightmap

import (
	"strings"

	"github.com/sightmap/sightmap/go/match"
)

// Stats is a count summary of a loaded Corpus: corpus-wide totals plus a
// per-view breakdown. It is derived from the corpus model, not from a fresh
// YAML walk — the loader has already expanded $refs and flattened hierarchies,
// so the counts describe what a consumer of the Corpus actually sees.
//
// The JSON field names are a published contract (`sightmap stats --json` and
// the atlas index generator both consume them) — never rename them.
//
// Two different totals are deliberately in play, because "how many components"
// has two useful answers:
//
//   - Components counts distinct component NAMES corpus-wide (see
//     Corpus.AllComponents): the size of the corpus vocabulary. A global
//     reused by three views is one component.
//   - Properties and Memory are summed over distinct component DEFINITIONS:
//     two definitions collapse only when they are byte-identical, so a
//     $ref-expanded copy of a global counts once (the expansion is an exact
//     copy) while two views that each define a different component under the
//     same local name — legal, since only global name collisions are
//     rejected — both count. Summing these over the name-deduped list instead
//     would silently drop the second view's properties and memory.
//
// A $ref expanded beneath a parent is not an exact copy (its selectors are
// scoped to the parent), so it is its own definition and its properties and
// memory count again — it is a distinct extraction site.
type Stats struct {
	Totals
	PerView []ViewStats `json:"per_view"` // one row per view, in corpus order
}

// Totals is the corpus-wide count summary without the per-view breakdown.
//
// It is split out from Stats because the two travel separately once published.
// The atlas catalog carries these five numbers as an entry's `stats` object and
// puts the per-view rows in a sibling field, so a catalog reader wants exactly
// this shape — embedding Stats there would give it a PerView that no valid
// catalog can ever populate. Embedding keeps `sightmap stats --json` emitting
// all six keys at one level, unchanged.
//
// The JSON field names are a published contract (`sightmap stats --json`, the
// atlas index generator, and the atlas package's catalog decoding all depend on
// them) — never rename them.
type Totals struct {
	Views      int `json:"views"`      // number of views
	Components int `json:"components"` // distinct component names corpus-wide
	Requests   int `json:"requests"`   // global + view-scoped request definitions
	Properties int `json:"properties"` // properties over distinct component definitions
	Memory     int `json:"memory"`     // file-, view-, component-, and request-level entries
}

// ViewStats is one per-view row: the components and requests reachable in that
// view after $ref expansion. A global component reused by several views appears
// in each view's row but only once in Stats.Components, and global (view-less)
// components and requests appear in the totals only — so the per-view columns
// need not sum to the totals.
type ViewStats struct {
	Name       string `json:"name"`
	Route      string `json:"route"`
	Components int    `json:"components"`
	Requests   int    `json:"requests"`
}

// IsEmpty reports whether the corpus carries nothing worth counting: no views,
// no components, no requests, and no memory entries. A corpus that holds only
// memory is legal (Validate accepts it) and is NOT empty.
func (s Stats) IsEmpty() bool {
	return s.Views == 0 && s.Components == 0 && s.Requests == 0 && s.Memory == 0
}

// Stats folds the corpus into its count summary. See Stats for exactly what
// each total means; the short version is that Components counts distinct names
// while Properties and Memory are summed over distinct definitions.
//
// Stats counts what the loader produced. A corpus with load-time errors (an
// unresolved $ref, a component missing its name or selector) has already had
// those definitions dropped, so its counts are an under-report; callers that
// need trustworthy numbers should run Validate first and refuse error-severity
// findings.
func (c *Corpus) Stats() Stats {
	s := Stats{
		Totals: Totals{
			Views:  len(c.Views),
			Memory: len(c.Memory),
		},
		PerView: make([]ViewStats, 0, len(c.Views)),
	}

	s.Components = len(c.AllComponents())

	// Properties and memory are summed over distinct definitions, so that two
	// same-named local components each contribute while a $ref-expanded copy
	// of a global does not double-count.
	seen := make(map[string]bool)
	countComponent := func(comp match.ComponentDef) {
		if comp.Name == "" {
			return
		}
		key := componentIdentity(comp)
		if seen[key] {
			return
		}
		seen[key] = true
		s.Properties += len(comp.Properties)
		s.Memory += len(comp.Memory)
	}
	for _, gc := range c.GlobalComponents {
		countComponent(gc)
	}

	countRequests := func(defs []RequestDef) {
		s.Requests += len(defs)
		for _, rd := range defs {
			s.Memory += len(rd.Memory)
		}
	}
	countRequests(c.Requests)

	for i := range c.Views {
		v := &c.Views[i]
		s.Memory += len(v.Memory)
		for _, vc := range v.Components {
			countComponent(vc)
		}
		countRequests(v.Requests)
		s.PerView = append(s.PerView, ViewStats{
			Name:       v.Name,
			Route:      v.Route,
			Components: len(v.Components),
			Requests:   len(v.Requests),
		})
	}
	return s
}

// componentIdentity builds the key that decides whether two flattened
// ComponentDefs are the same definition. It covers every field of the
// definition, so an exact copy (what a top-level $ref expands to) collapses
// while anything an author wrote differently does not.
func componentIdentity(c match.ComponentDef) string {
	var b strings.Builder
	writeField := func(parts ...string) {
		for _, p := range parts {
			b.WriteString(p)
			b.WriteByte(0x1f) // unit separator: cannot appear in a YAML scalar
		}
		b.WriteByte(0x1e) // record separator: ends this field
	}
	writeField(c.Name, c.Source, c.Stability)
	writeField(c.Selectors...)
	writeField(c.ParentChain...)
	writeField(c.Tags...)
	writeField(c.Memory...)
	for _, p := range c.Properties {
		writeField(p.Name, p.Extract, p.Transform)
	}
	return b.String()
}
