package sightmap

import "testing"

func TestMoreSpecific(t *testing.T) {
	cases := []struct {
		a, b string
		more bool
	}{
		// Simple selectors.
		{"span.label", "span", true},
		{"span", "span.label", false},
		{"span.a.b", "span.a", true},
		{"span.a", "span.a", false},
		{"span.a", "div.a", false},
		{"span.a", ".a", true},
		{"#x", "[id]", true},
		{"#x", "[id=x]", false}, // equivalent, not strictly narrower
		{"button[type=submit]", "button[type]", true},
		// Negations order only selectors whose positive parts are equivalent.
		{"button:not(.x)", "button", true},
		{"button", "button:not(.x)", false},
		{"button:not([class*=foo])", "button:not(.foobar)", true},
		{"button:not(.foobar)", "button:not([class*=foo])", false},
		{"button:not(.x):not(.y)", "button:not(.x)", true},
		{"button:not(.x)", "button:not(.y)", false},
		{"span[tabindex]", "span:not(.label)", true}, // what it requires beats what it excludes
		{"span:not(.label)", "span[tabindex]", false},
		{"span.a:not(.x)", "span.a.b", false},
		{"span.a.b", "span.a:not(.x)", true},
		{"div:has(> span)", "div", true},
		{"div:has(> span)", "div:has(> p)", false},
		{"div:is(.a)", "div", true},
		{"div", "div:is(.a)", false},

		// Attribute operators.
		{`[a^="row-1-"]`, `[a^="row-"]`, true},
		{`[a$="-label-text"]`, `[a$="-text"]`, true},
		{`[a^="values-6-"][a$="-label-text"]`, `[a^="values-"][a$="-text"]`, true},
		{`[a^="values-"][a$="-text"]`, `[a^="values-6-"][a$="-label-text"]`, false},
		{`[a="row-1-text"]`, `[a^="row-"][a$="text"][a*="1"]`, true},
		{`[a*="stepBadgeWrap"]`, `[a*="stepBadge"]`, true},
		{`.stepBadgeWrap`, `[class*="stepBadge"]`, true},
		{`[class*="stepBadge"]`, `.stepBadgeWrap`, false},
		{`[a="x y"]`, `[a~="y"]`, true},
		{`[a="xy"]`, `[a~="y"]`, false},
		{`[a="en-US"]`, `[a|="en"]`, true},
		{`[a|="en"]`, `[a^="en"]`, true},
		{`[a^="en"]`, `[a|="en"]`, false},
		{`[a^="Foo"]`, `[a^="foo"]`, false}, // values compare exactly

		// Chains.
		{"[data-component=Card] button", "button", true},
		{"[data-component=Card] > button", "[data-component=Card] button", true},
		{"[data-component=Card] button", "[data-component=Card] > button", false},
		{"[data-component=Card] button.primary", "[data-component=Card] button", true},
		{"[data-component=Card] button", "button.primary", false},
		{"button", "[data-component=Card] button", false},
		{"div p span", "div span", true}, // a descendant gap may hold anything
		{"div > p > span", "div span", true},
		{"div p > span", "div > span", false},
		{"label span span[data-testid=x]", "label span", true}, // a child definition's chain
		{"form div#sel svg.icon", "form svg", true},
		{"form div#sel svg.icon", "form > svg", false},
		{"x > y w > y z", "x > y z", true}, // the embedding that is not the nearest
	}
	for _, c := range cases {
		a, err := ParseSightmapSelector(c.a)
		if err != nil {
			t.Fatalf("%s: %v", c.a, err)
		}
		b, err := ParseSightmapSelector(c.b)
		if err != nil {
			t.Fatalf("%s: %v", c.b, err)
		}
		if got := a.MoreSpecific(b); got != c.more {
			t.Errorf("MoreSpecific(%s, %s) = %v, want %v", c.a, c.b, got, c.more)
		}
	}
}
