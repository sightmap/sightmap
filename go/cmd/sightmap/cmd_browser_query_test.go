package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/compquery"
	"github.com/sightmap/sightmap/go/match"
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

// A replacement selector must restore the same semantic query, rather than
// bypassing the corpus with a guessed ID. The root remains matched so this also
// exercises the common case of one stale selector in an otherwise useful map.
func TestVerifiedSelectorRepair(t *testing.T) {
	button := &sightmap.ComponentNode{Id: "2", Element: &sightmap.Element{Tag: "button", Attrs: map[string]string{"data-testid": "save-new"}}}
	root := &sightmap.ComponentNode{Id: "1", Element: &sightmap.Element{Tag: "main"}, Children: []*sightmap.ComponentNode{button}}
	q, err := compquery.ParseQuery("Editor Save")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		selector string
		repaired bool
	}{{"[data-testid=save-old]", false}, {"[data-testid=save-new]", true}} {
		corpus := &sightmap.Corpus{GlobalComponents: []sightmap.ComponentDef{{Name: "Editor", Selectors: []string{"main"}}, {Name: "Save", Selectors: []string{"main " + tc.selector}, ParentChain: []string{"Editor"}}}}
		matches := match.NewMatcher(corpus).Match(root, "https://example.test/editor")
		got, err := resolveMatchedQuery(root, matches, queryPropertyValues(matches), q)
		if tc.repaired {
			if err != nil || got != button {
				t.Fatalf("repaired query got %v, %v", got, err)
			}
		} else if got != nil || err == nil || !strings.Contains(err.Error(), "Recovery:") {
			t.Fatalf("stale selector got %v, %v; want failure with recovery", got, err)
		}
	}
}

func TestQueryRecoveryPreservesResolutionErrors(t *testing.T) {
	a, b := &sightmap.ComponentNode{Id: "1"}, &sightmap.ComponentNode{Id: "2"}
	root := &sightmap.ComponentNode{Children: []*sightmap.ComponentNode{a, b}}
	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{a: {Name: "Save"}, b: {Name: "Save"}}
	for _, query := range []string{"Save", "Save#2"} {
		q, err := compquery.ParseQuery(query)
		if err != nil {
			t.Fatal(err)
		}
		_, want := compquery.Resolve(root, matches, nil, q)
		_, got := resolveMatchedQuery(root, matches, nil, q)
		if got == nil || want == nil || got.Error() != want.Error() {
			t.Fatalf("%s: got %v, want %v", query, got, want)
		}
	}
	q, _ := compquery.ParseQuery("Save")
	_, err := resolveMatchedQuery(root, nil, nil, q)
	if err == nil || !strings.Contains(err.Error(), "Recovery:") {
		t.Fatalf("empty map: %v", err)
	}
}
