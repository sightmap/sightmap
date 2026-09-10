package match_test

import (
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// TestNot_HasLeafScoping is the load-bearing X:not(:has(Y)) leaf-scoping pattern
// (JB FormField: 'jb-form-field-container:not(:has(jb-form-field-container))').
// Only the leaf container — the one with no nested container of its own — should
// match. Regresses the bug where :not(:has()) was evaluated by the flat matcher,
// which ignored the inner :has() and so excluded EVERY container (0 offline).
func TestNot_HasLeafScoping(t *testing.T) {
	inner := node("inner", "jb-form-field-container", nil, node("lbl", "label", nil))
	outer := node("outer", "jb-form-field-container", nil, inner)
	root := node("root", "div", nil, outer)

	defs := []sightmap.ComponentDef{{
		Name:      "FormField",
		Selectors: []string{`jb-form-field-container:not(:has(jb-form-field-container))`},
	}}
	assertIDs(t, applyDefs(t, root, defs), "FormField", "inner")
}

// TestNot_DescendantCombinator covers a descendant combinator inside :not() (an
// ancestor constraint on the subject), the JB CheckoutNextButton case
// 'jb-checkout button.jb-button-primary:not(jb-sign-in button)'. The primary
// button nested under jb-sign-in must be excluded; the sibling step CTA (same
// class, not under jb-sign-in) must match. Regresses both a parse error and the
// need for ancestor context to evaluate it.
func TestNot_DescendantCombinator(t *testing.T) {
	login := node("login", "button", []string{"jb-button-primary"})
	signIn := node("signin", "jb-sign-in", nil, login)
	step := node("step", "button", []string{"jb-button-primary"})
	root := node("root", "div", nil, node("co", "jb-checkout", nil, signIn, step))

	defs := []sightmap.ComponentDef{{
		Name:      "CheckoutNextButton",
		Selectors: []string{`jb-checkout button.jb-button-primary:not(jb-sign-in button)`},
	}}
	assertIDs(t, applyDefs(t, root, defs), "CheckoutNextButton", "step")
}

// TestNot_DirectChildCombinator checks the > combinator inside :not(): only a
// button that is a DIRECT child of a toolbar is excluded; a more deeply nested
// one is not.
func TestNot_DirectChildCombinator(t *testing.T) {
	directChild := node("direct", "button", nil)
	nested := node("nested", "button", nil)
	root := node("root", "div", nil,
		node("tb", "toolbar", nil, directChild, node("wrap", "span", nil, nested)),
	)
	defs := []sightmap.ComponentDef{{
		Name:      "FreeButton",
		Selectors: []string{`button:not(toolbar > button)`},
	}}
	// directChild is a direct toolbar child → excluded; nested is a grandchild → kept.
	assertIDs(t, applyDefs(t, root, defs), "FreeButton", "nested")
}

// TestIs_HasNodeAware verifies an alternative inside :is() carrying a :has() is
// evaluated with tree context (same class of bug as :not(:has())).
func TestIs_HasNodeAware(t *testing.T) {
	withChild := node("withkid", "section", []string{"card"}, node("k", "img", nil))
	without := node("nokid", "section", []string{"card"})
	root := node("root", "div", nil, withChild, without)
	defs := []sightmap.ComponentDef{{
		Name:      "MediaCard",
		Selectors: []string{`section:is(.card:has(img))`},
	}}
	assertIDs(t, applyDefs(t, root, defs), "MediaCard", "withkid")
}
