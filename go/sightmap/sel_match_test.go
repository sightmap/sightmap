package sightmap_test

import (
	"github.com/sightmap/sightmap/go/sightmap"
	"testing"
)

func TestMatches_NilRule(t *testing.T) {
	node := nodeWith("div", "", nil, nil)
	if !sightmap.Matches(node, nil) {
		t.Error("nil rule should match everything")
	}
}

func TestMatches_Tag(t *testing.T) {
	node := nodeWith("button", "", nil, nil)
	rule := &sightmap.SelectorPart{Tag: "button"}
	if !sightmap.Matches(node, rule) {
		t.Error("button should match button rule")
	}
	ruleDiv := &sightmap.SelectorPart{Tag: "div"}
	if sightmap.Matches(node, ruleDiv) {
		t.Error("button should not match div rule")
	}
}

func TestMatches_TagCaseInsensitive(t *testing.T) {
	// node tag might come from DOM (lowercase) or from structured SelectorPart
	node := nodeWith("button", "", nil, nil)
	rule := &sightmap.SelectorPart{Tag: "BUTTON"}
	if !sightmap.Matches(node, rule) {
		t.Error("tag match should be case-insensitive")
	}
}

func TestMatches_MobileTagCaseInsensitive(t *testing.T) {
	// Synthetic mobile tag: node has "UIButton", parsed rule has "uibutton"
	node := nodeWith("UIButton", "", nil, nil)
	rule := &sightmap.SelectorPart{Tag: "uibutton"}
	if !sightmap.Matches(node, rule) {
		t.Error("UIButton should match uibutton rule (case-insensitive)")
	}
}

func TestMatches_ID(t *testing.T) {
	node := nodeWith("", "main-nav", nil, nil)
	if !sightmap.Matches(node, &sightmap.SelectorPart{Id: "main-nav"}) {
		t.Error("id match failed")
	}
	if sightmap.Matches(node, &sightmap.SelectorPart{Id: "other"}) {
		t.Error("wrong id should not match")
	}
}

func TestMatches_Classes(t *testing.T) {
	node := nodeWith("", "", []string{"btn", "primary", "large"}, nil)

	// Single class
	if !sightmap.Matches(node, &sightmap.SelectorPart{Classes: []string{"btn"}}) {
		t.Error("single class should match")
	}
	// Multiple classes (subset)
	if !sightmap.Matches(node, &sightmap.SelectorPart{Classes: []string{"btn", "primary"}}) {
		t.Error("class subset should match")
	}
	// Class not present
	if sightmap.Matches(node, &sightmap.SelectorPart{Classes: []string{"disabled"}}) {
		t.Error("absent class should not match")
	}
}

func TestMatches_AttrExact(t *testing.T) {
	node := nodeWith("input", "", nil, map[string]string{"type": "email"})
	rule := &sightmap.SelectorPart{Attrs: map[string]string{"type": "email"}}
	if !sightmap.Matches(node, rule) {
		t.Error("exact attr should match")
	}
	ruleBad := &sightmap.SelectorPart{Attrs: map[string]string{"type": "text"}}
	if sightmap.Matches(node, ruleBad) {
		t.Error("wrong attr value should not match")
	}
}

func TestMatches_AttrPresence(t *testing.T) {
	node := nodeWith("input", "", nil, map[string]string{"disabled": ""})
	rule := &sightmap.SelectorPart{
		Attrs:   map[string]string{"disabled": ""},
		AttrOps: map[string]string{"disabled": "[]"},
	}
	if !sightmap.Matches(node, rule) {
		t.Error("presence attr should match")
	}

	// Node without the attr
	nodeNoAttr := nodeWith("input", "", nil, nil)
	if sightmap.Matches(nodeNoAttr, rule) {
		t.Error("absent attr should not match presence rule")
	}
}

func TestMatches_AttrOnID_ResolvesToIdField(t *testing.T) {
	// id lives in the dedicated Id field, not Attrs. Attribute selectors on id
	// ([id^=...] etc.) must still resolve to it so they match offline like live.
	node := nodeWith("div", "issue_9f1c-42", nil, nil)

	prefix := &sightmap.SelectorPart{
		Attrs:   map[string]string{"id": "issue_"},
		AttrOps: map[string]string{"id": "^="},
	}
	if !sightmap.Matches(node, prefix) {
		t.Error(`[id^="issue_"] should match id="issue_9f1c-42"`)
	}

	exact := &sightmap.SelectorPart{Attrs: map[string]string{"id": "issue_9f1c-42"}}
	if !sightmap.Matches(node, exact) {
		t.Error(`[id="issue_9f1c-42"] should match`)
	}

	presence := &sightmap.SelectorPart{
		Attrs:   map[string]string{"id": ""},
		AttrOps: map[string]string{"id": "[]"},
	}
	if !sightmap.Matches(node, presence) {
		t.Error("[id] presence should match a node with an id")
	}

	bad := &sightmap.SelectorPart{
		Attrs:   map[string]string{"id": "cycle_"},
		AttrOps: map[string]string{"id": "^="},
	}
	if sightmap.Matches(node, bad) {
		t.Error(`[id^="cycle_"] should not match id="issue_9f1c-42"`)
	}

	noID := nodeWith("div", "", nil, nil)
	if sightmap.Matches(noID, presence) {
		t.Error("[id] presence should not match a node with no id")
	}
}

func TestMatches_AttrOnClass_ResolvesToClassesField(t *testing.T) {
	// class stored only in the Classes field (no Attrs["class"]) must still be
	// reachable by attribute selectors like [class*=...].
	node := nodeWith("svg", "", []string{"lucide", "lucide-check"}, nil)

	contains := &sightmap.SelectorPart{
		Attrs:   map[string]string{"class": "lucide-check"},
		AttrOps: map[string]string{"class": "*="},
	}
	if !sightmap.Matches(node, contains) {
		t.Error(`[class*="lucide-check"] should match Classes {lucide, lucide-check}`)
	}

	word := &sightmap.SelectorPart{
		Attrs:   map[string]string{"class": "lucide"},
		AttrOps: map[string]string{"class": "~="},
	}
	if !sightmap.Matches(node, word) {
		t.Error(`[class~="lucide"] should match`)
	}
}

func TestMatches_AttrPrefix(t *testing.T) {
	node := nodeWith("a", "", nil, map[string]string{"href": "https://example.com"})
	rule := &sightmap.SelectorPart{
		Attrs:   map[string]string{"href": "https://"},
		AttrOps: map[string]string{"href": "^="},
	}
	if !sightmap.Matches(node, rule) {
		t.Error("prefix attr should match")
	}
	ruleHttp := &sightmap.SelectorPart{
		Attrs:   map[string]string{"href": "http://"},
		AttrOps: map[string]string{"href": "^="},
	}
	if sightmap.Matches(node, ruleHttp) {
		t.Error("wrong prefix should not match")
	}
}

func TestMatches_AttrSuffix(t *testing.T) {
	node := nodeWith("a", "", nil, map[string]string{"href": "index.html"})
	rule := &sightmap.SelectorPart{
		Attrs:   map[string]string{"href": ".html"},
		AttrOps: map[string]string{"href": "$="},
	}
	if !sightmap.Matches(node, rule) {
		t.Error("suffix attr should match")
	}
}

func TestMatches_AttrSubstring(t *testing.T) {
	node := nodeWith("", "", nil, map[string]string{"aria-label": "Search products"})
	rule := &sightmap.SelectorPart{
		Attrs:   map[string]string{"aria-label": "Search"},
		AttrOps: map[string]string{"aria-label": "*="},
	}
	if !sightmap.Matches(node, rule) {
		t.Error("substring attr should match")
	}
}

func TestMatches_AttrIncludeWord(t *testing.T) {
	// ~= whitespace-separated word
	node := nodeWith("", "", nil, map[string]string{"class": "btn primary large"})
	rule := &sightmap.SelectorPart{
		Attrs:   map[string]string{"class": "primary"},
		AttrOps: map[string]string{"class": "~="},
	}
	if !sightmap.Matches(node, rule) {
		t.Error("~= should match word in list")
	}
	ruleBad := &sightmap.SelectorPart{
		Attrs:   map[string]string{"class": "prim"},
		AttrOps: map[string]string{"class": "~="},
	}
	if sightmap.Matches(node, ruleBad) {
		t.Error("~= should not match partial word")
	}
}

func TestMatches_AttrDashMatch(t *testing.T) {
	// |= matches exact or "value-"
	node := nodeWith("", "", nil, map[string]string{"lang": "en-US"})
	rule := &sightmap.SelectorPart{
		Attrs:   map[string]string{"lang": "en"},
		AttrOps: map[string]string{"lang": "|="},
	}
	if !sightmap.Matches(node, rule) {
		t.Error("|= should match 'en' against 'en-US'")
	}

	nodeExact := nodeWith("", "", nil, map[string]string{"lang": "en"})
	if !sightmap.Matches(nodeExact, rule) {
		t.Error("|= should match exact 'en'")
	}

	nodeFr := nodeWith("", "", nil, map[string]string{"lang": "fr"})
	if sightmap.Matches(nodeFr, rule) {
		t.Error("|= should not match 'fr' against 'en'")
	}
}

func TestMatches_Not(t *testing.T) {
	// button:not(.disabled) — enabled button matches, disabled does not
	enabled := nodeWith("button", "", []string{"primary"}, nil)
	disabled := nodeWith("button", "", []string{"primary", "disabled"}, nil)

	rule := &sightmap.SelectorPart{
		Tag: "button",
		Not: &sightmap.SelectorPart{Classes: []string{"disabled"}},
	}

	if !sightmap.Matches(enabled, rule) {
		t.Error("enabled button should match button:not(.disabled)")
	}
	if sightmap.Matches(disabled, rule) {
		t.Error("disabled button should not match button:not(.disabled)")
	}
}

func TestMatches_Is(t *testing.T) {
	// :is(.foo, .bar) — matches if node has class foo OR bar
	nodeFoo := nodeWith("div", "", []string{"foo"}, nil)
	nodeBar := nodeWith("div", "", []string{"bar"}, nil)
	nodeOther := nodeWith("div", "", []string{"other"}, nil)

	rule := &sightmap.SelectorPart{
		Is: []*sightmap.SelectorPart{
			{Classes: []string{"foo"}},
			{Classes: []string{"bar"}},
		},
	}
	if !sightmap.Matches(nodeFoo, rule) {
		t.Error("nodeFoo should match :is(.foo, .bar)")
	}
	if !sightmap.Matches(nodeBar, rule) {
		t.Error("nodeBar should match :is(.foo, .bar)")
	}
	if sightmap.Matches(nodeOther, rule) {
		t.Error("nodeOther should not match :is(.foo, .bar)")
	}
}

func TestMatches_ParsedAndMatched(t *testing.T) {
	// End-to-end: parse a selector string, match against a node.
	ps, err := sightmap.ParseSightmapSelector(`div[data-testid="add-to-cart"]`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rule := ps.Parts[0]

	match := nodeWith("div", "", nil, map[string]string{"data-testid": "add-to-cart"})
	noMatch := nodeWith("div", "", nil, map[string]string{"data-testid": "remove"})
	wrongTag := nodeWith("span", "", nil, map[string]string{"data-testid": "add-to-cart"})

	if !sightmap.Matches(match, rule) {
		t.Error("expected match")
	}
	if sightmap.Matches(noMatch, rule) {
		t.Error("wrong attr value should not match")
	}
	if sightmap.Matches(wrongTag, rule) {
		t.Error("wrong tag should not match")
	}
}
