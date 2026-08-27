package main

import (
	"context"
	"testing"

	"github.com/sightmap/sightmap/go/compquery"
	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// TestExtractQueryProperties pins down the logic shared by resolveComponentQuery
// and boundsByComponent: which nodes get probed for [prop] predicates (only
// those whose matched name is referenced by one of the given queries) and how
// the result projects into the node-id keyed map compquery.Resolve/FindCandidates
// consume.
//
// conn is nil throughout: none of the fixture's ComponentDefs declare
// Properties/Selectors, so observe.ExtractProperties finds no specs to probe and
// returns before ever touching conn — that's what makes this unit-testable
// without a live browser.
func TestExtractQueryProperties(t *testing.T) {
	corpus := &sightmap.Corpus{
		GlobalComponents: []sightmap.ComponentDef{
			{Name: "Card"},
			{Name: "Row"},
			{Name: "Star"},
			{Name: "Other"},
		},
	}
	matcher := match.NewMatcher(corpus)

	card := &sightmap.ComponentNode{Id: "n1"}
	row := &sightmap.ComponentNode{Id: "n2"}
	star := &sightmap.ComponentNode{Id: "n3"}
	other := &sightmap.ComponentNode{Id: "n4"}

	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		// Queried, and already carries a property value (as if a prior
		// extraction pass had populated it).
		card: {Name: "Card", Properties: []sightmap.PropertyValue{{Name: "title", Value: "Widgets"}}},
		// Queried, but has no property values yet.
		row: {Name: "Row"},
		// Queried, with a property value.
		star: {Name: "Star", Properties: []sightmap.PropertyValue{{Name: "filled", Value: "true"}}},
		// NOT referenced by any query, but already carries a property value.
		other: {Name: "Other", Properties: []sightmap.PropertyValue{{Name: "leftover", Value: "stale"}}},
	}

	cardQuery, err := compquery.ParseQuery(`Card[title="Widgets"]`)
	if err != nil {
		t.Fatalf("parse Card query: %v", err)
	}
	rowQuery, err := compquery.ParseQuery("Row")
	if err != nil {
		t.Fatalf("parse Row query: %v", err)
	}

	props := extractQueryProperties(context.Background(), nil, matcher, matches, "https://example.com/", cardQuery, rowQuery)

	cases := []struct {
		name       string
		nodeID     string
		wantProp   string
		wantVal    string
		wantAbsent bool
	}{
		{name: "queried node with a property surfaces it", nodeID: card.Id, wantProp: "title", wantVal: "Widgets"},
		{name: "queried node with no properties yet is absent", nodeID: row.Id, wantAbsent: true},
		{name: "unqueried node with a pre-existing property still surfaces (final map isn't filtered by query names)", nodeID: other.Id, wantProp: "leftover", wantVal: "stale"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pm, ok := props[tc.nodeID]
			if tc.wantAbsent {
				if ok {
					t.Errorf("node %s: got %v, want absent from props", tc.nodeID, pm)
				}
				return
			}
			if !ok {
				t.Fatalf("node %s: missing from props (got %v)", tc.nodeID, props)
			}
			if got := pm[tc.wantProp]; got != tc.wantVal {
				t.Errorf("node %s prop %q: got %q, want %q", tc.nodeID, tc.wantProp, got, tc.wantVal)
			}
		})
	}

	// Star wasn't in the queries passed to extractQueryProperties above, so its
	// pre-existing property should surface exactly like Other's (queryNames only
	// gates live-DOM probing, not what ends up in the returned map) — confirmed
	// by re-running with Star included in the query set instead.
	starQuery, err := compquery.ParseQuery("Star")
	if err != nil {
		t.Fatalf("parse Star query: %v", err)
	}
	props = extractQueryProperties(context.Background(), nil, matcher, matches, "https://example.com/", starQuery)
	if got, want := props[star.Id]["filled"], "true"; got != want {
		t.Errorf("star filled (queried case): got %q, want %q", got, want)
	}
}
