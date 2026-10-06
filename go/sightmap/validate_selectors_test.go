package sightmap

import (
	"strings"
	"testing"
)

func TestValidateComponentSelectors(t *testing.T) {
	type want struct{ code, severity string }
	cases := []struct {
		name string
		comp ComponentDef
		want []want
	}{
		{"portable privacy selector is clean", ComponentDef{Name: "A", Selectors: []string{`input[name="cc"]`}, Privacy: "block"}, nil},
		{"portable naming selector is clean", ComponentDef{Name: "A", Selectors: []string{`.card > .title`}}, nil},
		{"privacy with :has is an error", ComponentDef{Name: "A", Selectors: []string{`section:has(.x)`}, Privacy: "mask"}, []want{{CodeSelectorNotChainEvaluable, "error"}}},
		{"watch with :is is an error", ComponentDef{Name: "A", Selectors: []string{`:is(.a, .b)`}, Watch: true}, []want{{CodeSelectorNotInProfile, "error"}}},
		{"naming with :has is a warning", ComponentDef{Name: "A", Selectors: []string{`section:has(.x)`}}, []want{{CodeSelectorNotChainEvaluable, "warning"}}},
		{"privacy universal subject is an error", ComponentDef{Name: "A", Selectors: []string{`.card > *`}, Privacy: "unmask"}, []want{{CodeSelectorUniversalSubject, "error"}}},
		{"naming universal subject is a warning", ComponentDef{Name: "A", Selectors: []string{`*`}}, []want{{CodeSelectorUniversalSubject, "warning"}}},
		{"bare :not subject is universal", ComponentDef{Name: "A", Selectors: []string{`:not(.x)`}, Privacy: "block"}, []want{{CodeSelectorUniversalSubject, "error"}}},
		{"unmask on a bare type warns", ComponentDef{Name: "A", Selectors: []string{`body`}, Privacy: "unmask"}, []want{{CodePrivacyUnmaskBroad, "warning"}}},
		{"block on a bare type is fine", ComponentDef{Name: "A", Selectors: []string{`form`}, Privacy: "block"}, nil},
		{"each alternative is checked", ComponentDef{Name: "A", Selectors: []string{`.ok`, `li:has(.x)`}, Privacy: "block"}, []want{{CodeSelectorNotChainEvaluable, "error"}}},
		{"parse failures are reported elsewhere", ComponentDef{Name: "A", Selectors: []string{`[unclosed`}, Privacy: "block"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := validateComponentSelectors(c.comp)
			if len(got) != len(c.want) {
				t.Fatalf("got %d findings %v, want %d", len(got), got, len(c.want))
			}
			for i, f := range got {
				sev := "warning"
				if f.IsError() {
					sev = "error"
				}
				if f.Code != c.want[i].code || sev != c.want[i].severity {
					t.Errorf("finding %d = %s/%s, want %s/%s", i, f.Code, sev, c.want[i].code, c.want[i].severity)
				}
			}
		})
	}
}

func TestValidateComponentSelectors_NamesParentsForAChild(t *testing.T) {
	child := ComponentDef{Name: "Cvc", Selectors: []string{`form:has(.pay) input.cvc`}, Privacy: "block", ParentChain: []string{"Checkout"}}
	got := validateComponentSelectors(child)
	if len(got) != 1 || !strings.Contains(got[0].Message, "Checkout") {
		t.Fatalf("want one finding naming the parent Checkout, got %v", got)
	}
}

func TestValidate_SiblingCombinatorParseErrorIsCoded(t *testing.T) {
	errs := validateComponent(ComponentDef{Name: "A", Selectors: []string{`.a + .b`}}, map[string]string{})
	if len(errs) != 1 || errs[0].Code != CodeSelectorNotChainEvaluable || !errs[0].IsError() {
		t.Fatalf("want one coded parse error, got %v", errs)
	}
}
