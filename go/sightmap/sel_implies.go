package sightmap

import (
	"reflect"
	"strings"
)

// MoreSpecific reports whether a is strictly more specific than b. It is the
// relation the matcher uses to name an element claimed by more than one
// definition (SEP-0023): the more specific definition wins, so `span.label`
// beats `span` and `[data-id^="row-"][data-id$="-price"]` beats
// `[data-id$="-price"]` without either author writing a :not().
//
// Selectors are ordered by what they REQUIRE first and what they EXCLUDE
// second: a is more specific than b when a's positive selector (every :not()
// removed) implies b's and not the reverse, or when the two positive
// selectors imply each other and a's negations cover b's and not the reverse.
// Negations come second because they describe the other definitions an
// author had to fence off, not the element: `span[tabindex]` is the more
// specific description of a focusable span than `span:not(.label)`, though
// neither implies the other.
//
// The test is syntactic and deliberately incomplete: it recognises implication
// it can decide from the selector text alone, and never claims one that does
// not hold. Two selectors it cannot order are left as they are, so a consumer
// reports the overlap rather than resolving it silently.
func (a ParsedSelector) MoreSpecific(b ParsedSelector) bool {
	ab, ba := a.impliesPositive(b), b.impliesPositive(a)
	switch {
	case ab && !ba:
		return true
	case ab && ba:
		return a.coversNegations(b) && !b.coversNegations(a)
	}
	return false
}

// Implies reports whether every element a matches, b matches too, as far as
// the selector text can show: a's positive selector implies b's, and a's
// negations cover b's. Chains are compared from the subject compound
// outward: a's compound must imply b's at each position, a child combinator
// implies a descendant one, and a may carry further compounds b does not (an
// extra ancestor constraint only narrows). When b has compounds a does not,
// nothing can be said.
func (a ParsedSelector) Implies(b ParsedSelector) bool {
	return a.impliesPositive(b) && a.coversNegations(b)
}

// impliesPositive is Implies with every :not() on both sides ignored.
func (a ParsedSelector) impliesPositive(b ParsedSelector) bool {
	return alignChains(a, b, impliesPositivePart)
}

// coversNegations reports whether, at every aligned compound, each :not(y) of
// b has a :not(x) of a that y implies: whatever x excludes covers whatever y
// excludes. It says nothing about the positive selectors; callers pair it with
// impliesPositive.
func (a ParsedSelector) coversNegations(b ParsedSelector) bool {
	return alignChains(a, b, coversNegationsPart)
}

// alignChains embeds b's chain in a's, from the subject outward: the subjects
// align, and each further compound of b aligns with some compound of a to the
// left of the last alignment, under part. A child combinator in b requires
// the aligned compounds of a to be adjacent and joined by a child combinator;
// a descendant combinator lets any compounds of a lie between (`label span`
// is implied by `label > div span`, as in CSS). Compounds of a left over
// beyond b's first are extra ancestor constraints and only narrow.
func alignChains(a, b ParsedSelector, part func(a, b *SelectorPart) bool) bool {
	if len(b.Parts) == 0 || len(b.Parts) > len(a.Parts) {
		return len(b.Parts) == 0 && len(a.Parts) == 0
	}
	var embed func(ai, bi int) bool
	embed = func(ai, bi int) bool {
		if !part(a.Parts[ai], b.Parts[bi]) {
			return false
		}
		if bi == 0 {
			return true
		}
		if ai == 0 {
			return false
		}
		if combinatorBefore(b.Combinators, bi) == ">" {
			return combinatorBefore(a.Combinators, ai) == ">" && embed(ai-1, bi-1)
		}
		for j := ai - 1; j >= bi-1; j-- {
			if embed(j, bi-1) {
				return true
			}
		}
		return false
	}
	return embed(len(a.Parts)-1, len(b.Parts)-1)
}

// combinatorBefore is the combinator preceding Parts[i], "" for the first.
func combinatorBefore(comb []string, i int) string {
	if i < len(comb) {
		return strings.TrimSpace(comb[i])
	}
	return ""
}

// impliesPositivePart reports whether compound a implies compound b with the
// negations of both ignored: every other simple selector of b is implied by
// one of a's.
func impliesPositivePart(a, b *SelectorPart) bool {
	if a == nil || b == nil {
		return b == nil
	}
	if b.Tag != "" && b.Tag != "*" && a.Tag != b.Tag {
		return false
	}
	at := attrTests(a)
	for _, bt := range attrTests(b) {
		if !impliedByAny(at, bt) {
			return false
		}
	}
	// Relational and logical pseudo-classes are opaque: one of b's is implied
	// only by an identical one of a's.
	for _, h := range b.Has {
		if !containsDeepEqual(a.Has, h) {
			return false
		}
	}
	if len(b.Is) > 0 && !reflect.DeepEqual(a.Is, b.Is) && !containsDeepEqual(a.AndIs, b.Is) {
		return false
	}
	for _, l := range b.AndIs {
		if !reflect.DeepEqual(a.Is, l) && !containsDeepEqual(a.AndIs, l) {
			return false
		}
	}
	return true
}

// coversNegationsPart reports whether each :not(y) of b has a :not(x) of a
// that y implies.
func coversNegationsPart(a, b *SelectorPart) bool {
	if a == nil || b == nil {
		return b == nil
	}
	for _, y := range b.Not {
		found := false
		for _, x := range a.Not {
			if y.Implies(x) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func containsDeepEqual[T any](xs []T, x T) bool {
	for _, y := range xs {
		if reflect.DeepEqual(y, x) {
			return true
		}
	}
	return false
}

// attrTests lists a compound's attribute tests in one vocabulary: #id as
// [id="x"], .c as [class~="c"], then the attribute selectors themselves.
func attrTests(p *SelectorPart) []AttrTest {
	var out []AttrTest
	if p.Id != "" {
		out = append(out, AttrTest{Key: "id", Op: "=", Value: p.Id})
	}
	for _, c := range p.Classes {
		out = append(out, AttrTest{Key: "class", Op: "~=", Value: c})
	}
	for k, v := range p.Attrs {
		op := p.AttrOps[k]
		if op == "" {
			op = "="
		}
		out = append(out, AttrTest{Key: k, Op: op, Value: v})
	}
	return append(out, p.RepeatAttrs...)
}

func impliedByAny(tests []AttrTest, t AttrTest) bool {
	for _, s := range tests {
		if impliesTest(s, t) {
			return true
		}
	}
	return false
}

// impliesTest reports whether attribute test s implies t. Values compare
// exactly; a browser's case-insensitive attributes are not special-cased, which
// only ever withholds an implication.
func impliesTest(s, t AttrTest) bool {
	if s.Key != t.Key {
		return false
	}
	switch t.Op {
	case "[]":
		return true
	case "^=", "$=", "*=":
		// CSS matches nothing for an empty value here, so no test implies it.
		if t.Value == "" {
			return false
		}
	}
	switch t.Op {
	case "=":
		return s.Op == "=" && s.Value == t.Value
	case "^=":
		switch s.Op {
		case "=", "^=", "|=":
			return strings.HasPrefix(s.Value, t.Value)
		}
	case "$=":
		switch s.Op {
		case "=", "$=":
			return strings.HasSuffix(s.Value, t.Value)
		}
	case "*=":
		switch s.Op {
		case "=", "^=", "$=", "*=", "~=", "|=":
			return strings.Contains(s.Value, t.Value)
		}
	case "~=":
		switch s.Op {
		case "~=":
			return s.Value == t.Value
		case "=":
			return t.Value != "" && containsToken(s.Value, t.Value)
		}
	case "|=":
		switch s.Op {
		case "|=":
			return s.Value == t.Value
		case "=":
			return s.Value == t.Value || strings.HasPrefix(s.Value, t.Value+"-")
		}
	}
	return false
}

func containsToken(value, tok string) bool {
	for _, f := range strings.Fields(value) {
		if f == tok {
			return true
		}
	}
	return false
}
