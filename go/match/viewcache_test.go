package match

import (
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// What a URL compiles to depends only on the view it selects, so URLs of one
// view share one compiled entry, other views get their own, and URLs no view
// routes share the globals-only entry. The answers are the same as before.
func TestMatcherCachesPerView(t *testing.T) {
	c := &sightmap.Corpus{
		GlobalComponents: []sightmap.ComponentDef{{Name: "Nav", Selectors: []string{"nav"}}},
		Views: []sightmap.ViewDef{
			{Name: "Item", Route: "/item/*", Components: []sightmap.ComponentDef{{Name: "Price", Selectors: []string{"span.price"}}}},
			{Name: "Cart", Route: "/cart", Components: []sightmap.ComponentDef{{Name: "Total", Selectors: []string{"span.total"}}}},
		},
	}
	m := NewMatcher(c)
	// "https://shop.test/%zz" does not parse, so no view selects it.
	cases := []struct{ url, want string }{
		{"https://shop.test/item/1", "Price,Nav"},
		{"https://shop.test/item/2?ref=x", "Price,Nav"},
		{"https://shop.test/item/3", "Price,Nav"},
		{"https://shop.test/cart", "Total,Nav"},
		{"https://shop.test/about", "Nav"},
		{"https://shop.test/help", "Nav"},
		{"https://shop.test/%zz", "Nav"},
	}
	for _, tc := range cases {
		var names []string
		for _, d := range m.Components(tc.url) {
			names = append(names, d.Name)
		}
		if got := strings.Join(names, ","); got != tc.want {
			t.Errorf("%s: components %s, want %s", tc.url, got, tc.want)
		}
	}
	if len(m.cache) != 3 {
		t.Errorf("%d compiled entries for 7 URLs over 2 views and globals, want 3", len(m.cache))
	}
	entry := func(i int) *queryCacheEntry { return m.entryFor(cases[i].url) }
	if entry(0) != entry(2) {
		t.Error("URLs of one view do not share an entry")
	}
	if entry(0) == entry(3) {
		t.Error("different views share an entry")
	}
	if entry(4) != entry(5) || entry(4) != entry(6) {
		t.Error("URLs no view routes (including an unparseable one) do not share the globals-only entry")
	}
}
