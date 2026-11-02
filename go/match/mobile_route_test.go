package match_test

import (
	"testing"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

func TestMatcherViewComponentsOnSlashlessPageName(t *testing.T) {
	const pageURL = "App.ProductViewController"
	corpus := &sightmap.Corpus{
		Views: []sightmap.ViewDef{{
			Name:       "Product",
			Route:      "/app.productviewcontroller",
			Components: []sightmap.ComponentDef{{Name: "AddToCart", Selectors: []string{"button.add-to-cart"}}},
		}},
	}
	m := match.NewMatcher(corpus)

	comps := m.Components(pageURL)
	if len(comps) != 1 || comps[0].Name != "AddToCart" {
		t.Fatalf("Components(%q) = %v, want [AddToCart]", pageURL, comps)
	}

	root := node("root", "div", nil, node("btn", "button", []string{"add-to-cart"}))
	matches := m.Match(root, pageURL)
	var got []string
	for n, cm := range matches {
		got = append(got, cm.Name+"@"+n.Id)
	}
	if len(got) != 1 || got[0] != "AddToCart@btn" {
		t.Errorf("Match(%q) = %v, want [AddToCart@btn]", pageURL, got)
	}
}
