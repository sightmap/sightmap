package sightmap_test

import (
	"reflect"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

func reqNames(defs []sightmap.RequestDef) []string {
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	return names
}

func TestRequestsForURL(t *testing.T) {
	corpus := &sightmap.Corpus{
		Requests: []sightmap.RequestDef{
			{Name: "SearchFlights", Route: "/api/flights/search", Method: "POST"},
			{Name: "AnyFlights", Route: "/api/flights/**"}, // no method -> any method
			{Name: "GetUser", Route: "/api/users/:id", Method: "GET"},
		},
		Views: []sightmap.ViewDef{
			{Name: "Admin", Route: "/admin", Requests: []sightmap.RequestDef{
				{Name: "AdminPing", Route: "/api/flights/search", Method: "GET"},
			}},
		},
	}

	cases := []struct {
		name        string
		url, method string
		want        []string
	}{
		// All matches apply: the exact-route POST and the globstar both fire;
		// globals come before view-scoped defs.
		{"exact + globstar", "https://x.test/api/flights/search", "POST", []string{"SearchFlights", "AnyFlights"}},
		// GET excludes the POST-only def but keeps the method-less globstar and
		// the view-scoped GET def.
		{"method filter", "https://x.test/api/flights/search", "GET", []string{"AnyFlights", "AdminPing"}},
		// An empty method argument matches every route hit regardless of method.
		{"any method", "https://x.test/api/flights/search", "", []string{"SearchFlights", "AnyFlights", "AdminPing"}},
		{"param segment", "https://x.test/api/users/42", "GET", []string{"GetUser"}},
		{"param is one segment only", "https://x.test/api/users/42/orders", "GET", nil},
		{"query/fragment/trailing-slash ignored", "https://x.test/api/flights/search/?q=1#f", "POST", []string{"SearchFlights", "AnyFlights"}},
		{"method match is case-insensitive", "https://x.test/api/flights/search", "post", []string{"SearchFlights", "AnyFlights"}},
		{"no route match", "https://x.test/nope", "GET", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := reqNames(corpus.RequestsForURL(tc.url, tc.method))
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("RequestsForURL(%q, %q) = %v, want %v", tc.url, tc.method, got, tc.want)
			}
		})
	}
}
