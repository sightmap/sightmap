package sightmap

import "testing"

func TestIDSelector(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"", ""},
		{"submit", "#submit"},
		{"search-box_2", "#search-box_2"},
		{"product.1", `#product\.1`},
		{"shop.product.1", `#shop\.product\.1`},
		{"checkout.submit", `#checkout\.submit`},
		{"a:b/c[d]", `#a\:b\/c\[d\]`},
		{`q"x\y`, `#q\"x\\y`},
		{"-x", "#-x"},
		{"-.x", `#-\.x`},
		{"-", `#\-`},
		{"--x", `#\--x`},
		{"-1", `#\-1`},
		{"_1", "#_1"},
		{"café.menü", `#café\.menü`},
		{"1.circle", `#\31 \.circle`},
		{"123abc", `#\31 23abc`},
		{"9", `#\39`},
		{"0a", `#\30 a`},
		{"first name", `#first\ name`},
		{"- x", `#-\ x`},
		{`2"x`, `#\32 \"x`},
		{"a\u00a0b", `#a\a0 b`},
		{"a\u3000", `#a\3000`},
		{"a\nb", ""},
		{"a\tb", ""},
		{"a\x7fb", ""},
	}
	for _, tt := range tests {
		if got := IDSelector(tt.id); got != tt.want {
			t.Errorf("IDSelector(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
}

// Every selector IDSelector emits must parse and match exactly the element
// carrying that id, and no element whose id differs only where the unescaped
// form would mislead the parser.
func TestIDSelectorRoundTrip(t *testing.T) {
	ids := []string{
		"submit", "product.1", "shop.product.1", "checkout.submit",
		"a:b/c[d]", `q"x\y`, "-", "--x", "-1", "-.x", "café.menü",
		"1.circle", "123abc", "9", "0a", "first name", "- x", `2"x`,
		"a\u00a0b", "a\u3000", "a#b", "a>b c", "a,b", "a(b)",
	}
	for _, id := range ids {
		sel := IDSelector(id)
		ps, err := ParseSightmapSelector(sel)
		if err != nil {
			t.Errorf("IDSelector(%q) = %q does not parse: %v", id, sel, err)
			continue
		}
		if len(ps.Parts) != 1 {
			t.Errorf("IDSelector(%q) = %q parsed into %d compounds, want 1", id, sel, len(ps.Parts))
			continue
		}
		if !Matches(&Element{Tag: "div", Id: id}, ps.Parts[0]) {
			t.Errorf("IDSelector(%q) = %q does not match its element", id, sel)
		}
		if Matches(&Element{Tag: "div", Id: id + "x", Classes: []string{"submit", "1", "product"}}, ps.Parts[0]) {
			t.Errorf("IDSelector(%q) = %q matches a different id", id, sel)
		}
	}
}

func TestSelectorStringEscapesID(t *testing.T) {
	tests := []struct {
		el   Element
		want string
	}{
		{Element{Tag: "button", Id: "submit", Classes: []string{"primary"}}, "button#submit.primary"},
		{Element{Tag: "button", Id: "checkout.submit"}, `button#checkout\.submit`},
		{Element{Tag: "li", Id: "1.circle", Classes: []string{"row"}}, `li#\31 \.circle.row`},
	}
	for _, tt := range tests {
		if got := tt.el.SelectorString(); got != tt.want {
			t.Errorf("SelectorString(%+v) = %q, want %q", tt.el, got, tt.want)
		}
	}
}
