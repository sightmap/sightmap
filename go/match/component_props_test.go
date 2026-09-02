package match_test

import (
	"testing"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// productCardDefs is the flattened corpus for a ProductCard with a Price child,
// a SoldOutBadge child, and a Link child — exercising every SEP-0010 extract
// form: text (local), attr= (local), PATH.prop (descendant value), and
// exists:PATH (descendant presence).
func productCardDefs() []sightmap.ComponentDef {
	return []sightmap.ComponentDef{
		{
			Name:      "ProductCard",
			Selectors: []string{"[data-testid=pod]"},
			Properties: []sightmap.ComponentPropertyDef{
				{Name: "label", Extract: "text"},
				{Name: "price", Extract: "Price.text"},
				{Name: "sold_out", Extract: "exists:SoldOutBadge"},
			},
		},
		{
			Name:        "Price",
			Selectors:   []string{"[data-testid=pod] [data-testid=price]"},
			ParentChain: []string{"ProductCard"},
			Properties:  []sightmap.ComponentPropertyDef{{Name: "text", Extract: "text"}},
		},
		{
			Name:        "SoldOutBadge",
			Selectors:   []string{"[data-testid=pod] .sold-out"},
			ParentChain: []string{"ProductCard"},
		},
		{
			Name:        "Link",
			Selectors:   []string{"[data-testid=pod] a"},
			ParentChain: []string{"ProductCard"},
			Properties:  []sightmap.ComponentPropertyDef{{Name: "href", Extract: "attr=href"}},
		},
	}
}

func propVal(cm *sightmap.ComponentMatch, name string) (string, bool) {
	if cm == nil {
		return "", false
	}
	pv, ok := cm.Property(name)
	return pv.Value, ok
}

func TestResolveComponentProperties(t *testing.T) {
	price := &sightmap.ComponentNode{Id: "price", Name: "$42.00", Element: &sightmap.Element{Tag: "span", Attrs: map[string]string{"data-testid": "price"}}}
	badge := &sightmap.ComponentNode{Id: "badge", Name: "Sold Out", Element: &sightmap.Element{Tag: "span", Classes: []string{"sold-out"}}}
	link := &sightmap.ComponentNode{Id: "link", Name: "Buy", Element: &sightmap.Element{Tag: "a", Attrs: map[string]string{"href": "/p/1"}}}
	card := &sightmap.ComponentNode{
		Id:       "card",
		Name:     "Product X",
		Element:  &sightmap.Element{Tag: "div", Attrs: map[string]string{"data-testid": "pod"}},
		Children: []*sightmap.ComponentNode{price, badge, link},
	}

	noHrefLink := &sightmap.ComponentNode{Id: "link", Name: "Buy", Element: &sightmap.Element{Tag: "a"}}
	emptyCard := &sightmap.ComponentNode{
		Id:       "card",
		Name:     "", // empty accessible name → label omitted
		Element:  &sightmap.Element{Tag: "div", Attrs: map[string]string{"data-testid": "pod"}},
		Children: []*sightmap.ComponentNode{noHrefLink},
	}

	price1 := &sightmap.ComponentNode{Id: "p1", Name: "$10.00", Element: &sightmap.Element{Tag: "span", Attrs: map[string]string{"data-testid": "price"}}}
	price2 := &sightmap.ComponentNode{Id: "p2", Name: "$20.00", Element: &sightmap.Element{Tag: "span", Attrs: map[string]string{"data-testid": "price"}}}
	multiCard := &sightmap.ComponentNode{
		Id:       "card",
		Name:     "Product X",
		Element:  &sightmap.Element{Tag: "div", Attrs: map[string]string{"data-testid": "pod"}},
		Children: []*sightmap.ComponentNode{price1, price2},
	}

	amount := &sightmap.ComponentNode{Id: "amt", Name: "$5.00", Element: &sightmap.Element{Tag: "b", Attrs: map[string]string{"data-testid": "amount"}}}
	pricebox := &sightmap.ComponentNode{Id: "pb", Element: &sightmap.Element{Tag: "div", Attrs: map[string]string{"data-testid": "price"}}, Children: []*sightmap.ComponentNode{amount}}
	row := &sightmap.ComponentNode{Id: "row", Element: &sightmap.Element{Tag: "li", Attrs: map[string]string{"data-testid": "row"}}, Children: []*sightmap.ComponentNode{pricebox}}
	nestedDefs := []sightmap.ComponentDef{
		{
			Name:       "Row",
			Selectors:  []string{"[data-testid=row]"},
			Properties: []sightmap.ComponentPropertyDef{{Name: "amount", Extract: "Price.Amount.text"}},
		},
		{Name: "Price", Selectors: []string{"[data-testid=row] [data-testid=price]"}, ParentChain: []string{"Row"}},
		{Name: "Amount", Selectors: []string{"[data-testid=row] [data-testid=price] [data-testid=amount]"}, ParentChain: []string{"Row", "Price"}, Properties: []sightmap.ComponentPropertyDef{{Name: "text", Extract: "text"}}},
	}

	// Role-less nodes carry no accessible Name but do carry rendered Text.
	// `extract: text` must fall back to Text, and prefer Name when both exist.
	textOnlyCard := &sightmap.ComponentNode{
		Id:       "card",
		Name:     "", // role-less: no accessible name
		Text:     "B6 123",
		Element:  &sightmap.Element{Tag: "div", Attrs: map[string]string{"data-testid": "pod"}},
		Children: []*sightmap.ComponentNode{},
	}
	nameWinsCard := &sightmap.ComponentNode{
		Id:       "card",
		Name:     "Product X", // accessible name present
		Text:     "raw fallback text",
		Element:  &sightmap.Element{Tag: "div", Attrs: map[string]string{"data-testid": "pod"}},
		Children: []*sightmap.ComponentNode{},
	}

	type check struct {
		node *sightmap.ComponentNode
		prop string
		want string
		ok   bool
	}
	tests := []struct {
		name   string
		defs   []sightmap.ComponentDef
		root   *sightmap.ComponentNode
		checks []check
	}{
		{
			name: "all extract forms",
			defs: productCardDefs(),
			root: card,
			checks: []check{
				{card, "label", "Product X", true}, // local text
				{card, "price", "$42.00", true},    // PATH.prop into Price child's text
				{card, "sold_out", "true", true},   // exists:PATH — SoldOutBadge present
				{link, "href", "/p/1", true},       // attr= on Link child
				{price, "text", "$42.00", true},    // Price child carries its own resolved text
			},
		},
		{
			name: "silent omission",
			defs: productCardDefs(),
			root: emptyCard,
			checks: []check{
				{emptyCard, "label", "", false},    // empty accessible name
				{emptyCard, "price", "", false},    // no Price descendant
				{emptyCard, "sold_out", "", false}, // SoldOutBadge absent
				{noHrefLink, "href", "", false},    // attribute not carried
			},
		},
		{
			name: "multi-match resolves first in document order",
			defs: productCardDefs(),
			root: multiCard,
			checks: []check{
				{multiCard, "price", "$10.00", true},
			},
		},
		{
			name: "nested PATH.prop",
			defs: nestedDefs,
			root: row,
			checks: []check{
				{row, "amount", "$5.00", true},
			},
		},
		{
			name: "text falls back to rendered Text when accessible name is empty",
			defs: productCardDefs(),
			root: textOnlyCard,
			checks: []check{
				{textOnlyCard, "label", "B6 123", true},
			},
		},
		{
			name: "text prefers accessible name over rendered Text",
			defs: productCardDefs(),
			root: nameWinsCard,
			checks: []check{
				{nameWinsCard, "label", "Product X", true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := match.NewMatcher(&sightmap.Corpus{GlobalComponents: tt.defs}).Match(tt.root, "")
			for _, c := range tt.checks {
				v, ok := propVal(res[c.node], c.prop)
				if ok != c.ok || v != c.want {
					t.Errorf("%s = %q, %v; want %q, %v", c.prop, v, ok, c.want, c.ok)
				}
			}
		})
	}
}
