package sel_test

import (
	"testing"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/sel"
)

// parseOK parses s and fails t if there's an error.
func parseOK(t *testing.T, s string) sel.ParsedSelector {
	t.Helper()
	ps, err := sel.ParseSightmapSelector(s)
	if err != nil {
		t.Fatalf("ParseSightmapSelector(%q) error: %v", s, err)
	}
	return ps
}

// parseErr asserts that parsing s returns an error.
func parseErr(t *testing.T, s string) {
	t.Helper()
	_, err := sel.ParseSightmapSelector(s)
	if err == nil {
		t.Fatalf("ParseSightmapSelector(%q) expected error, got nil", s)
	}
}

func TestParse_Tag(t *testing.T) {
	ps := parseOK(t, "div")
	if len(ps.Parts) != 1 {
		t.Fatalf("parts: got %d, want 1", len(ps.Parts))
	}
	if ps.Parts[0].Tag != "div" {
		t.Errorf("tag: got %q, want %q", ps.Parts[0].Tag, "div")
	}
}

func TestParse_TagCaseNormalized(t *testing.T) {
	ps := parseOK(t, "BUTTON")
	if ps.Parts[0].Tag != "button" {
		t.Errorf("tag: got %q, want %q", ps.Parts[0].Tag, "button")
	}
}

func TestParse_SyntheticMobileTag(t *testing.T) {
	// Mobile tags like UIButton are preserved (lowercased by the parser,
	// but matched case-insensitively by sel.Matches).
	ps := parseOK(t, "UIButton")
	if ps.Parts[0].Tag != "uibutton" {
		t.Errorf("tag: got %q, want %q", ps.Parts[0].Tag, "uibutton")
	}
}

func TestParse_ID(t *testing.T) {
	ps := parseOK(t, "#main-nav")
	if ps.Parts[0].Id != "main-nav" {
		t.Errorf("id: got %q, want %q", ps.Parts[0].Id, "main-nav")
	}
}

func TestParse_Classes(t *testing.T) {
	ps := parseOK(t, ".product-card.featured")
	if len(ps.Parts[0].Classes) != 2 {
		t.Fatalf("classes: got %v, want 2", ps.Parts[0].Classes)
	}
	if ps.Parts[0].Classes[0] != "product-card" || ps.Parts[0].Classes[1] != "featured" {
		t.Errorf("classes: got %v, want [product-card featured]", ps.Parts[0].Classes)
	}
}

func TestParse_TagWithClasses(t *testing.T) {
	ps := parseOK(t, "nav.main-nav")
	if ps.Parts[0].Tag != "nav" {
		t.Errorf("tag: got %q, want nav", ps.Parts[0].Tag)
	}
	if len(ps.Parts[0].Classes) != 1 || ps.Parts[0].Classes[0] != "main-nav" {
		t.Errorf("classes: got %v, want [main-nav]", ps.Parts[0].Classes)
	}
}

func TestParse_AttrPresence(t *testing.T) {
	ps := parseOK(t, "[disabled]")
	part := ps.Parts[0]
	if part.Attrs["disabled"] != "" {
		t.Errorf("attrs[disabled]: got %q, want empty", part.Attrs["disabled"])
	}
	if part.AttrOps["disabled"] != "[]" {
		t.Errorf("attrOps[disabled]: got %q, want []", part.AttrOps["disabled"])
	}
}

func TestParse_AttrExact(t *testing.T) {
	ps := parseOK(t, `div[data-testid="add-to-cart"]`)
	part := ps.Parts[0]
	if part.Tag != "div" {
		t.Errorf("tag: got %q, want div", part.Tag)
	}
	if part.Attrs["data-testid"] != "add-to-cart" {
		t.Errorf("attr: got %q, want add-to-cart", part.Attrs["data-testid"])
	}
	if op := part.AttrOps["data-testid"]; op != "" {
		t.Errorf("attrOp for = should be absent, got %q", op)
	}
}

func TestParse_AttrSingleQuote(t *testing.T) {
	ps := parseOK(t, "[data-testid='add']")
	if ps.Parts[0].Attrs["data-testid"] != "add" {
		t.Errorf("attr: got %q, want add", ps.Parts[0].Attrs["data-testid"])
	}
}

func TestParse_AttrUnquoted(t *testing.T) {
	ps := parseOK(t, "[type=email]")
	if ps.Parts[0].Attrs["type"] != "email" {
		t.Errorf("attr: got %q, want email", ps.Parts[0].Attrs["type"])
	}
}

func TestParse_AttrOperators(t *testing.T) {
	cases := []struct {
		sel string
		op  string
		val string
	}{
		{`[aria-label*="Search"]`, "*=", "Search"},
		{`[href^="https://"]`, "^=", "https://"},
		{`[class$="active"]`, "$=", "active"},
		{`[class~="btn"]`, "~=", "btn"},
		{`[lang|="en"]`, "|=", "en"},
	}
	for _, tc := range cases {
		t.Run(tc.sel, func(t *testing.T) {
			ps := parseOK(t, tc.sel)
			part := ps.Parts[0]
			// find the key
			var key, gotOp, gotVal string
			for k, v := range part.Attrs {
				key = k
				gotVal = v
			}
			if part.AttrOps != nil {
				gotOp = part.AttrOps[key]
			}
			if gotOp != tc.op {
				t.Errorf("op: got %q, want %q", gotOp, tc.op)
			}
			if gotVal != tc.val {
				t.Errorf("val: got %q, want %q", gotVal, tc.val)
			}
		})
	}
}

func TestParse_NotPseudo(t *testing.T) {
	ps := parseOK(t, "button:not(.disabled)")
	part := ps.Parts[0]
	if part.Tag != "button" {
		t.Errorf("tag: got %q, want button", part.Tag)
	}
	if part.Not == nil {
		t.Fatal("Not is nil")
	}
	if len(part.Not.Classes) != 1 || part.Not.Classes[0] != "disabled" {
		t.Errorf("Not.Classes: got %v, want [disabled]", part.Not.Classes)
	}
}

func TestParse_NotWithAttr(t *testing.T) {
	ps := parseOK(t, `div:not([hidden])`)
	if ps.Parts[0].Not == nil {
		t.Fatal("Not is nil")
	}
	if ps.Parts[0].Not.AttrOps["hidden"] != "[]" {
		t.Errorf("Not attr presence: got %q", ps.Parts[0].Not.AttrOps["hidden"])
	}
}

func TestParse_DescendantCombinator(t *testing.T) {
	ps := parseOK(t, "nav.main-nav a.nav-link")
	if len(ps.Parts) != 2 {
		t.Fatalf("parts: got %d, want 2", len(ps.Parts))
	}
	if ps.Combinators[0] != "" {
		t.Errorf("combinators[0]: got %q, want empty", ps.Combinators[0])
	}
	if ps.Combinators[1] != " " {
		t.Errorf("combinators[1]: got %q, want space (descendant)", ps.Combinators[1])
	}
	if ps.Parts[0].Tag != "nav" || ps.Parts[0].Classes[0] != "main-nav" {
		t.Errorf("part[0]: got %+v", ps.Parts[0])
	}
	if ps.Parts[1].Tag != "a" || ps.Parts[1].Classes[0] != "nav-link" {
		t.Errorf("part[1]: got %+v", ps.Parts[1])
	}
}

func TestParse_DirectChildCombinator(t *testing.T) {
	ps := parseOK(t, "div.foo > button.bar")
	if len(ps.Parts) != 2 {
		t.Fatalf("parts: got %d, want 2", len(ps.Parts))
	}
	if ps.Combinators[1] != ">" {
		t.Errorf("combinator: got %q, want >", ps.Combinators[1])
	}
}

func TestParse_MobileSelector(t *testing.T) {
	ps := parseOK(t, `UIButton[label="Add to Cart"]`)
	part := ps.Parts[0]
	if part.Tag != "uibutton" {
		t.Errorf("tag: got %q, want uibutton", part.Tag)
	}
	if part.Attrs["label"] != "Add to Cart" {
		t.Errorf("attr: got %q, want Add to Cart", part.Attrs["label"])
	}
}

func TestParse_MultiPart(t *testing.T) {
	// Three-part descendant chain
	ps := parseOK(t, "form.search > div.field input[type=email]")
	if len(ps.Parts) != 3 {
		t.Fatalf("parts: got %d, want 3", len(ps.Parts))
	}
	if ps.Combinators[1] != ">" {
		t.Errorf("combinators[1]: got %q, want >", ps.Combinators[1])
	}
	if ps.Combinators[2] != " " {
		t.Errorf("combinators[2]: got %q, want space", ps.Combinators[2])
	}
}

func TestParse_Errors(t *testing.T) {
	cases := []string{
		"",              // empty
		"div + span",    // unsupported combinator (+)
		"div ~ span",    // unsupported combinator (~)
		":nth-child(2)", // unsupported pseudo
		":hover",        // unsupported pseudo
		"[attr",         // unclosed bracket
		"div:not(",      // unclosed :not
	}
	for _, s := range cases {
		t.Run(s, func(t *testing.T) {
			parseErr(t, s)
		})
	}
}

func TestParse_IsPseudo(t *testing.T) {
	ps := parseOK(t, `:is([data-testid="main-content"], [data-testid="top-content"]) [data-testid="foo"]`)
	if len(ps.Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(ps.Parts))
	}
	part0 := ps.Parts[0]
	if len(part0.Is) != 2 {
		t.Fatalf("expected 2 Is alternatives, got %d", len(part0.Is))
	}
	if part0.Is[0].Attrs["data-testid"] != "main-content" {
		t.Errorf("Is[0]: got %+v", part0.Is[0])
	}
	if part0.Is[1].Attrs["data-testid"] != "top-content" {
		t.Errorf("Is[1]: got %+v", part0.Is[1])
	}
	// descendant combinator
	if ps.Combinators[1] != " " {
		t.Errorf("combinator: got %q, want space", ps.Combinators[1])
	}
}

func TestParse_WherePseudo(t *testing.T) {
	// :where() is identical semantics to :is()
	ps := parseOK(t, `:where(.foo, .bar)`)
	if len(ps.Parts[0].Is) != 2 {
		t.Fatalf("expected 2 Is alternatives for :where(), got %d", len(ps.Parts[0].Is))
	}
}

func TestParse_HexEscapeBasic(t *testing.T) {
	// \32 is the CSS hex escape for '2' (digit-leading names need this).
	// The trailing space after the hex digits is mandatory and must be consumed.
	ps := parseOK(t, `.\32 xl-foo`)
	part := ps.Parts[0]
	if len(part.Classes) != 1 {
		t.Fatalf("classes: got %v, want 1", part.Classes)
	}
	if part.Classes[0] != "2xl-foo" {
		t.Errorf("class: got %q, want \"2xl-foo\"", part.Classes[0])
	}
}

func TestParse_HexEscapeInSelector(t *testing.T) {
	// Full selector as probe.js would generate for a div with a 2xl: class.
	// div.\32 xl\:sui-col-span-12[data-testid="layout-section"]
	ps := parseOK(t, `div.\32 xl\:sui-col-span-12[data-testid="layout-section"]`)
	part := ps.Parts[0]
	if part.Tag != "div" {
		t.Errorf("tag: got %q, want \"div\"", part.Tag)
	}
	if len(part.Classes) != 1 {
		t.Fatalf("classes: got %v, want 1", part.Classes)
	}
	if part.Classes[0] != "2xl:sui-col-span-12" {
		t.Errorf("class: got %q, want \"2xl:sui-col-span-12\"", part.Classes[0])
	}
	if part.Attrs["data-testid"] != "layout-section" {
		t.Errorf("attrs[data-testid]: got %q, want \"layout-section\"", part.Attrs["data-testid"])
	}
}

func TestParse_HexEscapeNoRegression(t *testing.T) {
	// \: simple escape (go-0032 regression check) still works alongside hex escapes.
	ps := parseOK(t, `a.hover\:text-blue`)
	if ps.Parts[0].Tag != "a" {
		t.Errorf("tag: got %q, want \"a\"", ps.Parts[0].Tag)
	}
	if len(ps.Parts[0].Classes) != 1 || ps.Parts[0].Classes[0] != "hover:text-blue" {
		t.Errorf("class: got %v", ps.Parts[0].Classes)
	}
}

func TestParse_TailwindEscapedClass(t *testing.T) {
	// Tailwind variant classes use escaped colons (e.g. hover\:text-blue).
	// parseIdentifier must consume the escape mid-identifier so the tag
	// is never overwritten.
	ps := parseOK(t, `a.hover\:sui-text-primary`)
	part := ps.Parts[0]
	if part.Tag != "a" {
		t.Errorf("tag: got %q, want \"a\" (Tailwind escaped class must not overwrite tag)", part.Tag)
	}
	if len(part.Classes) != 1 {
		t.Fatalf("classes: got %v, want 1 entry", part.Classes)
	}
	if part.Classes[0] != "hover:sui-text-primary" {
		t.Errorf("class: got %q, want \"hover:sui-text-primary\"", part.Classes[0])
	}
}

func TestParse_TailwindEscapedClassWithAttr(t *testing.T) {
	// Selector from probe.js for a div with Tailwind classes and data-testid.
	ps := parseOK(t, `div.sui-col-span-12.md\:sui-col-span-12[data-testid="layout-section"]`)
	part := ps.Parts[0]
	if part.Tag != "div" {
		t.Errorf("tag: got %q, want \"div\"", part.Tag)
	}
	if len(part.Classes) != 2 {
		t.Fatalf("classes: got %v, want 2", part.Classes)
	}
	if part.Classes[0] != "sui-col-span-12" {
		t.Errorf("classes[0]: got %q, want \"sui-col-span-12\"", part.Classes[0])
	}
	if part.Classes[1] != "md:sui-col-span-12" {
		t.Errorf("classes[1]: got %q, want \"md:sui-col-span-12\"", part.Classes[1])
	}
	if part.Attrs["data-testid"] != "layout-section" {
		t.Errorf("attrs[data-testid]: got %q, want \"layout-section\"", part.Attrs["data-testid"])
	}
}

func TestParse_TailwindMultipleEscapedClasses(t *testing.T) {
	// Multiple Tailwind variants on one element (focus-visible\: and hover\:).
	ps := parseOK(t, `a.focus-visible\:ring-2.hover\:underline`)
	part := ps.Parts[0]
	if part.Tag != "a" {
		t.Errorf("tag: got %q, want \"a\"", part.Tag)
	}
	if len(part.Classes) != 2 {
		t.Fatalf("classes: got %v, want 2", part.Classes)
	}
	if part.Classes[0] != "focus-visible:ring-2" {
		t.Errorf("classes[0]: got %q, want \"focus-visible:ring-2\"", part.Classes[0])
	}
	if part.Classes[1] != "hover:underline" {
		t.Errorf("classes[1]: got %q, want \"hover:underline\"", part.Classes[1])
	}
}

func TestParse_UniversalSelector(t *testing.T) {
	ps := parseOK(t, "*")
	// Universal selector produces an empty SelectorPart — matches everything.
	if len(ps.Parts) != 1 {
		t.Fatalf("parts: got %d, want 1", len(ps.Parts))
	}
	part := ps.Parts[0]
	if part.Tag != "" || part.Id != "" || len(part.Classes) != 0 {
		t.Errorf("universal selector should produce empty SelectorPart, got %+v", part)
	}
}

func TestParse_MultipleAttributes(t *testing.T) {
	ps := parseOK(t, `input[type="email"][required]`)
	part := ps.Parts[0]
	if part.Tag != "input" {
		t.Errorf("tag: got %q, want input", part.Tag)
	}
	if part.Attrs["type"] != "email" {
		t.Errorf("type attr: got %q, want email", part.Attrs["type"])
	}
	if part.AttrOps["required"] != "[]" {
		t.Errorf("required attrOp: got %q, want []", part.AttrOps["required"])
	}
}

// nodeWith builds a minimal SelectorPart for testing Matches.
func nodeWith(tag, id string, classes []string, attrs map[string]string) *comps.SelectorPart {
	return &comps.SelectorPart{
		Tag:     tag,
		Id:      id,
		Classes: classes,
		Attrs:   attrs,
	}
}
