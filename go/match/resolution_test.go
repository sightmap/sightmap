package match_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// n builds a tree node from a compact selector-like spec, "tag.class#id[a=v]",
// with children. Only the forms the tests use are supported.
func n(spec string, children ...*sightmap.ComponentNode) *sightmap.ComponentNode {
	el := &sightmap.Element{}
	rest := spec
	if i := strings.IndexAny(rest, ".#["); i >= 0 {
		el.Tag, rest = rest[:i], rest[i:]
	} else {
		el.Tag, rest = rest, ""
	}
	for rest != "" {
		switch rest[0] {
		case '.', '#':
			j := strings.IndexAny(rest[1:], ".#[")
			tok := rest[1:]
			if j >= 0 {
				tok = rest[1 : j+1]
			}
			if rest[0] == '.' {
				el.Classes = append(el.Classes, tok)
			} else {
				el.Id = tok
			}
			rest = rest[1+len(tok):]
		case '[':
			end := strings.IndexByte(rest, ']')
			k, v, _ := strings.Cut(rest[1:end], "=")
			if el.Attrs == nil {
				el.Attrs = map[string]string{}
			}
			el.Attrs[k] = v
			rest = rest[end+1:]
		}
	}
	return &sightmap.ComponentNode{Element: el, Children: children}
}

func loadCorpus(t *testing.T, yaml string) *sightmap.Corpus {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte("version: 1\n"+yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// find returns the first node in root whose element has class cls.
func find(root *sightmap.ComponentNode, cls string) *sightmap.ComponentNode {
	if root.Element != nil {
		for _, c := range root.Element.Classes {
			if c == cls {
				return root
			}
		}
	}
	for _, c := range root.Children {
		if f := find(c, cls); f != nil {
			return f
		}
	}
	return nil
}

type resolutionCase struct {
	name   string
	yaml   string
	tree   *sightmap.ComponentNode
	url    string
	target string // class of the node to check
	want   string // effective privacy
}

// privacyCases are SEP-0009's resolution rules: every matching component
// declares, whichever component names the element; the strictest declaration
// on one element wins; the nearest wins down the tree; and block is absolute.
var privacyCases = []resolutionCase{
	{
		name: "a child naming every input in a form does not discard a block",
		yaml: `components:
  - name: CheckoutForm
    selector: 'form.checkout'
    children:
      - name: Field
        selector: 'input'
  - name: CardNumber
    selector: 'input.cc'
    privacy: block
`,
		tree: n("body", n("form.checkout", n("input.email"), n("div", n("input.cc")))), target: "cc", want: "block",
	},
	{
		name: "a child naming every child of a card does not discard a block",
		yaml: `components:
  - name: Card
    selector: '.card'
    children:
      - name: CardPart
        selector: '> *'
  - name: CardNumber
    selector: 'input.cc'
    privacy: block
`,
		tree: n("body", n("div.card", n("input.cc"))), target: "cc", want: "block",
	},
	{
		name: "a descendant selector naming the node does not discard a block",
		yaml: `components:
  - name: Any
    selector: 'body *'
  - name: CardNumber
    selector: 'input.cc'
    privacy: block
`,
		tree: n("body", n("input.cc")), target: "cc", want: "block",
	},
	{
		name: "a view component naming the node does not discard a global block",
		yaml: `components:
  - name: CardNumber
    selector: 'input.cc'
    privacy: block
views:
  - name: Checkout
    route: /checkout
    components:
      - name: Field
        selector: 'input'
`,
		tree: n("body", n("input.cc")), url: "/checkout", target: "cc", want: "block",
	},
	{
		name: "a view component sharing a global's name does not remove its privacy",
		yaml: `components:
  - name: CardNumber
    selector: 'input.cc'
    privacy: block
  - name: Input
    selector: 'input'
views:
  - name: Checkout
    route: /checkout
    components:
      - name: CardNumber
        selector: '.order-summary .card-last4'
`,
		tree: n("body", n("input.cc")), url: "/checkout", target: "cc", want: "block",
	},
	{
		name: "block is absolute: an unmask inside it has no effect",
		yaml: `components:
  - name: Payment
    selector: '.payment'
    privacy: block
  - name: Total
    selector: '.payment .total'
    privacy: unmask
`,
		tree: n("body", n("section.payment", n("p.total"))), target: "total", want: "block",
	},
	{
		name: "the strictest declaration on an element wins over the first (unmask, mask)",
		yaml: `components:
  - name: Public
    selector: '[data-public]'
    privacy: unmask
  - name: Address
    selector: '.address'
    privacy: mask
`,
		tree: n("body", n("div.address[data-public=1]")), target: "address", want: "mask",
	},
	{
		name: "the strictest declaration on an element wins over the first (mask, unmask)",
		yaml: `components:
  - name: Address
    selector: '.address'
    privacy: mask
  - name: Public
    selector: '[data-public]'
    privacy: unmask
`,
		tree: n("body", n("div.address[data-public=1]")), target: "address", want: "mask",
	},
	{
		name: "the strictest declaration on an element wins over the first (mask, block)",
		yaml: `components:
  - name: Address
    selector: '.address'
    privacy: mask
  - name: Sensitive
    selector: '[data-sensitive]'
    privacy: block
`,
		tree: n("body", n("div.address[data-sensitive=1]")), target: "address", want: "block",
	},
	{
		name: "the strictest declaration on an element wins over the first (block, mask)",
		yaml: `components:
  - name: Sensitive
    selector: '[data-sensitive]'
    privacy: block
  - name: Address
    selector: '.address'
    privacy: mask
`,
		tree: n("body", n("div.address[data-sensitive=1]")), target: "address", want: "block",
	},
	{
		name: "an unmask reopens an enclosing mask",
		yaml: `components:
  - name: Profile
    selector: 'form.profile'
    privacy: mask
  - name: DisplayName
    selector: '.display-name'
    privacy: unmask
`,
		tree: n("body", n("form.profile", n("p.display-name"))), target: "display-name", want: "unmask",
	},
	{
		name: "the nearest declaration wins down the tree",
		yaml: `components:
  - name: Profile
    selector: 'form.profile'
    privacy: mask
  - name: PublicSection
    selector: '.public'
    privacy: unmask
  - name: Phone
    selector: '.phone'
    privacy: mask
`,
		tree: n("body", n("form.profile", n("div.public", n("p.phone")))), target: "phone", want: "mask",
	},
	{
		name: "an unrecognized value is a block, and an unmask inside cannot reopen it",
		yaml: `components:
  - name: Vault
    selector: '.vault'
    privacy: secret
  - name: Label
    selector: '.vault .label'
    privacy: unmask
`,
		tree: n("body", n("div.vault", n("p.label"))), target: "label", want: "block",
	},
}

func TestMatch_PrivacyResolution(t *testing.T) {
	for _, c := range privacyCases {
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

// TestMatch_WatchResolution is SEP-0015's rule: a watched component is
// reported wherever its selector matches, whichever component names the
// element.
func TestMatch_WatchResolution(t *testing.T) {
	m := match.NewMatcher(loadCorpus(t, `components:
  - name: CheckoutForm
    selector: 'form.checkout'
    children:
      - name: FormMessage
        selector: '.message'
  - name: PaymentDeclined
    selector: '.message.declined'
    watch: true
`))
	tree := n("body", n("form.checkout", n("div.message.declined"), n("div.message.info")))
	got := m.Match(tree, "")
	if cm := got[find(tree, "declined")]; cm == nil || !cm.Watch {
		t.Errorf("the declined banner is named %v, and must be watched", cm)
	}
	if cm := got[find(tree, "info")]; cm == nil || cm.Watch {
		t.Errorf("an info message matches no watched component and must not be watched: %v", cm)
	}
}
