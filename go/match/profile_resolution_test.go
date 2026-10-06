package match_test

import (
	"testing"

	"github.com/sightmap/sightmap/go/match"
)

// profileCases are SEP-0018's rules for privacy rules it rejects: a rule
// outside the capture-baseline profile, or whose subject matches nearly every
// element, never matches loosely. A block or mask covers the whole document;
// an unmask is ignored.
var profileCases = []resolutionCase{
	{
		name: "a block outside the profile covers the document",
		yaml: `components:
  - name: Secret
    selector: ':is(.a):is(.b)'
    privacy: block
  - name: Other
    selector: 'p.other'
`,
		tree: n("body", n("p.other")), target: "other", want: "block",
	},
	{
		name: "a block with two ids covers the document",
		yaml: `components:
  - name: Secret
    selector: '#a#b'
    privacy: block
  - name: Other
    selector: 'p.other'
`,
		tree: n("body", n("p.other")), target: "other", want: "block",
	},
	{
		name: "a block on an empty ~= value covers the document",
		yaml: `components:
  - name: Secret
    selector: '[a~=""]'
    privacy: block
  - name: Other
    selector: 'p.other'
`,
		tree: n("body", n("p.other")), target: "other", want: "block",
	},
	{
		name: "a block that depends on descendants covers the document",
		yaml: `components:
  - name: Payment
    selector: 'section:has(input[name=cardnumber])'
    privacy: block
  - name: Other
    selector: 'p.other'
`,
		tree: n("body", n("section", n("input[name=cardnumber]")), n("p.other")), target: "other", want: "block",
	},
	{
		name: "a child inheriting a parent's :has() covers the document",
		yaml: `components:
  - name: Card
    selector: '.card:has(.x)'
    children:
      - name: Cvc
        selector: 'input.cvc'
        privacy: block
  - name: Other
    selector: 'p.other'
`,
		tree: n("body", n("div.card", n("input.cvc")), n("p.other")), target: "other", want: "block",
	},
	{
		name: "one alternative outside the profile makes the mask cover the document",
		yaml: `components:
  - name: Hint
    selector: ['.ok', '.b:hover']
    privacy: mask
  - name: Other
    selector: 'p.other'
`,
		tree: n("body", n("p.other")), target: "other", want: "mask",
	},
	{
		name: "an unmask on '*' does not reopen a mask",
		yaml: `components:
  - name: Checkout
    selector: 'form.checkout'
    privacy: mask
  - name: Anything
    selector: '*'
    privacy: unmask
  - name: Field
    selector: 'input.f'
`,
		tree: n("body", n("form.checkout", n("input.f"))), target: "f", want: "mask",
	},
	{
		name: "an unmask on ':not(.x)' does not reopen a mask",
		yaml: `components:
  - name: Checkout
    selector: 'form.checkout'
    privacy: mask
  - name: NotX
    selector: ':not(.x)'
    privacy: unmask
  - name: Field
    selector: 'input.f'
`,
		tree: n("body", n("form.checkout", n("input.f"))), target: "f", want: "mask",
	},
	{
		name: "an unmask outside the profile is ignored",
		yaml: `components:
  - name: Checkout
    selector: 'form.checkout'
    privacy: mask
  - name: Hinted
    selector: 'input:is(.f)'
    privacy: unmask
`,
		tree: n("body", n("form.checkout", n("input.f"))), target: "f", want: "mask",
	},
}

func TestMatch_ProfileResolution(t *testing.T) {
	for _, c := range profileCases {
		t.Run(c.name, func(t *testing.T) {
			m := match.NewMatcher(loadCorpus(t, c.yaml))
			node := find(c.tree, c.target)
			cm := m.Match(c.tree, c.url)[node]
			if cm == nil {
				t.Fatalf("node .%s is not named by any component", c.target)
			}
			if cm.Privacy != c.want {
				t.Errorf("Match privacy = %q, want %q", cm.Privacy, c.want)
			}
		})
	}
}

// TestMatch_WatchOutsideProfileNotReported: SEP-0018 reports no watch rule the
// profile rejects.
func TestMatch_WatchOutsideProfileNotReported(t *testing.T) {
	m := match.NewMatcher(loadCorpus(t, `components:
  - name: Banner
    selector: '.banner'
  - name: FirstBanner
    selector: 'div:is(.banner)'
    watch: true
`))
	tree := n("body", n("div.banner"))
	if cm := m.Match(tree, "")[find(tree, "banner")]; cm == nil || cm.Watch {
		t.Errorf("a watch rule outside the profile must not be reported, got %v", cm)
	}
}
