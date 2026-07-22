package sel_test

import (
	"testing"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/sel"
)

func TestMatches_NilRule(t *testing.T) {
	node := nodeWith("div", "", nil, nil)
	if !sel.Matches(node, nil) {
		t.Error("nil rule should match everything")
	}
}

func TestMatches_Tag(t *testing.T) {
	node := nodeWith("button", "", nil, nil)
	rule := &comps.SelectorPart{Tag: "button"}
	if !sel.Matches(node, rule) {
		t.Error("button should match button rule")
	}
	ruleDiv := &comps.SelectorPart{Tag: "div"}
	if sel.Matches(node, ruleDiv) {
		t.Error("button should not match div rule")
	}
}

func TestMatches_TagCaseInsensitive(t *testing.T) {
	// node tag might come from DOM (lowercase) or from structured SelectorPart
	node := nodeWith("button", "", nil, nil)
	rule := &comps.SelectorPart{Tag: "BUTTON"}
	if !sel.Matches(node, rule) {
		t.Error("tag match should be case-insensitive")
	}
}

func TestMatches_MobileTagCaseInsensitive(t *testing.T) {
	// Synthetic mobile tag: node has "UIButton", parsed rule has "uibutton"
	node := nodeWith("UIButton", "", nil, nil)
	rule := &comps.SelectorPart{Tag: "uibutton"}
	if !sel.Matches(node, rule) {
		t.Error("UIButton should match uibutton rule (case-insensitive)")
	}
}

func TestMatches_ID(t *testing.T) {
	node := nodeWith("", "main-nav", nil, nil)
	if !sel.Matches(node, &comps.SelectorPart{Id: "main-nav"}) {
		t.Error("id match failed")
	}
	if sel.Matches(node, &comps.SelectorPart{Id: "other"}) {
		t.Error("wrong id should not match")
	}
}

func TestMatches_Classes(t *testing.T) {
	node := nodeWith("", "", []string{"btn", "primary", "large"}, nil)

	// Single class
	if !sel.Matches(node, &comps.SelectorPart{Classes: []string{"btn"}}) {
		t.Error("single class should match")
	}
	// Multiple classes (subset)
	if !sel.Matches(node, &comps.SelectorPart{Classes: []string{"btn", "primary"}}) {
		t.Error("class subset should match")
	}
	// Class not present
	if sel.Matches(node, &comps.SelectorPart{Classes: []string{"disabled"}}) {
		t.Error("absent class should not match")
	}
}

func TestMatches_AttrExact(t *testing.T) {
	node := nodeWith("input", "", nil, map[string]string{"type": "email"})
	rule := &comps.SelectorPart{Attrs: map[string]string{"type": "email"}}
	if !sel.Matches(node, rule) {
		t.Error("exact attr should match")
	}
	ruleBad := &comps.SelectorPart{Attrs: map[string]string{"type": "text"}}
	if sel.Matches(node, ruleBad) {
		t.Error("wrong attr value should not match")
	}
}

func TestMatches_AttrPresence(t *testing.T) {
	node := nodeWith("input", "", nil, map[string]string{"disabled": ""})
	rule := &comps.SelectorPart{
		Attrs:   map[string]string{"disabled": ""},
		AttrOps: map[string]string{"disabled": "[]"},
	}
	if !sel.Matches(node, rule) {
		t.Error("presence attr should match")
	}

	// Node without the attr
	nodeNoAttr := nodeWith("input", "", nil, nil)
	if sel.Matches(nodeNoAttr, rule) {
		t.Error("absent attr should not match presence rule")
	}
}

func TestMatches_AttrPrefix(t *testing.T) {
	node := nodeWith("a", "", nil, map[string]string{"href": "https://example.com"})
	rule := &comps.SelectorPart{
		Attrs:   map[string]string{"href": "https://"},
		AttrOps: map[string]string{"href": "^="},
	}
	if !sel.Matches(node, rule) {
		t.Error("prefix attr should match")
	}
	ruleHttp := &comps.SelectorPart{
		Attrs:   map[string]string{"href": "http://"},
		AttrOps: map[string]string{"href": "^="},
	}
	if sel.Matches(node, ruleHttp) {
		t.Error("wrong prefix should not match")
	}
}

func TestMatches_AttrSuffix(t *testing.T) {
	node := nodeWith("a", "", nil, map[string]string{"href": "index.html"})
	rule := &comps.SelectorPart{
		Attrs:   map[string]string{"href": ".html"},
		AttrOps: map[string]string{"href": "$="},
	}
	if !sel.Matches(node, rule) {
		t.Error("suffix attr should match")
	}
}

func TestMatches_AttrSubstring(t *testing.T) {
	node := nodeWith("", "", nil, map[string]string{"aria-label": "Search products"})
	rule := &comps.SelectorPart{
		Attrs:   map[string]string{"aria-label": "Search"},
		AttrOps: map[string]string{"aria-label": "*="},
	}
	if !sel.Matches(node, rule) {
		t.Error("substring attr should match")
	}
}

func TestMatches_AttrIncludeWord(t *testing.T) {
	// ~= whitespace-separated word
	node := nodeWith("", "", nil, map[string]string{"class": "btn primary large"})
	rule := &comps.SelectorPart{
		Attrs:   map[string]string{"class": "primary"},
		AttrOps: map[string]string{"class": "~="},
	}
	if !sel.Matches(node, rule) {
		t.Error("~= should match word in list")
	}
	ruleBad := &comps.SelectorPart{
		Attrs:   map[string]string{"class": "prim"},
		AttrOps: map[string]string{"class": "~="},
	}
	if sel.Matches(node, ruleBad) {
		t.Error("~= should not match partial word")
	}
}

func TestMatches_AttrDashMatch(t *testing.T) {
	// |= matches exact or "value-"
	node := nodeWith("", "", nil, map[string]string{"lang": "en-US"})
	rule := &comps.SelectorPart{
		Attrs:   map[string]string{"lang": "en"},
		AttrOps: map[string]string{"lang": "|="},
	}
	if !sel.Matches(node, rule) {
		t.Error("|= should match 'en' against 'en-US'")
	}

	nodeExact := nodeWith("", "", nil, map[string]string{"lang": "en"})
	if !sel.Matches(nodeExact, rule) {
		t.Error("|= should match exact 'en'")
	}

	nodeFr := nodeWith("", "", nil, map[string]string{"lang": "fr"})
	if sel.Matches(nodeFr, rule) {
		t.Error("|= should not match 'fr' against 'en'")
	}
}

func TestMatches_Not(t *testing.T) {
	// button:not(.disabled) — enabled button matches, disabled does not
	enabled := nodeWith("button", "", []string{"primary"}, nil)
	disabled := nodeWith("button", "", []string{"primary", "disabled"}, nil)

	rule := &comps.SelectorPart{
		Tag: "button",
		Not: &comps.SelectorPart{Classes: []string{"disabled"}},
	}

	if !sel.Matches(enabled, rule) {
		t.Error("enabled button should match button:not(.disabled)")
	}
	if sel.Matches(disabled, rule) {
		t.Error("disabled button should not match button:not(.disabled)")
	}
}

func TestMatches_Is(t *testing.T) {
	// :is(.foo, .bar) — matches if node has class foo OR bar
	nodeFoo := nodeWith("div", "", []string{"foo"}, nil)
	nodeBar := nodeWith("div", "", []string{"bar"}, nil)
	nodeOther := nodeWith("div", "", []string{"other"}, nil)

	rule := &comps.SelectorPart{
		Is: []*comps.SelectorPart{
			{Classes: []string{"foo"}},
			{Classes: []string{"bar"}},
		},
	}
	if !sel.Matches(nodeFoo, rule) {
		t.Error("nodeFoo should match :is(.foo, .bar)")
	}
	if !sel.Matches(nodeBar, rule) {
		t.Error("nodeBar should match :is(.foo, .bar)")
	}
	if sel.Matches(nodeOther, rule) {
		t.Error("nodeOther should not match :is(.foo, .bar)")
	}
}

func TestMatches_ParsedAndMatched(t *testing.T) {
	// End-to-end: parse a selector string, match against a node.
	ps, err := sel.ParseSightmapSelector(`div[data-testid="add-to-cart"]`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rule := ps.Parts[0]

	match := nodeWith("div", "", nil, map[string]string{"data-testid": "add-to-cart"})
	noMatch := nodeWith("div", "", nil, map[string]string{"data-testid": "remove"})
	wrongTag := nodeWith("span", "", nil, map[string]string{"data-testid": "add-to-cart"})

	if !sel.Matches(match, rule) {
		t.Error("expected match")
	}
	if sel.Matches(noMatch, rule) {
		t.Error("wrong attr value should not match")
	}
	if sel.Matches(wrongTag, rule) {
		t.Error("wrong tag should not match")
	}
}
