package sightmap_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// ---- SEP-0025: query conditions in routes -----------------------------------

func TestMatchRouteQueryConditions(t *testing.T) {
	cases := []struct {
		pattern, url string
		want         bool
	}{
		// A literal value: the key's first value must equal it.
		{"/profile?section=gift-cards", "/profile?section=gift-cards", true},
		{"/profile?section=gift-cards", "/profile?section=order-again", false},
		{"/profile?section=gift-cards", "/profile", false},
		{"/profile?section=gift-cards", "/profile?other=gift-cards", false},
		{"/profile?section=gift-cards", "/profile?utm=x&section=gift-cards", true}, // keys the route doesn't name are ignored
		{"/profile?section=gift-cards", "/profile?section=gift-cards&section=x", true},
		{"/profile?section=gift-cards", "/profile?section=x&section=gift-cards", false}, // first value only
		{"/profile?section=gift-cards", "/profile?section=Gift-Cards", false},           // case-sensitive
		{"/profile?section=gift-cards", "/settings?section=gift-cards", false},          // the path still has to match
		// Trailing slashes are normalized off the path part on both sides.
		{"/profile/?section=gift-cards", "/profile?section=gift-cards", true},
		{"/profile?section=gift-cards", "/profile/?section=gift-cards", true},
		// Every condition must hold, in any order.
		{"/page?id=7&lang=en", "/page?lang=en&id=7", true},
		{"/page?id=7&lang=en", "/page?id=7", false},
		// KEY=:name and KEY=* require presence, with any value (empty included).
		{"/page?id=:id", "/page?id=101", true},
		{"/page?id=:id", "/page?id=", true},
		{"/page?id=:id", "/page?id", true},
		{"/page?id=:id", "/page", false},
		{"/page?id=*", "/page?id=101", true},
		{"/page?id=*", "/page?ids=101", false},
		// A "*" inside a value is a wildcard within it.
		{"/graphs?type=sparkline*", "/graphs?type=sparkline-weekly", true},
		{"/graphs?type=sparkline*", "/graphs?type=bar", false},
		{"/graphs?type=*line", "/graphs?type=sparkline", true},
		{"/graphs?type=%2A", "/graphs?type=*", true}, // an encoded star is literal
		{"/graphs?type=%2A", "/graphs?type=x", false},
		// Values decode as url.query reads them: + and %20 are both a space.
		{"/search?q=a+b", "/search?q=a%20b", true},
		{"/search?q=a%20b", "/search?q=a+b", true},
		{"/search?q=a%2Bb", "/search?q=a+b", false},
		// The path part keeps its globs.
		{"/:org/library?type=metrics", "/acme/library?type=metrics", true},
		{"/admin/**?tab=users", "/admin/a/b?tab=users", true},
		// A path-only route matches whatever the query string holds.
		{"/profile", "/profile?section=gift-cards", true},
		// A malformed query part matches nothing.
		{"/profile?section", "/profile?section=x", false},
		{"/profile?", "/profile", false},
		{"/page?id=1&id=2", "/page?id=1", false},
	}
	for _, tc := range cases {
		if got := sightmap.MatchRoute(tc.pattern, tc.url); got != tc.want {
			t.Errorf("MatchRoute(%q, %q) = %v, want %v", tc.pattern, tc.url, got, tc.want)
		}
	}
}

// A view per ?section= of one page: each section resolves to its own view, the
// page itself to the path-only view, and a broader path view loses to both.
func TestViewForURLQueryConditions(t *testing.T) {
	c := &sightmap.Corpus{
		Views: []sightmap.ViewDef{
			{Name: "Page", Route: "/en-us/:page"},
			{Name: "ProfileMenu", Route: "/en-us/profile"},
			{Name: "GiftCards", Route: "/en-us/profile?section=gift-cards"},
			{Name: "OrderAgain", Route: "/en-us/profile?section=order-again"},
			{Name: "CmsPage", Route: "/page?id=:id"},
			{Name: "Article", Route: "/page?id=101"},
			{Name: "Bare", Route: "/page"},
		},
	}
	cases := []struct{ url, want string }{
		{"https://example.com/en-us/profile?section=gift-cards", "GiftCards"},
		{"https://example.com/en-us/profile?section=order-again&utm_source=x", "OrderAgain"},
		{"https://example.com/en-us/profile?section=order-history", "ProfileMenu"},
		{"https://example.com/en-us/profile", "ProfileMenu"},
		{"https://example.com/en-us/menu?section=gift-cards", "Page"},
		{"/page?id=101", "Article"},
		{"/page?id=206", "CmsPage"},
		{"/page", "Bare"},
	}
	for _, tc := range cases {
		got := ""
		if v := c.ViewForURL(tc.url); v != nil {
			got = v.Name
		}
		if got != tc.want {
			t.Errorf("ViewForURL(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

// Conditions add to the path's score as the segments they mirror, so a
// presence condition on a wildcard path can tie a literal path, and the tie
// falls to declaration order and is reported.
func TestViewForURLQueryConditionTie(t *testing.T) {
	c := &sightmap.Corpus{
		Views: []sightmap.ViewDef{
			{Name: "Admin", Route: "/users/admin"},     // 3+3
			{Name: "Tabbed", Route: "/users/*?tab=:t"}, // 3+1+2
			{Name: "Billing", Route: "/users/*?tab=billing"},
		},
	}
	if v := c.ViewForURL("/users/admin?tab=x"); v == nil || v.Name != "Admin" {
		t.Errorf("ViewForURL tie = %v, want Admin (first declared)", v)
	}
	if got, want := c.TiedViews("/users/admin?tab=x"), []string{"Admin", "Tabbed"}; !slices.Equal(got, want) {
		t.Errorf("TiedViews = %v, want %v", got, want)
	}
	if v := c.ViewForURL("/users/admin?tab=billing"); v == nil || v.Name != "Billing" {
		t.Errorf("ViewForURL(literal value) = %v, want Billing (3+1+3 beats 3+3)", v)
	}
	if got := c.TiedViews("/users/admin"); got != nil {
		t.Errorf("TiedViews without a query = %v, want none", got)
	}
}

func TestTagsForURLQueryConditions(t *testing.T) {
	c := &sightmap.Corpus{
		Views: []sightmap.ViewDef{
			{Name: "Profile", Route: "/profile", Tags: []string{"account"}},
			{Name: "GiftCards", Route: "/profile?section=gift-cards", Tags: []string{"payments"}},
		},
	}
	if got, want := c.TagsForURL("/profile?section=gift-cards"), []string{"account", "payments"}; !slices.Equal(got, want) {
		t.Errorf("TagsForURL(gift-cards) = %v, want %v", got, want)
	}
	if got, want := c.TagsForURL("/profile?section=other"), []string{"account"}; !slices.Equal(got, want) {
		t.Errorf("TagsForURL(other) = %v, want %v", got, want)
	}
}

func TestRequestsForURLQueryConditions(t *testing.T) {
	c := &sightmap.Corpus{
		Requests: []sightmap.RequestDef{
			{Name: "GraphQL", Route: "/graphql"},
			{Name: "Items", Route: "/graphql?operationName=Items"},
			{Name: "Persisted", Route: "/graphql?hash=*"},
		},
	}
	names := func(rs []sightmap.RequestDef) []string {
		var out []string
		for _, r := range rs {
			out = append(out, r.Name)
		}
		return out
	}
	cases := []struct {
		url  string
		want []string
	}{
		{"https://api.example.com/graphql?operationName=Items", []string{"GraphQL", "Items"}},
		{"https://api.example.com/graphql?operationName=Cart&hash=ab12", []string{"GraphQL", "Persisted"}},
		{"https://api.example.com/graphql", []string{"GraphQL"}},
	}
	for _, tc := range cases {
		if got := names(c.RequestsForURL(tc.url, "")); !slices.Equal(got, tc.want) {
			t.Errorf("RequestsForURL(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestValidateRouteQuery(t *testing.T) {
	c := loadYAML(t, map[string]string{"app.yaml": `
version: 1
views:
  - name: GiftCards
    route: /profile?section=gift-cards&tab=:tab
  - name: GiftCardsAgain
    route: /profile/?tab=:tab&section=gift-cards
  - name: OrderAgain
    route: /profile?section=order-again
  - name: NoValue
    route: /profile?section
  - name: Twice
    route: /page?id=1&id=2
  - name: BadEscape
    route: /page?id=%zz
requests:
  - name: Empty
    route: /api/items?
`})
	got := codes(sightmap.Validate(c))
	want := []string{"route-conflict", "route-query-invalid", "route-query-invalid", "route-query-invalid", "route-query-invalid"}
	if !slices.Equal(got, want) {
		t.Errorf("Validate codes = %v, want %v", got, want)
	}
}

// A query :name is a presence condition: it binds nothing, so it neither warns
// as an unbound segment nor satisfies a url.path extract.
func TestRouteQueryParamDoesNotBind(t *testing.T) {
	c := loadYAML(t, map[string]string{"app.yaml": `
version: 1
views:
  - name: CmsPage
    route: /page?pageId=:pageId
    properties:
      - name: page_id
        extract: { from: url.query, path: pageId }
  - name: Dangling
    route: /doc?id=:id
    properties:
      - name: doc
        extract: { from: url.path, path: id }
`})
	got := codes(sightmap.Validate(c))
	if want := []string{"url-property-param-unresolved"}; !slices.Equal(got, want) {
		for _, e := range sightmap.Validate(c) {
			t.Log(e.Code, e.Message)
		}
		t.Errorf("Validate codes = %v, want %v", got, want)
	}
}

// permutations returns every ordering of xs.
func permutations[T any](xs []T) [][]T {
	if len(xs) <= 1 {
		return [][]T{slices.Clone(xs)}
	}
	var out [][]T
	for i := range xs {
		rest := slices.Concat(xs[:i:i], xs[i+1:])
		for _, p := range permutations(rest) {
			out = append(out, append([]T{xs[i]}, p...))
		}
	}
	return out
}

// Order is not significant: the same URL with its parameters shuffled, against
// the same views declared in any order, with any route's conditions in either
// order, resolves to one view.
func TestViewForURLQueryOrderInsensitive(t *testing.T) {
	views := []sightmap.ViewDef{
		{Name: "Page", Route: "/test"},
		{Name: "CooView", Route: "/test?b=coo"},
		{Name: "FooView", Route: "/test?a=foo"},
		{Name: "FooCooView", Route: "/test?a=foo&b=coo"},
	}
	cases := []struct {
		params []string
		want   string
	}{
		{[]string{"a=foo", "b=coo"}, "FooCooView"},
		{[]string{"a=foo", "b=coo", "x=1"}, "FooCooView"},
		{[]string{"b=coo"}, "CooView"},
		{[]string{"b=coo", "a=other", "x=1"}, "CooView"},
		{[]string{"a=foo", "b=other"}, "FooView"},
		{[]string{"a=other", "b=other", "x=1"}, "Page"},
		{[]string{"x=1"}, "Page"},
	}
	// The spelling of FooCooView's conditions, both ways.
	for _, fooCoo := range []string{"/test?a=foo&b=coo", "/test?b=coo&a=foo"} {
		views[3].Route = fooCoo
		for _, order := range permutations(views) {
			c := &sightmap.Corpus{Views: order}
			for _, tc := range cases {
				for _, params := range permutations(tc.params) {
					u := "/test?" + strings.Join(params, "&")
					got := ""
					if v := c.ViewForURL(u); v != nil {
						got = v.Name
					}
					if got != tc.want {
						t.Errorf("ViewForURL(%q) with FooCooView %q, declared %v = %q, want %q",
							u, fooCoo, viewNames(order), got, tc.want)
					}
					if tied := c.TiedViews(u); tied != nil {
						t.Errorf("TiedViews(%q) = %v, want none", u, tied)
					}
				}
			}
		}
	}
}

func viewNames(vs []sightmap.ViewDef) []string {
	names := make([]string, len(vs))
	for i, v := range vs {
		names[i] = v.Name
	}
	return names
}
