package match_test

import (
	"reflect"
	"testing"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// el is a shorthand for an observed element identity.
func el(tag string, classes ...string) sightmap.Element {
	return sightmap.Element{Tag: tag, Classes: classes}
}

// chainMatcher builds a Matcher over global defs.
func chainMatcher(defs ...sightmap.ComponentDef) *match.Matcher {
	return match.NewMatcher(&sightmap.Corpus{GlobalComponents: defs})
}

func TestMatchChain_NearestEnclosingName(t *testing.T) {
	// CheckoutForm encloses an untagged SubmitButton child; a "click" on the
	// button resolves the nearest-enclosing name (SubmitButton), per spec.
	m := chainMatcher(
		sightmap.ComponentDef{Name: "CheckoutForm", Selectors: []string{"form.checkout"}},
		sightmap.ComponentDef{Name: "SubmitButton", Selectors: []string{"button.submit"}},
	)
	chain := []sightmap.Element{
		el("form", "checkout"),
		el("button", "submit"),
	}
	got := m.NamesForChain(chain, "")
	if want := []string{"SubmitButton"}; !reflect.DeepEqual(got, want) {
		t.Errorf("NamesForChain = %v, want %v", got, want)
	}
}

func TestMatchChain_NearestEnclosingFallsBackToAncestor(t *testing.T) {
	// The leaf itself matches nothing; the nearest enclosing match is an
	// ancestor. Nearest-enclosing therefore resolves the ancestor's name.
	m := chainMatcher(
		sightmap.ComponentDef{Name: "CheckoutForm", Selectors: []string{"form.checkout"}},
	)
	chain := []sightmap.Element{
		el("form", "checkout"),
		el("div", "row"),
		el("span"),
	}
	got := m.NamesForChain(chain, "")
	if want := []string{"CheckoutForm"}; !reflect.DeepEqual(got, want) {
		t.Errorf("NamesForChain = %v, want %v", got, want)
	}
}

func TestMatchChain_TagUnionAcrossLevels(t *testing.T) {
	// Tagged CheckoutForm ancestor, untagged SubmitButton leaf: identity is the
	// nearest (SubmitButton) but tags union the tagged ancestor in.
	m := chainMatcher(
		sightmap.ComponentDef{Name: "CheckoutForm", Selectors: []string{"form.checkout"}, Tags: []string{"defect"}},
		sightmap.ComponentDef{Name: "SubmitButton", Selectors: []string{"button.submit"}},
	)
	chain := []sightmap.Element{
		el("form", "checkout"),
		el("button", "submit"),
	}
	if got, want := m.NamesForChain(chain, ""), []string{"SubmitButton"}; !reflect.DeepEqual(got, want) {
		t.Errorf("NamesForChain = %v, want %v", got, want)
	}
	if got, want := m.TagsForChain(chain, ""), []string{"defect"}; !reflect.DeepEqual(got, want) {
		t.Errorf("TagsForChain = %v, want %v", got, want)
	}
}

func TestMatchChain_TagUnionDedupedAndSorted(t *testing.T) {
	m := chainMatcher(
		sightmap.ComponentDef{Name: "Outer", Selectors: []string{"div.outer"}, Tags: []string{"flaky", "defect"}},
		sightmap.ComponentDef{Name: "Inner", Selectors: []string{"button"}, Tags: []string{"defect", "slow"}},
	)
	chain := []sightmap.Element{
		el("div", "outer"),
		el("button"),
	}
	got := m.TagsForChain(chain, "")
	// Union {flaky,defect,slow}, deduplicated, lexicographically sorted.
	if want := []string{"defect", "flaky", "slow"}; !reflect.DeepEqual(got, want) {
		t.Errorf("TagsForChain = %v, want %v", got, want)
	}
}

func TestMatchChain_DepthAnnotation(t *testing.T) {
	m := chainMatcher(
		sightmap.ComponentDef{Name: "Outer", Selectors: []string{"div.outer"}},
		sightmap.ComponentDef{Name: "Inner", Selectors: []string{"button.submit"}},
	)
	chain := []sightmap.Element{
		el("div", "outer"),     // depth 0
		el("section"),          // depth 1 (matches nothing)
		el("button", "submit"), // depth 2
	}
	got := m.MatchChain(chain, "")
	want := []match.ChainMatch{
		{Depth: 0, Name: "Outer"},
		{Depth: 2, Name: "Inner"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MatchChain = %+v, want %+v", got, want)
	}
}

func TestMatchChain_DescendantSelectorAttributesToLeaf(t *testing.T) {
	// A descendant selector "nav a.link" completes at the anchor (depth 2), so
	// the match is attributed to that deepest node, not the nav.
	m := chainMatcher(
		sightmap.ComponentDef{Name: "NavLink", Selectors: []string{"nav a.link"}},
	)
	chain := []sightmap.Element{
		el("nav"),
		el("ul"),
		el("a", "link"),
	}
	got := m.MatchChain(chain, "")
	want := []match.ChainMatch{{Depth: 2, Name: "NavLink"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MatchChain = %+v, want %+v", got, want)
	}
}

func TestMatchChain_SameDepthConflictReturnsBothNames(t *testing.T) {
	// Two definitions match the leaf at the same (deepest) level: both surface,
	// rather than one being silently picked, so a caller can see the conflict.
	m := chainMatcher(
		sightmap.ComponentDef{Name: "Primary", Selectors: []string{"button.primary"}},
		sightmap.ComponentDef{Name: "Submit", Selectors: []string{"button.submit"}},
	)
	chain := []sightmap.Element{el("button", "primary", "submit")}
	got := m.NamesForChain(chain, "")
	if want := []string{"Primary", "Submit"}; !reflect.DeepEqual(got, want) {
		t.Errorf("NamesForChain = %v, want %v", got, want)
	}
}

func TestMatchChain_RouteAwareViewScoped(t *testing.T) {
	// A view-scoped component applies on the chain only under its route.
	corpus := &sightmap.Corpus{
		Views: []sightmap.ViewDef{
			{
				Name:  "Checkout",
				Route: "/checkout",
				Components: []sightmap.ComponentDef{
					{Name: "PayButton", Selectors: []string{"button.pay"}},
				},
			},
		},
	}
	m := match.NewMatcher(corpus)
	chain := []sightmap.Element{el("button", "pay")}

	if got, want := m.NamesForChain(chain, "https://x.test/checkout"), []string{"PayButton"}; !reflect.DeepEqual(got, want) {
		t.Errorf("on-route NamesForChain = %v, want %v", got, want)
	}
	if got := m.NamesForChain(chain, "https://x.test/other"); got != nil {
		t.Errorf("off-route NamesForChain = %v, want nil", got)
	}
}

func TestMatchChain_NoMatch(t *testing.T) {
	m := chainMatcher(
		sightmap.ComponentDef{Name: "Btn", Selectors: []string{"button"}},
	)
	chain := []sightmap.Element{el("div"), el("span")}
	if got := m.MatchChain(chain, ""); got != nil {
		t.Errorf("MatchChain = %v, want nil", got)
	}
	if got := m.NamesForChain(chain, ""); got != nil {
		t.Errorf("NamesForChain = %v, want nil", got)
	}
	if got := m.TagsForChain(chain, ""); got != nil {
		t.Errorf("TagsForChain = %v, want nil", got)
	}
}

func TestMatchChain_EmptyChain(t *testing.T) {
	m := chainMatcher(sightmap.ComponentDef{Name: "Btn", Selectors: []string{"button"}})
	if got := m.MatchChain(nil, ""); got != nil {
		t.Errorf("MatchChain(nil) = %v, want nil", got)
	}
}

func TestMatchChain_NoDefs(t *testing.T) {
	m := match.NewMatcher(&sightmap.Corpus{})
	if got := m.MatchChain([]sightmap.Element{el("button")}, ""); got != nil {
		t.Errorf("MatchChain = %v, want nil", got)
	}
}

func TestMatchChain_EffectivePrivacy(t *testing.T) {
	// The form's block reaches the undeclared input below it, a nearer unmask
	// replaces it for the total, and an unrelated sibling chain stays "".
	m := chainMatcher(
		sightmap.ComponentDef{Name: "CheckoutForm", Selectors: []string{"form.checkout"}, Privacy: "block"},
		sightmap.ComponentDef{Name: "CardNumber", Selectors: []string{"input.cc"}},
		sightmap.ComponentDef{Name: "OrderTotal", Selectors: []string{"span.total"}, Privacy: "unmask"},
		sightmap.ComponentDef{Name: "BuyButton", Selectors: []string{"button.buy"}},
	)
	privacyAt := func(chain []sightmap.Element) map[string]string {
		out := map[string]string{}
		for _, cm := range m.MatchChain(chain, "") {
			out[cm.Name] = cm.Privacy
		}
		return out
	}
	if got, want := privacyAt([]sightmap.Element{el("form", "checkout"), el("div"), el("input", "cc")}),
		map[string]string{"CheckoutForm": "block", "CardNumber": "block"}; !reflect.DeepEqual(got, want) {
		t.Errorf("inherited block: got %v, want %v", got, want)
	}
	if got, want := privacyAt([]sightmap.Element{el("form", "checkout"), el("span", "total")}),
		map[string]string{"CheckoutForm": "block", "OrderTotal": "unmask"}; !reflect.DeepEqual(got, want) {
		t.Errorf("nearer unmask: got %v, want %v", got, want)
	}
	if got, want := privacyAt([]sightmap.Element{el("main"), el("button", "buy")}),
		map[string]string{"BuyButton": ""}; !reflect.DeepEqual(got, want) {
		t.Errorf("no declaration: got %v, want %v", got, want)
	}
}

func TestMatchChain_PrivacyAgreesWithMatch(t *testing.T) {
	// Two definitions match the same node with different privacy. The chain
	// resolves it as Match does (first matching definition decides), and every
	// ChainMatch at that depth carries the one resolved value.
	m := chainMatcher(
		sightmap.ComponentDef{Name: "Masked", Selectors: []string{"div.panel"}, Privacy: "mask"},
		sightmap.ComponentDef{Name: "Open", Selectors: []string{"div.panel"}, Privacy: "unmask"},
	)
	chain := []sightmap.Element{el("div", "panel")}
	tree := &sightmap.ComponentNode{Element: &chain[0]}
	want := m.Match(tree, "")[tree].Privacy
	for _, cm := range m.MatchChain(chain, "") {
		if cm.Privacy != want {
			t.Errorf("%s: chain privacy %q, Match privacy %q", cm.Name, cm.Privacy, want)
		}
	}
}

func TestWithholds(t *testing.T) {
	ex := func(from, path string) sightmap.Extract { return sightmap.Extract{From: from, Path: path} }
	for _, tc := range []struct {
		privacy string
		e       sightmap.Extract
		want    bool
	}{
		{"", ex(sightmap.FromDOMText, ""), false},
		{"unmask", ex(sightmap.FromDOMText, ""), false},
		{"mask", ex(sightmap.FromDOMText, ""), true},
		{"block", ex(sightmap.FromDOMRawText, ""), true},
		{"mask", ex(sightmap.FromDOMAttr, "data-sku"), true},
		{"mask", ex(sightmap.FromDOMAttr, "checked"), false},
		{"block", ex(sightmap.FromDOMAttr, "checked"), true},
		{"mask", ex(sightmap.FromDOMState, "expanded"), false},
		{"block", ex(sightmap.FromDOMState, "expanded"), true},
		{"block", ex(sightmap.FromComponent, "Price.text"), false},
	} {
		if got := match.Withholds(tc.privacy, tc.e); got != tc.want {
			t.Errorf("Withholds(%q, %s %s) = %v, want %v", tc.privacy, tc.e.From, tc.e.Path, got, tc.want)
		}
	}
}

func TestMatchChain_SubtreeSelectorPrivacyNeedsItsArgumentOnTheChain(t *testing.T) {
	// A chain has no subtree, so section:has(input.cc) matches only when the
	// input is on the chain. Off the chain its block is not applied.
	m := chainMatcher(
		sightmap.ComponentDef{Name: "PaymentSection", Selectors: []string{"section:has(input.cc)"}, Privacy: "block"},
		sightmap.ComponentDef{Name: "Note", Selectors: []string{"span.note"}},
		sightmap.ComponentDef{Name: "Card", Selectors: []string{"input.cc"}},
	)
	privacyOf := func(chain []sightmap.Element, name string) string {
		for _, cm := range m.MatchChain(chain, "") {
			if cm.Name == name {
				return cm.Privacy
			}
		}
		t.Fatalf("%s did not match", name)
		return ""
	}
	if got := privacyOf([]sightmap.Element{el("section"), el("input", "cc")}, "Card"); got != "block" {
		t.Errorf("argument on the chain: privacy %q, want block", got)
	}
	if got := privacyOf([]sightmap.Element{el("section"), el("div"), el("span", "note")}, "Note"); got != "" {
		t.Errorf("argument off the chain: privacy %q, want \"\"", got)
	}
}
