package sightmap

import (
	"strings"
	"testing"
)

// TestClassAttrMatchesAgreesWithJoin checks that the per-class evaluation gives
// the same answer as matching against the space-joined class list, whenever it
// claims to apply.
func TestClassAttrMatchesAgreesWithJoin(t *testing.T) {
	classLists := [][]string{
		{"card", "card-link"},
		{"a", "b", "c"},
		{"x__value", "y"},
		{"en", "en-US"},
		{"en-US", "en"},
		{"", "a"},
		{"a", ""},
		{"a\tb", "c"},
		{"card", "card"},
	}
	ruleVals := []string{"", "card", "card-link", "a", "b", "c", "a b", "b c", "__value", "link", "en", "en-US", "US", "-", "x", "\t", "a\tb", "ard"}
	ops := []string{"=", "[]", "^=", "$=", "*=", "~=", "|=", "?"}

	applied := 0
	for _, classes := range classLists {
		joined := strings.Join(classes, " ")
		for _, rv := range ruleVals {
			for _, op := range ops {
				got, ok := classAttrMatches(op, classes, rv)
				if !ok {
					continue
				}
				applied++
				if want := attrMatches(op, joined, rv); got != want {
					t.Errorf("classes=%q op=%q ruleVal=%q: got %v, want %v", classes, op, rv, got, want)
				}
			}
		}
	}
	if applied == 0 {
		t.Fatal("classAttrMatches never applied")
	}
}
