package main

import (
	"sort"
	"testing"

	"github.com/sightmap/sightmap/go/compquery"
	"github.com/sightmap/sightmap/go/sightmap"
)

// TestSelectBoundsResults exercises selectBoundsResults, the pure selection
// core of `bounds` --all/--substring/query-grammar modes, against a fixture
// tree with a descendant relationship (Row > Star) and two Card instances
// disambiguated by a [title] property. The descendant-chain and predicate
// cases are the exact scenarios issue #273 reported as "matched no
// component" before bounds routed through compquery.
func TestSelectBoundsResults(t *testing.T) {
	box := &sightmap.Bounds{X: 0, Y: 0, Width: 10, Height: 10}

	star := &sightmap.ComponentNode{Id: "star", Bounds: box}
	row := &sightmap.ComponentNode{Id: "row", Bounds: box, Children: []*sightmap.ComponentNode{star}}
	cardWidgets := &sightmap.ComponentNode{Id: "card-widgets", Bounds: box}
	cardOther := &sightmap.ComponentNode{Id: "card-other", Bounds: box}
	cardOffscreen := &sightmap.ComponentNode{Id: "card-offscreen"} // no Bounds: never selectable
	root := &sightmap.ComponentNode{
		Id:       "root",
		Children: []*sightmap.ComponentNode{row, cardWidgets, cardOther, cardOffscreen},
	}

	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		row:           {Name: "Row"},
		star:          {Name: "Star"},
		cardWidgets:   {Name: "Card"},
		cardOther:     {Name: "Card"},
		cardOffscreen: {Name: "Card"},
	}
	props := map[string]map[string]string{
		cardWidgets.Id: {"title": "Widgets"},
		cardOther.Id:   {"title": "Gadgets"},
	}

	cases := []struct {
		name      string
		all       bool
		substring bool
		queries   []string
		wantIDs   []string
	}{
		{
			name:    "--all returns every matched component that has bounds",
			all:     true,
			wantIDs: []string{"row", "star", "card-widgets", "card-other"},
		},
		{
			name:      "--substring matches a case-insensitive name fragment",
			substring: true,
			queries:   []string{"row"},
			wantIDs:   []string{"row"},
		},
		{
			name:    "query grammar: a [prop] predicate narrows to one Card instance",
			queries: []string{`Card[title="Widgets"]`},
			wantIDs: []string{"card-widgets"},
		},
		{
			name:    "query grammar: a descendant chain resolves through compquery (issue #273)",
			queries: []string{"Row Star"},
			wantIDs: []string{"star"},
		},
		{
			name:    "query grammar: a name with no match returns nothing",
			queries: []string{"NoSuchComponent"},
			wantIDs: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var parsed []*compquery.Query
			var queryProps map[string]map[string]string
			if !tc.all && !tc.substring {
				queryProps = props
				for _, qs := range tc.queries {
					q, err := compquery.ParseQuery(qs)
					if err != nil {
						t.Fatalf("parse %q: %v", qs, err)
					}
					parsed = append(parsed, q)
				}
			}

			results := selectBoundsResults(root, matches, tc.queries, tc.all, tc.substring, parsed, queryProps, 100, 100)

			var gotIDs []string
			for _, r := range results {
				gotIDs = append(gotIDs, r.Id)
			}
			sort.Strings(gotIDs)
			wantIDs := append([]string(nil), tc.wantIDs...)
			sort.Strings(wantIDs)

			if len(gotIDs) != len(wantIDs) {
				t.Fatalf("got IDs %v, want %v", gotIDs, wantIDs)
			}
			for i := range gotIDs {
				if gotIDs[i] != wantIDs[i] {
					t.Errorf("got IDs %v, want %v", gotIDs, wantIDs)
					break
				}
			}
		})
	}
}
