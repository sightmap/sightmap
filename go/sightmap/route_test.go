package sightmap_test

import (
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// ---- Route matching: trailing slashes + :param ------------------------------

func TestMatchRouteTrailingSlash(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		// Trailing slash on the path (the common real-app case) must not defeat
		// a slash-free pattern, and vice versa.
		{"/*/projects", "/acme/projects/", true},
		{"/*/projects", "/acme/projects", true},
		{"/*/projects/", "/acme/projects", true},
		{"/*/projects/", "/acme/projects/", true},
		{"/settings", "/settings/", true},
		{"/settings/", "/settings", true},
		// Root is never stripped to empty.
		{"/", "/", true},
		{"/", "", true},
		// :param matches a single segment, like *.
		{"/api/users/:id/orders", "/api/users/42/orders", true},
		{"/api/users/:id/orders", "/api/users/42/orders/", true},
		{"/api/users/:id", "/api/users/42/extra", false},
		// Single-segment wildcard still doesn't cross a slash.
		{"/users/*", "/users/42/edit", false},
		{"/users/*", "/users/42", true},
	}
	for _, tc := range cases {
		if got := sightmap.MatchRoute(tc.pattern, tc.path); got != tc.want {
			t.Errorf("MatchRoute(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

// ---- View resolution: most specific wins ------------------------------------

func TestViewForURLMostSpecificWins(t *testing.T) {
	// Declaration order deliberately puts the GENERIC route first so a
	// first-match implementation would pick the wrong view.
	c := &sightmap.Corpus{
		Views: []sightmap.View{
			{Name: "Generic", Route: "/users/*"},
			{Name: "Specific", Route: "/users/me"},
			{Name: "Param", Route: "/users/:id/edit"},
			{Name: "Tree", Route: "/admin/**"},
			{Name: "Root", Route: "/"},
		},
	}

	cases := []struct {
		url, want string
	}{
		{"/users/me", "Specific"},   // literal beats /users/*
		{"/users/42", "Generic"},    // only /users/* matches
		{"/users/42/edit", "Param"}, // :param route matches a 3rd segment
		{"/admin/foo/bar", "Tree"},  // ** depth
		{"/", "Root"},               // root scores 1, beats nothing else here
		{"/users/me/", "Specific"},  // trailing slash normalized
		{"/nope/nowhere", ""},       // no match
	}
	for _, tc := range cases {
		v := c.ViewForURL(tc.url)
		got := ""
		if v != nil {
			got = v.Name
		}
		if got != tc.want {
			t.Errorf("ViewForURL(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

// TestViewForURLTieBreakDeclarationOrder verifies that equally specific routes
// resolve to the first-declared view.
func TestViewForURLTieBreakDeclarationOrder(t *testing.T) {
	c := &sightmap.Corpus{
		Views: []sightmap.View{
			{Name: "First", Route: "/a/*"},
			{Name: "Second", Route: "/*/b"},
		},
	}
	if v := c.ViewForURL("/a/b"); v == nil || v.Name != "First" {
		got := "<nil>"
		if v != nil {
			got = v.Name
		}
		t.Errorf("ViewForURL(/a/b) = %q, want First (declaration-order tiebreak)", got)
	}
}
