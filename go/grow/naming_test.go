package grow

import (
	"testing"

	"github.com/sightmap/sightmap/go/explore"
)

func TestRoutePattern(t *testing.T) {
	cases := map[string]string{
		"https://books.toscrape.com/":                                               "/",
		"https://books.toscrape.com/catalogue/page-2.html":                          "/catalogue/*",
		"https://books.toscrape.com/catalogue/a-light-in-the-attic_1000/index.html": "/catalogue/*/index.html",
		"https://books.toscrape.com/catalogue/category/books/travel_2/index.html":   "/catalogue/category/books/*/index.html",
		"https://app.clay.com/workspaces/4088/settings/billing":                     "/workspaces/*/settings/billing",
	}
	for in, want := range cases {
		if got := RoutePattern(in); got != want {
			t.Errorf("RoutePattern(%s) = %q, want %q", in, got, want)
		}
	}
}

func TestRouteName(t *testing.T) {
	cases := map[string]string{
		"/":                                      "Home",
		"/catalogue/*":                           "CataloguePage",
		"/catalogue/*/index.html":                "CatalogueDetail",
		"/catalogue/category/books/*/index.html": "CategoryBooksDetail",
		"/workspaces/*/settings/billing":         "SettingsBillingDetail",
		"/*":                                     "Page",
	}
	for in, want := range cases {
		if got := RouteName(in); got != want {
			t.Errorf("RouteName(%s) = %q, want %q", in, got, want)
		}
	}
}

func node(name, tag string, attrs map[string]string) *explore.Node {
	if attrs == nil {
		attrs = map[string]string{}
	}
	return &explore.Node{Name: name, Tag: tag, Attrs: attrs}
}

func TestGroupName(t *testing.T) {
	many := []*explore.Node{node("Travel", "a", nil), node("Mystery", "a", nil)}
	cases := []struct {
		hook, tag, kind string
		members         []*explore.Node
		count           int
		want            string
	}{
		{"ul.nav-list", "a", "link", many, 28, "NavListLink"},
		{"article.product_pod", "a", "link", many, 40, "ProductPodLink"},
		{"div.product_price", "button", "button", []*explore.Node{node("Add to basket", "button", nil)}, 20, "ProductPriceButton"},
		{"div.col-sm-8", "a", "link", []*explore.Node{node("Books to Scrape", "a", nil)}, 1, "BooksScrapeLink"},
		{"", "input", "input", []*explore.Node{node("", "input", map[string]string{"placeholder": "Search products"})}, 1, "SearchProductsInput"},
		{"form[aria-label=\"Login\"]", "input", "button", []*explore.Node{node("", "input", nil), node("", "input", nil)}, 2, "FormButton"},
		{"ul.pager", "a", "nav", many, 2, "PagerLink"},
		{"", "button", "card", many, 3, "ButtonCard"},
	}
	for _, c := range cases {
		if got := GroupName(c.hook, c.tag, "link", c.members, c.kind, c.count); got != c.want {
			t.Errorf("GroupName(%q,%q,%q) = %q, want %q", c.hook, c.tag, c.kind, got, c.want)
		}
	}
}

func TestStabilityPrefersDataAttributesOverHashes(t *testing.T) {
	if Stability(`[data-test="login-button"]`) <= Stability(`button.btn-primary`) {
		t.Fatal("data-test should outrank a class")
	}
	if Stability(`#login-button`) <= Stability(`div.css-1x2y3z4`) {
		t.Fatal("an id should outrank a hashed class")
	}
	if Stability(`div.sc-a1b2c3d4`) >= 0 {
		t.Fatal("hashed classes score negative")
	}
}
