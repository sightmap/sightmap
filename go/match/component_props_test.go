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

func TestResolveComponentProperties_AllForms(t *testing.T) {
	price := &sightmap.ComponentNode{Id: "price", Name: "$42.00", Element: &sightmap.Element{Tag: "span", Attrs: map[string]string{"data-testid": "price"}}}
	badge := &sightmap.ComponentNode{Id: "badge", Name: "Sold Out", Element: &sightmap.Element{Tag: "span", Classes: []string{"sold-out"}}}
	link := &sightmap.ComponentNode{Id: "link", Name: "Buy", Element: &sightmap.Element{Tag: "a", Attrs: map[string]string{"href": "/p/1"}}}
	card := &sightmap.ComponentNode{
		Id:       "card",
		Name:     "Product X",
		Element:  &sightmap.Element{Tag: "div", Attrs: map[string]string{"data-testid": "pod"}},
		Children: []*sightmap.ComponentNode{price, badge, link},
	}

	res := match.NewMatcher(&sightmap.Corpus{GlobalComponents: productCardDefs()}).Match(card, "")

	// Local text on the card itself.
	if v, ok := propVal(res[card], "label"); !ok || v != "Product X" {
		t.Errorf("label = %q, %v; want \"Product X\", true", v, ok)
	}
	// PATH.prop into the Price child's own text.
	if v, ok := propVal(res[card], "price"); !ok || v != "$42.00" {
		t.Errorf("price = %q, %v; want \"$42.00\", true", v, ok)
	}
	// exists:PATH — the SoldOutBadge is present.
	if v, ok := propVal(res[card], "sold_out"); !ok || v != "true" {
		t.Errorf("sold_out = %q, %v; want \"true\", true", v, ok)
	}
	// attr= on the Link child.
	if v, ok := propVal(res[link], "href"); !ok || v != "/p/1" {
		t.Errorf("href = %q, %v; want \"/p/1\", true", v, ok)
	}
	// The Price child carries its own resolved text.
	if v, ok := propVal(res[price], "text"); !ok || v != "$42.00" {
		t.Errorf("Price.text = %q, %v; want \"$42.00\", true", v, ok)
	}
}

func TestResolveComponentProperties_SilentOmission(t *testing.T) {
	// A card with no SoldOutBadge and no Price, and a link with no href.
	link := &sightmap.ComponentNode{Id: "link", Name: "Buy", Element: &sightmap.Element{Tag: "a"}}
	card := &sightmap.ComponentNode{
		Id:       "card",
		Name:     "", // empty accessible name → label omitted
		Element:  &sightmap.Element{Tag: "div", Attrs: map[string]string{"data-testid": "pod"}},
		Children: []*sightmap.ComponentNode{link},
	}

	res := match.NewMatcher(&sightmap.Corpus{GlobalComponents: productCardDefs()}).Match(card, "")

	if _, ok := propVal(res[card], "label"); ok {
		t.Error("label should be omitted when accessible name is empty")
	}
	if _, ok := propVal(res[card], "price"); ok {
		t.Error("price should be omitted when there is no Price descendant")
	}
	if _, ok := propVal(res[card], "sold_out"); ok {
		t.Error("sold_out should be omitted when SoldOutBadge is absent")
	}
	if _, ok := propVal(res[link], "href"); ok {
		t.Error("href should be omitted when the attribute is not carried")
	}
}

func TestResolveComponentProperties_MultiMatchFirstInDocumentOrder(t *testing.T) {
	price1 := &sightmap.ComponentNode{Id: "p1", Name: "$10.00", Element: &sightmap.Element{Tag: "span", Attrs: map[string]string{"data-testid": "price"}}}
	price2 := &sightmap.ComponentNode{Id: "p2", Name: "$20.00", Element: &sightmap.Element{Tag: "span", Attrs: map[string]string{"data-testid": "price"}}}
	card := &sightmap.ComponentNode{
		Id:       "card",
		Name:     "Product X",
		Element:  &sightmap.Element{Tag: "div", Attrs: map[string]string{"data-testid": "pod"}},
		Children: []*sightmap.ComponentNode{price1, price2},
	}

	res := match.NewMatcher(&sightmap.Corpus{GlobalComponents: productCardDefs()}).Match(card, "")

	if v, ok := propVal(res[card], "price"); !ok || v != "$10.00" {
		t.Errorf("price = %q, %v; want first-in-document-order \"$10.00\", true", v, ok)
	}
}

func TestResolveComponentProperties_NestedPath(t *testing.T) {
	amount := &sightmap.ComponentNode{Id: "amt", Name: "$5.00", Element: &sightmap.Element{Tag: "b", Attrs: map[string]string{"data-testid": "amount"}}}
	pricebox := &sightmap.ComponentNode{Id: "pb", Element: &sightmap.Element{Tag: "div", Attrs: map[string]string{"data-testid": "price"}}, Children: []*sightmap.ComponentNode{amount}}
	row := &sightmap.ComponentNode{Id: "row", Element: &sightmap.Element{Tag: "li", Attrs: map[string]string{"data-testid": "row"}}, Children: []*sightmap.ComponentNode{pricebox}}

	defs := []sightmap.ComponentDef{
		{
			Name:       "Row",
			Selectors:  []string{"[data-testid=row]"},
			Properties: []sightmap.ComponentPropertyDef{{Name: "amount", Extract: "Price.Amount.text"}},
		},
		{Name: "Price", Selectors: []string{"[data-testid=row] [data-testid=price]"}, ParentChain: []string{"Row"}},
		{Name: "Amount", Selectors: []string{"[data-testid=row] [data-testid=price] [data-testid=amount]"}, ParentChain: []string{"Row", "Price"}, Properties: []sightmap.ComponentPropertyDef{{Name: "text", Extract: "text"}}},
	}

	res := match.NewMatcher(&sightmap.Corpus{GlobalComponents: defs}).Match(row, "")
	if v, ok := propVal(res[row], "amount"); !ok || v != "$5.00" {
		t.Errorf("amount (Price.Amount.text) = %q, %v; want \"$5.00\", true", v, ok)
	}
}
