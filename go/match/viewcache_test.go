package match

import (
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
	urls := []string{
		"https://shop.test/item/1", "https://shop.test/item/2?ref=x", "https://shop.test/item/3",
		"https://shop.test/cart", "https://shop.test/about", "https://shop.test/help",
	}
	names := func(u string) []string {
		var out []string
		for _, d := range m.Components(u) {
			out = append(out, d.Name)
		}
		return out
	}
	want := map[string]string{"/item/1": "Price,Nav", "/item/2?ref=x": "Price,Nav", "/item/3": "Price,Nav", "/cart": "Total,Nav", "/about": "Nav", "/help": "Nav"}
	for _, u := range urls {
		got := ""
		for i, n := range names(u) {
			if i > 0 {
				got += ","
			}
			got += n
		}
		if w := want[u[len("https://shop.test"):]]; got != w {
			t.Errorf("%s: components %s, want %s", u, got, w)
		}
	}
	if len(m.cache) != 3 {
		t.Errorf("%d compiled entries for 6 URLs over 2 views and globals, want 3", len(m.cache))
	}
	if m.entryFor(urls[0]) != m.entryFor(urls[2]) || m.entryFor(urls[0]) == m.entryFor(urls[3]) || m.entryFor(urls[4]) != m.entryFor(urls[5]) {
		t.Error("entries are not shared exactly by view")
	}
}
