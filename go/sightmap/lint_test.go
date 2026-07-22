package sightmap_test

import (
	"testing"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// hasRule reports whether any warning in ws has the given rule identifier.
func hasRule(ws []sightmap.LintWarning, rule string) bool {
	for _, w := range ws {
		if w.Rule == rule {
			return true
		}
	}
	return false
}

func TestLint_NameCase(t *testing.T) {
	// ".nav-link" is a class selector — won't trigger broad-tag-selector.
	c := corpusFrom([]match.SightmapComponent{
		{Name: "navLink", Selectors: []string{".nav-link"}},
	}, nil)
	warnings := sightmap.Lint(c)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
	if warnings[0].Rule != "name-case" {
		t.Errorf("expected rule 'name-case', got %q", warnings[0].Rule)
	}
}

func TestLint_BroadTagGlobal(t *testing.T) {
	c := corpusFrom([]match.SightmapComponent{
		{Name: "Button", Selectors: []string{"button"}},
	}, nil)
	warnings := sightmap.Lint(c)
	// Bare "button" triggers both broad-tag-selector and multi-instance-no-property
	if !hasRule(warnings, "broad-tag-selector") {
		t.Errorf("expected rule 'broad-tag-selector', got: %v", warnings)
	}
	if !hasRule(warnings, "multi-instance-no-property") {
		t.Errorf("expected rule 'multi-instance-no-property', got: %v", warnings)
	}
}

func TestLint_BroadTagInView(t *testing.T) {
	// Bare tag selector inside a view should NOT trigger broad-tag-selector,
	// but WILL trigger multi-instance-no-property.
	c := corpusFrom(nil, []sightmap.View{
		{
			Name:  "Page",
			Route: "/",
			Components: []match.SightmapComponent{
				{Name: "Button", Selectors: []string{"button"}},
			},
		},
	})
	warnings := sightmap.Lint(c)
	for _, w := range warnings {
		if w.Rule == "broad-tag-selector" {
			t.Errorf("unexpected broad-tag-selector warning in view: %v", w)
		}
	}
	// But multi-instance-no-property IS expected
	if !hasRule(warnings, "multi-instance-no-property") {
		t.Errorf("expected 'multi-instance-no-property' warning for bare button in view; got: %v", warnings)
	}
}

func TestLint_IdHash(t *testing.T) {
	c := corpusFrom([]match.SightmapComponent{
		{Name: "Root", Selectors: []string{"div#root-12345"}},
	}, nil)
	warnings := sightmap.Lint(c)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
	if warnings[0].Rule != "id-hash-selector" {
		t.Errorf("expected rule 'id-hash-selector', got %q", warnings[0].Rule)
	}
}

func TestLint_DeepNesting(t *testing.T) {
	c := corpusFrom([]match.SightmapComponent{
		{Name: "Input", Selectors: []string{"form div section input"}},
	}, nil)
	warnings := sightmap.Lint(c)
	// Bare "input" triggers both deep-nesting and multi-instance-no-property
	if !hasRule(warnings, "deep-nesting") {
		t.Errorf("expected rule 'deep-nesting', got: %v", warnings)
	}
	if !hasRule(warnings, "multi-instance-no-property") {
		t.Errorf("expected rule 'multi-instance-no-property', got: %v", warnings)
	}
}

func TestLint_CommaInSelector(t *testing.T) {
	// "a, button" contains a comma that should have been split by the loader.
	// It also fails ParseSightmapSelector (comma stops the parse), so we
	// expect both a "validation" warning and a "comma-in-selector" warning.
	c := corpusFrom([]match.SightmapComponent{
		{Name: "Link", Selectors: []string{"a, button"}},
	}, nil)
	warnings := sightmap.Lint(c)
	if !hasRule(warnings, "comma-in-selector") {
		t.Errorf("expected a 'comma-in-selector' warning; got: %v", warnings)
	}
}

func TestLint_ValidationErrors(t *testing.T) {
	// A component with an empty name is a validation error; Lint should
	// surface it as a warning with Rule="validation".
	c := corpusFrom([]match.SightmapComponent{
		{Name: "", Selectors: []string{"div"}},
	}, nil)
	warnings := sightmap.Lint(c)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
	if warnings[0].Rule != "validation" {
		t.Errorf("expected rule 'validation', got %q", warnings[0].Rule)
	}
}

func TestLint_DeepNestingFalsePositive(t *testing.T) {
	// Selector with spaces INSIDE a quoted attribute value should NOT trigger
	// deep-nesting — only 2 real selector parts (1 combinator).
	c := corpusFrom([]match.SightmapComponent{
		{Name: "LogoLink", Selectors: []string{`#nav-header a[aria-label="Link to homepage"]`}},
	}, nil)
	warnings := sightmap.Lint(c)
	for _, w := range warnings {
		if w.Rule == "deep-nesting" {
			t.Errorf("false deep-nesting warning for selector with spaces inside quoted attr: %v", w)
		}
	}
}

func TestLint_CommaInIsNotFlagged(t *testing.T) {
	// A comma inside :is() is valid and must NOT trigger comma-in-selector.
	c := corpusFrom([]match.SightmapComponent{
		{Name: "Content", Selectors: []string{`:is([data-testid="main"], [data-testid="top"]) [data-testid="foo"]`}},
	}, nil)
	warnings := sightmap.Lint(c)
	for _, w := range warnings {
		if w.Rule == "comma-in-selector" {
			t.Errorf("false comma-in-selector warning for comma inside :is(): %v", w)
		}
	}
}

func TestLint_NoDuplicateWarnings(t *testing.T) {
	// A global component referenced via $ref in two views should produce
	// at most one warning per (rule, component, selector) triplet.
	global := match.SightmapComponent{Name: "Button", Selectors: []string{"button"}}
	c := corpusFrom(
		[]match.SightmapComponent{global},
		[]sightmap.View{
			{Name: "PageA", Route: "/a", Components: []match.SightmapComponent{global}},
			{Name: "PageB", Route: "/b", Components: []match.SightmapComponent{global}},
		},
	)
	warnings := sightmap.Lint(c)
	// Count broad-tag-selector warnings for Button — must be exactly 1.
	count := 0
	for _, w := range warnings {
		if w.Rule == "broad-tag-selector" && w.Component == "Button" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 broad-tag-selector warning for Button, got %d", count)
	}
}

func TestLint_Clean(t *testing.T) {
	// nav.navbar: has a class, so it is not a bare-tag selector.
	c := corpusFrom(
		[]match.SightmapComponent{
			{Name: "NavBar", Selectors: []string{"nav.navbar"}},
		},
		[]sightmap.View{
			{
				Name:       "Home",
				Route:      "/",
				Components: []match.SightmapComponent{{Name: "Hero", Selectors: []string{".hero"}}},
			},
		},
	)
	warnings := sightmap.Lint(c)
	if len(warnings) != 0 {
		t.Fatalf("expected 0 warnings, got %d: %v", len(warnings), warnings)
	}
}

// TestLint_MultiInstanceNoProperty tests that components with broad selectors
// and no properties trigger the multi-instance-no-property warning.
func TestLint_MultiInstanceNoProperty_Button(t *testing.T) {
	c := corpusFrom([]match.SightmapComponent{
		{Name: "DeleteButton", Selectors: []string{"button.btn-danger"}},
	}, nil)

	warnings := sightmap.Lint(c)

	if !hasRule(warnings, "multi-instance-no-property") {
		t.Errorf("expected 'multi-instance-no-property' warning; got: %v", warnings)
	}
	// Verify the warning is for DeleteButton
	for _, w := range warnings {
		if w.Rule == "multi-instance-no-property" && w.Component != "DeleteButton" {
			t.Errorf("expected warning for DeleteButton, got %q", w.Component)
		}
	}
}

func TestLint_MultiInstanceNoProperty_Card(t *testing.T) {
	c := corpusFrom([]match.SightmapComponent{
		{Name: "ProductCard", Selectors: []string{`[data-component="product-card"]`}},
	}, nil)

	warnings := sightmap.Lint(c)

	if !hasRule(warnings, "multi-instance-no-property") {
		t.Errorf("expected 'multi-instance-no-property' warning; got: %v", warnings)
	}
}

func TestLint_MultiInstanceWithProperties_NoWarn(t *testing.T) {
	c := corpusFrom([]match.SightmapComponent{
		{
			Name:      "ProductCard",
			Selectors: []string{`[data-component="product-card"]`},
			Properties: []match.Property{
				{Name: "sku", Extract: "attr:data-sku"},
			},
		},
	}, nil)

	warnings := sightmap.Lint(c)

	for _, w := range warnings {
		if w.Rule == "multi-instance-no-property" {
			t.Errorf("unexpected 'multi-instance-no-property' warning for component with properties: %v", w)
		}
	}
}

func TestLint_Singleton_NoWarn(t *testing.T) {
	// Singletons like SiteHeader should not warn even without properties
	c := corpusFrom([]match.SightmapComponent{
		{Name: "SiteHeader", Selectors: []string{`header[role="banner"]`}},
	}, nil)

	warnings := sightmap.Lint(c)

	for _, w := range warnings {
		if w.Rule == "multi-instance-no-property" {
			t.Errorf("unexpected 'multi-instance-no-property' warning for singleton: %v", w)
		}
	}
}

func TestLint_UniqueSelector_NoWarn(t *testing.T) {
	// Selectors with data-testid are unique and should not warn
	c := corpusFrom([]match.SightmapComponent{
		{Name: "SubmitButton", Selectors: []string{`button[data-testid="submit-form"]`}},
	}, nil)

	warnings := sightmap.Lint(c)

	for _, w := range warnings {
		if w.Rule == "multi-instance-no-property" {
			t.Errorf("unexpected 'multi-instance-no-property' warning for unique selector: %v", w)
		}
	}
}

func TestLint_ColonAnchoredDataComponent_NoWarn(t *testing.T) {
	// [data-component*=":Name:"] is colon-anchored on both sides — specific in
	// module:ComponentName:version registries. Must NOT warn.
	cases := []struct {
		name string
		sel  string
	}{
		{"double-quoted", `[data-component*=":ProductDetails:"]`},
		{"single-quoted", `[data-component*=':ShareBelt:']`},
		{"with-module-prefix", `[data-component*=":Buybox:"]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := corpusFrom([]match.SightmapComponent{
				{Name: "TheComp", Selectors: []string{tc.sel}},
			}, nil)
			for _, w := range sightmap.Lint(c) {
				if w.Rule == "multi-instance-no-property" {
					t.Errorf("unexpected multi-instance warning for colon-anchored selector %q: %v", tc.sel, w)
				}
			}
		})
	}

	// ^= prefix is still broad and SHOULD warn
	c := corpusFrom([]match.SightmapComponent{
		{Name: "TheComp", Selectors: []string{`[data-component^="product-details:ProductDetails"]`}},
	}, nil)
	hasWarn := false
	for _, w := range sightmap.Lint(c) {
		if w.Rule == "multi-instance-no-property" {
			hasWarn = true
		}
	}
	if !hasWarn {
		t.Error("expected multi-instance warning for ^= prefix data-component selector")
	}
}

func TestLint_UniqueIdSelector_NoWarn(t *testing.T) {
	// Selectors with #id are unique and should not warn
	c := corpusFrom([]match.SightmapComponent{
		{Name: "MainNav", Selectors: []string{"#main-navigation"}},
	}, nil)

	warnings := sightmap.Lint(c)

	for _, w := range warnings {
		if w.Rule == "multi-instance-no-property" {
			t.Errorf("unexpected 'multi-instance-no-property' warning for #id selector: %v", w)
		}
	}
}

func TestLint_ChildRepeatsParent_Warns(t *testing.T) {
	// A child component with a flattened selector that repeats the ancestor class
	// triggers child-repeats-parent. flattenOne in loader.go prepends the parent's
	// selector with a space, so if the raw child YAML was ".dashboard .freelancer-card"
	// and the parent matched ".dashboard", the stored Selectors entry is
	// ".dashboard .dashboard .freelancer-card" — the doubled form we detect here.
	c := corpusFrom(nil, []sightmap.View{
		{
			Name:  "DashboardPage",
			Route: "/dashboard",
			Components: []match.SightmapComponent{
				{
					Name:        "FreelancerCard",
					Selectors:   []string{".dashboard .dashboard .freelancer-card"},
					ParentChain: []string{"Dashboard"},
				},
			},
		},
	})
	warnings := sightmap.Lint(c)
	if !hasRule(warnings, "child-repeats-parent") {
		t.Errorf("expected 'child-repeats-parent' warning for doubled child selector; got: %v", warnings)
	}
}

func TestLint_ChildRepeatsParent_MultiClass(t *testing.T) {
	// Repeated class in a longer chain still fires.
	c := corpusFrom(nil, []sightmap.View{
		{
			Name:  "Page",
			Route: "/",
			Components: []match.SightmapComponent{
				{
					Name:        "HelpLink",
					Selectors:   []string{".help-card .help-card a"},
					ParentChain: []string{"HelpCard"},
				},
			},
		},
	})
	warnings := sightmap.Lint(c)
	if !hasRule(warnings, "child-repeats-parent") {
		t.Errorf("expected 'child-repeats-parent' warning for doubled selector in chain; got: %v", warnings)
	}
}

func TestLint_ChildRepeatsParent_NoWarnCorrect(t *testing.T) {
	// A correctly-scoped child (no repeated class) must NOT trigger the rule.
	c := corpusFrom(nil, []sightmap.View{
		{
			Name:  "DashboardPage",
			Route: "/dashboard",
			Components: []match.SightmapComponent{
				{
					Name:        "FreelancerCard",
					Selectors:   []string{".freelancer-card"},
					ParentChain: []string{"Dashboard"},
				},
			},
		},
	})
	warnings := sightmap.Lint(c)
	for _, w := range warnings {
		if w.Rule == "child-repeats-parent" {
			t.Errorf("unexpected 'child-repeats-parent' warning for correct child selector: %v", w)
		}
	}
}

func TestLint_ChildRepeatsParent_NoWarnGlobal(t *testing.T) {
	// A top-level global component with a multi-class selector like ".x .x" must
	// NOT trigger child-repeats-parent — the rule is gated on ParentChain.
	c := corpusFrom([]match.SightmapComponent{
		{
			Name:        "WeirdGlobal",
			Selectors:   []string{".nav .nav"},
			ParentChain: nil, // no parent
		},
	}, nil)
	warnings := sightmap.Lint(c)
	for _, w := range warnings {
		if w.Rule == "child-repeats-parent" {
			t.Errorf("unexpected 'child-repeats-parent' warning for global component (no ParentChain): %v", w)
		}
	}
}

func TestLint_ChildComponent_NoWarn(t *testing.T) {
	// Child components scoped to parent should not warn
	c := corpusFrom(nil, []sightmap.View{
		{
			Name:  "ProductPage",
			Route: "/product",
			Components: []match.SightmapComponent{
				{
					Name:      "ProductGrid",
					Selectors: []string{`[data-testid="product-grid"]`},
					Properties: []match.Property{
						{Name: "ProductCard", Extract: ".product-card"},
					},
				},
			},
		},
	})

	warnings := sightmap.Lint(c)

	// ProductCard is a child property, not a standalone component, so no warning expected
	for _, w := range warnings {
		if w.Rule == "multi-instance-no-property" {
			t.Errorf("unexpected 'multi-instance-no-property' warning: %v", w)
		}
	}
}
