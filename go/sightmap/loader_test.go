package sightmap

import (
	"reflect"
	"testing"
)

func TestSplitSelectors_ParenAware(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		// No parens — simple split
		{"a, b", []string{"a", "b"}},
		// Comma inside parens — must NOT split
		{`:is([data-testid="main"], [data-testid="top"]) [data-testid="foo"]`,
			[]string{`:is([data-testid="main"], [data-testid="top"]) [data-testid="foo"]`}},
		// Multiple top-level selectors, one with :is()
		{`:is(a, b) span, div.bar`,
			[]string{`:is(a, b) span`, `div.bar`}},
		// Nested parens
		{`:is(:not(.foo), .bar), button`, []string{`:is(:not(.foo), .bar)`, `button`}},
		// Empty
		{"", nil},
		// Whitespace only between commas
		{"a,  , b", []string{"a", "b"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			got := splitSelectors(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("splitSelectors(%q)\n  got  %v\n  want %v", tc.input, got, tc.want)
			}
		})
	}
}
