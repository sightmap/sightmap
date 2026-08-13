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
