package main

import (
	"reflect"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// TestQueryPropertyValues pins down how the matcher's already-resolved component
// properties (SEP-0010) project into the node-id keyed map that
// compquery.Resolve/FindCandidates consume: every match carrying property values
// is included (keyed by node id), and a match with no values is omitted. Values
// are resolved offline by the matcher, so there is no live-DOM pass here.
func TestQueryPropertyValues(t *testing.T) {
	card := &sightmap.ComponentNode{Id: "n1"}
	row := &sightmap.ComponentNode{Id: "n2"}
	other := &sightmap.ComponentNode{Id: "n4"}

	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		card:  {Name: "Card", Properties: []sightmap.PropertyValue{{Name: "title", Value: "Widgets"}}},
		row:   {Name: "Row"}, // no values → omitted
		other: {Name: "Other", Properties: []sightmap.PropertyValue{{Name: "leftover", Value: "stale"}}},
	}
	props := queryPropertyValues(matches)

	cases := []struct {
		name   string
		nodeID string
		want   map[string]string // nil means the node must be absent from props
	}{
		{name: "match with a property value is projected", nodeID: card.Id, want: map[string]string{"title": "Widgets"}},
		{name: "match with no property values is omitted", nodeID: row.Id, want: nil},
		{name: "value survives regardless of which component matched", nodeID: other.Id, want: map[string]string{"leftover": "stale"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := props[tc.nodeID]
			if tc.want == nil {
				if ok {
					t.Errorf("got %v, want node absent from props", got)
				}
				return
			}
			if !ok || !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
