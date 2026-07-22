package sel_test

import (
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/sel"
)

// cn builds a ComponentNode with the given selector part and children.
func cn(part *comps.SelectorPart, children ...*comps.ComponentNode) *comps.ComponentNode {
	return &comps.ComponentNode{Selector: part, Children: children}
}

// mustLastPart parses a selector and returns its final (target) part.
func mustLastPart(t *testing.T, s string) *comps.SelectorPart {
	t.Helper()
	ps, err := sel.ParseSightmapSelector(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	if len(ps.Parts) == 0 {
		t.Fatalf("parse %q: zero parts", s)
	}
	return ps.Parts[len(ps.Parts)-1]
}

// ── Parsing ──────────────────────────────────────────────────────────────────

func TestParse_Has_Descendant(t *testing.T) {
	part := mustLastPart(t, "div:has(input)")
	if part.Tag != "div" {
		t.Fatalf("expected tag div, got %q", part.Tag)
	}
	if len(part.Has) != 1 {
		t.Fatalf("expected 1 :has entry, got %d", len(part.Has))
	}
	alts := part.Has[0].Alternatives
	if len(alts) != 1 {
		t.Fatalf("expected 1 alternative, got %d", len(alts))
	}
	if got := alts[0].Combinators[0]; got != " " {
		t.Errorf("expected descendant combinator %q, got %q", " ", got)
	}
	if alts[0].Parts[0].Tag != "input" {
		t.Errorf("expected inner tag input, got %q", alts[0].Parts[0].Tag)
	}
}

func TestParse_Has_DirectChild(t *testing.T) {
	part := mustLastPart(t, "div:has(> input)")
	if got := part.Has[0].Alternatives[0].Combinators[0]; got != ">" {
		t.Errorf("expected child combinator >, got %q", got)
	}
}

func TestParse_Has_Chain(t *testing.T) {
	part := mustLastPart(t, "div:has(.row .price)")
	alt := part.Has[0].Alternatives[0]
	if len(alt.Parts) != 2 {
		t.Fatalf("expected 2 chain parts, got %d", len(alt.Parts))
	}
	if alt.Combinators[0] != " " || alt.Combinators[1] != " " {
		t.Errorf("expected descendant combinators, got %v", alt.Combinators)
	}
}

func TestParse_Has_Multiple_AreSeparateEntries(t *testing.T) {
	part := mustLastPart(t, "div:has(input):has(.price)")
	if len(part.Has) != 2 {
		t.Fatalf("expected 2 :has entries (AND), got %d", len(part.Has))
	}
}

func TestParse_Has_CommaAlternatives(t *testing.T) {
	part := mustLastPart(t, "div:has(button, input)")
	if len(part.Has) != 1 {
		t.Fatalf("expected 1 :has entry, got %d", len(part.Has))
	}
	if len(part.Has[0].Alternatives) != 2 {
		t.Fatalf("expected 2 alternatives (OR), got %d", len(part.Has[0].Alternatives))
	}
}

func TestParse_Has_AttrInside(t *testing.T) {
	part := mustLastPart(t, `[data-testid="form-group"]:has(input[type="checkbox"])`)
	inner := part.Has[0].Alternatives[0].Parts[0]
	if inner.Tag != "input" || inner.Attrs["type"] != "checkbox" {
		t.Errorf("unexpected inner selector: %+v", inner)
	}
}

func TestParse_Has_Errors(t *testing.T) {
	cases := []struct {
		sel, wantSubstr string
	}{
		{"div:has(+ span)", "sibling combinator"},
		{"div:has(span ~ a)", "sibling combinator"},
		{"div:has()", ":has() argument"},
		{"div:has(span", "expected ')'"},
		{"div:zzz(x)", "unsupported pseudo-class"},
	}
	for _, tc := range cases {
		_, err := sel.ParseSightmapSelector(tc.sel)
		if err == nil {
			t.Errorf("%q: expected error, got nil", tc.sel)
			continue
		}
		if !strings.Contains(err.Error(), tc.wantSubstr) {
			t.Errorf("%q: error %q does not contain %q", tc.sel, err.Error(), tc.wantSubstr)
		}
	}
}

// ── Matching ─────────────────────────────────────────────────────────────────

func TestMatchesNode_Has_Descendant(t *testing.T) {
	// div > span > input
	tree := cn(nodeWith("div", "", nil, nil),
		cn(nodeWith("span", "", nil, nil),
			cn(nodeWith("input", "", nil, nil))))

	if !sel.MatchesNode(tree, mustLastPart(t, "div:has(input)")) {
		t.Error("div:has(input) should match a div with a nested input")
	}
	if sel.MatchesNode(tree, mustLastPart(t, "div:has(button)")) {
		t.Error("div:has(button) should not match (no button)")
	}
}

func TestMatchesNode_Has_DirectChild(t *testing.T) {
	// div > span > input   (input is a grandchild, not a direct child)
	deep := cn(nodeWith("div", "", nil, nil),
		cn(nodeWith("span", "", nil, nil),
			cn(nodeWith("input", "", nil, nil))))
	if sel.MatchesNode(deep, mustLastPart(t, "div:has(> input)")) {
		t.Error("div:has(> input) should NOT match a grandchild input")
	}

	// div > input   (direct child)
	shallow := cn(nodeWith("div", "", nil, nil),
		cn(nodeWith("input", "", nil, nil)))
	if !sel.MatchesNode(shallow, mustLastPart(t, "div:has(> input)")) {
		t.Error("div:has(> input) should match a direct-child input")
	}
}

func TestMatchesNode_Has_Chain(t *testing.T) {
	// div > .row > .price
	tree := cn(nodeWith("div", "", nil, nil),
		cn(nodeWith("", "", []string{"row"}, nil),
			cn(nodeWith("", "", []string{"price"}, nil))))
	if !sel.MatchesNode(tree, mustLastPart(t, "div:has(.row .price)")) {
		t.Error("div:has(.row .price) should match")
	}
	if sel.MatchesNode(tree, mustLastPart(t, "div:has(.price .row)")) {
		t.Error("div:has(.price .row) should NOT match (wrong order)")
	}
}

func TestMatchesNode_Has_MultipleAreAnded(t *testing.T) {
	both := cn(nodeWith("div", "", nil, nil),
		cn(nodeWith("input", "", nil, nil)),
		cn(nodeWith("", "", []string{"price"}, nil)))
	if !sel.MatchesNode(both, mustLastPart(t, "div:has(input):has(.price)")) {
		t.Error("both :has present should match")
	}

	onlyInput := cn(nodeWith("div", "", nil, nil),
		cn(nodeWith("input", "", nil, nil)))
	if sel.MatchesNode(onlyInput, mustLastPart(t, "div:has(input):has(.price)")) {
		t.Error("missing one :has should NOT match (AND semantics)")
	}
}

func TestMatchesNode_Has_CommaAlternativesAreOred(t *testing.T) {
	tree := cn(nodeWith("div", "", nil, nil),
		cn(nodeWith("input", "", nil, nil)))
	if !sel.MatchesNode(tree, mustLastPart(t, "div:has(button, input)")) {
		t.Error("div:has(button, input) should match when input present (OR)")
	}
	if sel.MatchesNode(tree, mustLastPart(t, "div:has(button, a)")) {
		t.Error("div:has(button, a) should NOT match (neither present)")
	}
}

func TestMatchesNode_Has_Nested(t *testing.T) {
	// div > .row > input
	tree := cn(nodeWith("div", "", nil, nil),
		cn(nodeWith("", "", []string{"row"}, nil),
			cn(nodeWith("input", "", nil, nil))))
	if !sel.MatchesNode(tree, mustLastPart(t, "div:has(.row:has(input))")) {
		t.Error("nested :has should match")
	}
	if sel.MatchesNode(tree, mustLastPart(t, "div:has(.row:has(button))")) {
		t.Error("nested :has should NOT match when inner missing")
	}
}

func TestMatchesNode_Has_RealFormGroup(t *testing.T) {
	// The HD assembly case: only the form-group containing a checkbox should match.
	fg := func(control *comps.ComponentNode) *comps.ComponentNode {
		return cn(nodeWith("div", "", nil, map[string]string{"data-testid": "form-group"}),
			cn(nodeWith("div", "", nil, nil), control))
	}
	assembly := fg(cn(nodeWith("input", "", nil, map[string]string{"type": "checkbox"})))
	protection := fg(cn(nodeWith("input", "", nil, map[string]string{"type": "radio"})))

	rule := mustLastPart(t, `[data-testid="form-group"]:has(input[type="checkbox"])`)
	if !sel.MatchesNode(assembly, rule) {
		t.Error("assembly form-group (with checkbox) should match")
	}
	if sel.MatchesNode(protection, rule) {
		t.Error("protection form-group (radio only) should NOT match")
	}
}
