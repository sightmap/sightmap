package match_test

import (
	"testing"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/match"
)

// TestHas_FormGroupScoping is the end-to-end (snapshot/coverage) path for the
// :has() relational pseudo-class: only the form-group that contains a checkbox
// should match, mirroring the homedepot assembly-vs-protection rows.
func TestHas_FormGroupScoping(t *testing.T) {
	formGroup := func(id string, control *comps.ComponentNode) *comps.ComponentNode {
		return nodeAttr(id, "div", map[string]string{"data-testid": "form-group"},
			node("", "div", nil, control))
	}
	root := node("root", "div", nil,
		formGroup("assembly", nodeAttr("cb", "input", map[string]string{"type": "checkbox"})),
		formGroup("protection", nodeAttr("rb", "input", map[string]string{"type": "radio"})),
	)

	defs := []match.SightmapComponent{{
		Name:      "AssemblyOption",
		Selectors: []string{`[data-testid="form-group"]:has(input[type="checkbox"])`},
	}}

	got := applyDefs(t, root, defs)
	assertIDs(t, got, "AssemblyOption", "assembly")
}

// TestHas_DirectChildScoping verifies the > combinator inside :has() through the
// full matcher.
func TestHas_DirectChildScoping(t *testing.T) {
	// container with a direct-child button vs one with only a nested button.
	root := node("root", "div", nil,
		node("hasDirect", "div", []string{"box"}, node("b1", "button", nil)),
		node("hasNested", "div", []string{"box"}, node("", "span", nil, node("b2", "button", nil))),
	)
	defs := []match.SightmapComponent{{
		Name:      "BoxWithButton",
		Selectors: []string{"div.box:has(> button)"},
	}}
	got := applyDefs(t, root, defs)
	assertIDs(t, got, "BoxWithButton", "hasDirect")
}
