package main

import (
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

	if got := props[card.Id]["title"]; got != "Widgets" {
		t.Errorf("card title: got %q, want %q", got, "Widgets")
	}
	if _, ok := props[row.Id]; ok {
		t.Errorf("row carries no property values, should be absent from the map")
	}
	if got := props[other.Id]["leftover"]; got != "stale" {
		t.Errorf("other leftover: got %q, want %q", got, "stale")
	}
}
