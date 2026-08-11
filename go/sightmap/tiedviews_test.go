package sightmap_test

import (
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

func TestTiedViews(t *testing.T) {
	c := &sightmap.Corpus{
		Views: []sightmap.ViewDef{
			{Name: "AStar", Route: "/a/*"},    // score 4
			{Name: "StarB", Route: "/*/b"},    // score 4 — ties with AStar for /a/b
			{Name: "Specific", Route: "/a/b"}, // score 6 — wins /a/b outright
			{Name: "Users", Route: "/users/*"},
		},
	}

	// /x/b matches /a/* (no) ... actually /*/b (score 4) only. Single match → no tie.
	if got := c.TiedViews("/x/b"); got != nil {
		t.Errorf("TiedViews(/x/b) = %v, want nil (single match)", got)
	}
	// /a/b: /a/* and /*/b tie at 4, but /a/b (6) is strictly more specific → the
	// top score is unique → no tie.
	if got := c.TiedViews("/a/b"); got != nil {
		t.Errorf("TiedViews(/a/b) = %v, want nil (a literal wins outright)", got)
	}
	// /a/z: /a/* (4) matches; /*/b (4) does not (z != b). Single top → no tie.
	if got := c.TiedViews("/a/z"); got != nil {
		t.Errorf("TiedViews(/a/z) = %v, want nil", got)
	}
}

func TestTiedViews_GenuineTie(t *testing.T) {
	c := &sightmap.Corpus{
		Views: []sightmap.ViewDef{
			{Name: "AStar", Route: "/a/*"}, // score 4
			{Name: "StarB", Route: "/*/b"}, // score 4 — both match /a/b at equal specificity
		},
	}
	got := c.TiedViews("/a/b")
	if len(got) != 2 {
		t.Fatalf("TiedViews(/a/b) = %v, want 2 tied views", got)
	}
	if got[0] != "AStar" {
		t.Errorf("first tied view (winner) = %q, want AStar (declaration order)", got[0])
	}
}
