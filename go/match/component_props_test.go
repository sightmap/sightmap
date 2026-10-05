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
				{Name: "label", Extract: sightmap.Extract{From: sightmap.FromDOMText}},
				{Name: "price", Extract: sightmap.Extract{From: sightmap.FromComponent, Path: "Price.text"}},
				{Name: "sold_out", Extract: sightmap.Extract{From: sightmap.FromComponentExists, Path: "SoldOutBadge"}},
			},
		},
		{
			Name:        "Price",
			Selectors:   []string{"[data-testid=pod] [data-testid=price]"},
			ParentChain: []string{"ProductCard"},
			Properties:  []sightmap.ComponentPropertyDef{{Name: "text", Extract: sightmap.Extract{From: sightmap.FromDOMText}}},
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
			Properties:  []sightmap.ComponentPropertyDef{{Name: "href", Extract: sightmap.Extract{From: sightmap.FromDOMAttr, Path: "href"}}},
		},
	}
}

// TestResolveRawText covers the SEP-0013 raw_text extractor: it always returns
// the node's raw rendered Text, never the accessibility Name — the deterministic
// escape when the AX name welds in extra text. Mirrors the JetBlue sub-fare tile,
// whose heading AX name is "Main Most popular" but whose raw text is "Main", so
// SubFare[tier="Main"] can only match via raw_text.
func TestResolveRawText(t *testing.T) {
	defs := []sightmap.ComponentDef{{
		Name:      "SubFare",
		Selectors: []string{"[data-testid=tile]"},
		Properties: []sightmap.ComponentPropertyDef{
			{Name: "welded", Extract: sightmap.Extract{From: sightmap.FromDOMText}},  // AX name (welded)
			{Name: "tier", Extract: sightmap.Extract{From: sightmap.FromDOMRawText}}, // raw text (clean)
		},
	}}
	tile := &sightmap.ComponentNode{
		Id:      "tile",
		Name:    "Main Most popular", // welded accessibility name (Name)
		RawText: "Main",              // clean own text (RawText)
		Element: &sightmap.Element{Tag: "div", Attrs: map[string]string{"data-testid": "tile"}},
	}
	// A node with no own text at all: raw_text omits (per SEP-0013). Name is
	// non-empty to prove raw_text never falls back to the accessibility name.
	noText := &sightmap.ComponentNode{
		Id:      "tile",
		Name:    "Only a name",
		RawText: "",
		Element: &sightmap.Element{Tag: "div", Attrs: map[string]string{"data-testid": "tile"}},
	}

	res := match.NewMatcher(&sightmap.Corpus{GlobalComponents: defs}).Match(tile, "")
	if v, ok := propVal(res[tile], "welded"); !ok || v != "Main Most popular" {
		t.Errorf("welded (text) = %q, %v; want \"Main Most popular\", true", v, ok)
	}
	if v, ok := propVal(res[tile], "tier"); !ok || v != "Main" {
		t.Errorf("tier (raw_text) = %q, %v; want \"Main\", true", v, ok)
	}

	res2 := match.NewMatcher(&sightmap.Corpus{GlobalComponents: defs}).Match(noText, "")
	if v, ok := propVal(res2[noText], "tier"); ok {
		t.Errorf("tier (raw_text) on empty Text = %q, %v; want omitted", v, ok)
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
			Properties: []sightmap.ComponentPropertyDef{{Name: "amount", Extract: sightmap.Extract{From: sightmap.FromComponent, Path: "Price.Amount.text"}}},
		},
		{Name: "Price", Selectors: []string{"[data-testid=row] [data-testid=price]"}, ParentChain: []string{"Row"}},
		{Name: "Amount", Selectors: []string{"[data-testid=row] [data-testid=price] [data-testid=amount]"}, ParentChain: []string{"Row", "Price"}, Properties: []sightmap.ComponentPropertyDef{{Name: "text", Extract: sightmap.Extract{From: sightmap.FromDOMText}}}},
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

func TestResolveExtractObject(t *testing.T) {
	ex := func(from, path string) sightmap.Extract { return sightmap.Extract{From: from, Path: path} }
	defs := []sightmap.ComponentDef{
		{
			Name:      "Card",
			Selectors: []string{".card"},
			Properties: []sightmap.ComponentPropertyDef{
				{Name: "price", Extract: sightmap.Extract{From: sightmap.FromDOMText, Pattern: `\$([\d.]+)`}},
				{Name: "tags", Extract: sightmap.Extract{From: sightmap.FromComponent, Path: "Tag[].value", Join: ","}},
				{Name: "first_tag", Extract: ex(sightmap.FromComponent, "Tag.value")},
				{Name: "short_tags", Extract: sightmap.Extract{From: sightmap.FromComponent, Path: "Tag[].value", Pattern: `^(\w{3})`, Join: "|"}},
				{Name: "has_tag", Extract: ex(sightmap.FromComponentExists, "Tag")},
				{Name: "has_badge", Extract: ex(sightmap.FromComponentExists, "Badge")},
				{Name: "on", Extract: ex(sightmap.FromDOMState, "checked")},
				{Name: "markup", Extract: ex(sightmap.FromDOMAttr, "checked")},
				{Name: "missing", Extract: sightmap.Extract{From: sightmap.FromDOMText, Pattern: `^nope$`}},
			},
		},
		{
			Name:       "Tag",
			Selectors:  []string{".tag"},
			Properties: []sightmap.ComponentPropertyDef{{Name: "value", Extract: sightmap.Extract{From: sightmap.FromDOMText}}},
		},
	}
	tag := func(id, text string) *sightmap.ComponentNode {
		return &sightmap.ComponentNode{Id: id, Name: text, Element: &sightmap.Element{Tag: "span", Classes: []string{"tag"}, Attrs: map[string]string{"class": "tag"}}}
	}
	card := &sightmap.ComponentNode{
		Id:         "card",
		Name:       "Add to cart · $10.95",
		Properties: map[string]string{"checked": "false"},
		Element:    &sightmap.Element{Tag: "div", Classes: []string{"card"}, Attrs: map[string]string{"class": "card", "checked": ""}},
		Children:   []*sightmap.ComponentNode{tag("t1", "sale"), tag("t2", ""), tag("t3", "featured")},
	}
	res := match.NewMatcher(&sightmap.Corpus{GlobalComponents: defs}).Match(card, "")
	for name, want := range map[string]string{
		"price":      "10.95",
		"tags":       "sale,featured", // the empty tag is dropped
		"first_tag":  "sale",
		"short_tags": "sal|fea",
		"has_tag":    "true",
		"on":         "false", // state, not the valueless markup attribute
	} {
		if v, ok := propVal(res[card], name); !ok || v != want {
			t.Errorf("%s = %q, %v; want %q", name, v, ok, want)
		}
	}
	for _, name := range []string{"has_badge", "markup", "missing"} {
		if v, ok := propVal(res[card], name); ok {
			t.Errorf("%s = %q; want omitted", name, v)
		}
	}
}

// Only a segment written Name[] fans out; every other segment takes its first
// match within each node the previous segment produced.
func TestResolveMultiValuedPathFansOutPerSegment(t *testing.T) {
	text := sightmap.Extract{From: sightmap.FromDOMText}
	defs := []sightmap.ComponentDef{
		{
			Name: "List", Selectors: []string{".list"},
			Properties: []sightmap.ComponentPropertyDef{
				{Name: "every_row_first_price", Extract: sightmap.Extract{From: sightmap.FromComponent, Path: "Row[].Price.v", Join: ","}},
				{Name: "first_row_every_price", Extract: sightmap.Extract{From: sightmap.FromComponent, Path: "Row.Price[].v", Join: ","}},
				{Name: "every_price", Extract: sightmap.Extract{From: sightmap.FromComponent, Path: "Row[].Price[].v", Join: ","}},
			},
		},
		{Name: "Row", Selectors: []string{".row"}},
		{Name: "Price", Selectors: []string{".price"}, Properties: []sightmap.ComponentPropertyDef{{Name: "v", Extract: text}}},
	}
	el := func(class string) *sightmap.Element {
		return &sightmap.Element{Tag: "div", Classes: []string{class}, Attrs: map[string]string{"class": class}}
	}
	price := func(id, v string) *sightmap.ComponentNode {
		return &sightmap.ComponentNode{Id: id, Name: v, Element: el("price")}
	}
	row := func(id string, prices ...*sightmap.ComponentNode) *sightmap.ComponentNode {
		return &sightmap.ComponentNode{Id: id, Element: el("row"), Children: prices}
	}
	list := &sightmap.ComponentNode{Id: "list", Element: el("list"), Children: []*sightmap.ComponentNode{
		row("r1", price("p1", "1"), price("p2", "2")),
		row("r2", price("p3", "3"), price("p4", "4")),
	}}
	res := match.NewMatcher(&sightmap.Corpus{GlobalComponents: defs}).Match(list, "")
	for name, want := range map[string]string{
		"every_row_first_price": "1,3",
		"first_row_every_price": "1,2",
		"every_price":           "1,2,3,4",
	} {
		if v, ok := propVal(res[list], name); !ok || v != want {
			t.Errorf("%s = %q, %v; want %q", name, v, ok, want)
		}
	}
}
