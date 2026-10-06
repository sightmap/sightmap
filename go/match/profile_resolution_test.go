package match_test

import (
	"reflect"
	"testing"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
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
			if got := m.Privacy(c.tree, c.url)[node]; got != c.want {
				t.Errorf("Privacy = %q, want %q", got, c.want)
			}
			if got := m.PrivacyForChain(elementsTo(c.tree, node), c.url); got[len(got)-1] != c.want {
				t.Errorf("PrivacyForChain = %q, want %q at the leaf", got, c.want)
			}
		})
	}
}

// pathTo returns the nodes from root to target, inclusive.
func pathTo(root, target *sightmap.ComponentNode) []*sightmap.ComponentNode {
	if root == target {
		return []*sightmap.ComponentNode{root}
	}
	for _, c := range root.Children {
		if p := pathTo(c, target); p != nil {
			return append([]*sightmap.ComponentNode{root}, p...)
		}
	}
	return nil
}

func elementsTo(root, target *sightmap.ComponentNode) []sightmap.Element {
	var out []sightmap.Element
	for _, n := range pathTo(root, target) {
		out = append(out, *n.Element)
	}
	return out
}

func TestPrivacy_ConsumerRules(t *testing.T) {
	corpus := loadCorpus(t, `components:
  - name: Ok
    selector: '.ok'
    privacy: unmask
  - name: Other
    selector: 'p.other'
`)
	tree := n("body", n("div.no-capture", n("p.ok")), n("p.other"))
	ok, other := find(tree, "ok"), find(tree, "other")
	for _, c := range []struct {
		name      string
		rule      match.PrivacyRule
		ok, other string
	}{
		{"a consumer block applies, and a corpus unmask cannot reopen it", match.PrivacyRule{Selector: ".no-capture", Privacy: "block"}, "block", ""},
		{"a consumer unmask is ignored", match.PrivacyRule{Selector: "p.other", Privacy: "unmask"}, "unmask", ""},
		{"a consumer mask outside the profile covers the document", match.PrivacyRule{Selector: "p:hover", Privacy: "mask"}, "mask", "mask"},
	} {
		got := match.NewMatcher(corpus, match.WithPrivacyRules(c.rule)).Privacy(tree, "")
		if got[ok] != c.ok || got[other] != c.other {
			t.Errorf("%s: ok=%q other=%q, want %q and %q", c.name, got[ok], got[other], c.ok, c.other)
		}
	}
}

func TestPrivacyAttributes(t *testing.T) {
	m := match.NewMatcher(loadCorpus(t, `components:
  - name: Card
    selector: 'form[data-form] input[name="cc"]:not([type=hidden])'
    privacy: block
  - name: Banner
    selector: '[data-banner]'
    watch: true
  - name: Named
    selector: '[data-ignored]'
`))
	if got, want := m.PrivacyAttributes(""), []string{"data-banner", "data-form", "name", "type"}; !reflect.DeepEqual(got, want) {
		t.Errorf("PrivacyAttributes = %v, want %v", got, want)
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
