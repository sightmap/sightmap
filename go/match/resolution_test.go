package match_test

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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
			if got := m.Privacy(c.tree, c.url)[node]; got != c.want {
				t.Errorf("Privacy = %q, want %q", got, c.want)
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
	if cm := got[find(tree, "declined")]; cm == nil || !reflect.DeepEqual(cm.Watched, []string{"PaymentDeclined"}) {
		t.Errorf("the declined banner must report PaymentDeclined, got %v", cm)
	}
}

func TestWatched_ViewSharingAGlobalName(t *testing.T) {
	m := match.NewMatcher(loadCorpus(t, `components:
  - name: PromoBanner
    selector: '.promo'
    watch: true
views:
  - name: Sale
    route: /sale
    components:
      - name: PromoBanner
        selector: '.sale-header .promo-title'
`))
	tree := n("body", n("div.promo"))
	if got := m.Watched(tree, "/sale")[find(tree, "promo")]; !reflect.DeepEqual(got, []string{"PromoBanner"}) {
		t.Errorf("the global PromoBanner must stay watched on /sale, got %v", got)
	}
}

func TestWatched_EveryWatchedComponentOnOneElement(t *testing.T) {
	m := match.NewMatcher(loadCorpus(t, `components:
  - name: OutOfStock
    selector: '.notice.out-of-stock'
    watch: true
  - name: Notice
    selector: '.notice'
    watch: true
`))
	tree := n("body", n("div.notice.out-of-stock"))
	node := find(tree, "notice")
	want := []string{"Notice", "OutOfStock"}
	if got := m.Watched(tree, "")[node]; !reflect.DeepEqual(got, want) {
		t.Errorf("Watched = %v, want %v", got, want)
	}
	if cm := m.Match(tree, "")[node]; cm == nil || !reflect.DeepEqual(cm.Watched, want) {
		t.Errorf("Match Watched = %v, want %v", cm, want)
	}
}

// TestWatched_DistinctComponentsSharingAName proves dedup is by definition, not
// name: two distinct watched components with the same name (names are unique
// only per parent) are both reported, while one component with several
// selectors is reported once.
func TestWatched_DistinctComponentsSharingAName(t *testing.T) {
	m := match.NewMatcher(loadCorpus(t, `components:
  - name: Checkout
    selector: 'form.checkout'
    children:
      - name: Banner
        selector: '.promo'
        watch: true
  - name: Account
    selector: 'section.account'
    children:
      - name: Banner
        selector: '.promo'
        watch: true
  - name: Multi
    selector: ['.promo', 'div.promo']
    watch: true
`))
	// One node inside both scopes: two distinct Banner defs match it, and Multi
	// matches it through both of its selectors.
	tree := n("body", n("form.checkout", n("section.account", n("div.promo"))))
	node := find(tree, "promo")
	want := []string{"Banner", "Banner", "Multi"}
	if got := m.Watched(tree, "")[node]; !reflect.DeepEqual(got, want) {
		t.Errorf("Watched = %v, want %v", got, want)
	}
	if cm := m.Match(tree, "")[node]; cm == nil || !reflect.DeepEqual(cm.Watched, want) {
		t.Errorf("Match Watched = %v, want %v", cm, want)
	}
}

func TestWithholds(t *testing.T) {
	ex := func(from, path string) sightmap.Extract { return sightmap.Extract{From: from, Path: path} }
	for _, tc := range []struct {
		privacy string
		e       sightmap.Extract
		want    bool
	}{
		{"", ex(sightmap.FromDOMText, ""), false},
		{"unmask", ex(sightmap.FromDOMText, ""), false},
		{"mask", ex(sightmap.FromDOMText, ""), true},
		{"block", ex(sightmap.FromDOMRawText, ""), true},
		{"mask", ex(sightmap.FromDOMAttr, "data-sku"), true},
		{"mask", ex(sightmap.FromDOMAttr, "checked"), false},
		{"block", ex(sightmap.FromDOMAttr, "checked"), true},
		{"mask", ex(sightmap.FromDOMState, "expanded"), false},
		{"block", ex(sightmap.FromDOMState, "expanded"), true},
		{"block", ex(sightmap.FromComponent, "Price.text"), false},
	} {
		if got := match.Withholds(tc.privacy, tc.e); got != tc.want {
			t.Errorf("Withholds(%q, %s %s) = %v, want %v", tc.privacy, tc.e.From, tc.e.Path, got, tc.want)
		}
	}
}

// randomCaptureCorpus is benchCorpus with random privacy and watch on its
// definitions, including an unrecognized privacy value.
func randomCaptureCorpus(seed uint64, comps int, exotic bool) *sightmap.Corpus {
	c := benchCorpus(seed, comps, exotic)
	r := rand.New(rand.NewPCG(seed, 3))
	choices := []string{"", "", "", "block", "mask", "unmask", "unmask", "secret"}
	for i := range c.GlobalComponents {
		c.GlobalComponents[i].Privacy = choices[r.IntN(len(choices))]
		c.GlobalComponents[i].Watch = r.IntN(4) == 0
	}
	return c
}

// oracle resolves privacy and watch independently of the package: the
// pre-index reference matcher, SEP-0009's fold written out longhand, and
// SEP-0018's handling of rules outside the capture-baseline profile.
func oracle(root *sightmap.ComponentNode, defs []sightmap.ComponentDef) (map[*sightmap.ComponentNode]string, map[*sightmap.ComponentNode][]string) {
	rank := map[string]int{"unmask": 1, "mask": 2, "block": 3}
	floor := 0
	var queries []match.MatchQuery
	privacyOf := map[*match.MatchQuery]int{}
	watchOf := map[*match.MatchQuery]bool{}
	type pending struct {
		q     match.MatchQuery
		r     int
		watch bool
	}
	var ps []pending
	for _, d := range defs {
		if d.Privacy == "" && !d.Watch {
			continue
		}
		r := 0
		if d.Privacy != "" {
			var ok bool
			if r, ok = rank[d.Privacy]; !ok {
				r = 3
			}
		}
		for _, sel := range d.Selectors {
			parsed, err := sightmap.ParseSightmapSelector(sel)
			prof, perr := sightmap.ParseProfileSelector(sel, sightmap.ProfileCaptureBaseline)
			valid := err == nil && perr == nil
			pr, w := r, d.Watch && valid
			if pr != 0 && (!valid || prof.Subject().Constraint == sightmap.ConstraintNone) {
				if pr != 1 && pr > floor {
					floor = pr
				}
				pr = 0
			}
			if pr == 0 && !w {
				continue
			}
			ps = append(ps, pending{match.MatchQuery{Name: d.Name, Parts: parsed.Parts, Combinators: parsed.Combinators}, pr, w})
		}
	}
	for _, p := range ps {
		queries = append(queries, p.q)
	}
	for i := range queries {
		privacyOf[&queries[i]] = ps[i].r
		watchOf[&queries[i]] = ps[i].watch
	}
	local := map[*sightmap.ComponentNode]int{}
	watched := map[*sightmap.ComponentNode][]string{}
	referenceFindAllMatches(root, queries, func(node *sightmap.ComponentNode, q *match.MatchQuery) {
		if privacyOf[q] > local[node] {
			local[node] = privacyOf[q]
		}
		if watchOf[q] && !slices.Contains(watched[node], q.Name) {
			watched[node] = append(watched[node], q.Name)
		}
	})
	for _, w := range watched {
		slices.Sort(w)
	}
	names := []string{"", "unmask", "mask", "block"}
	privacy := map[*sightmap.ComponentNode]string{}
	var walk func(n *sightmap.ComponentNode, parent int)
	walk = func(n *sightmap.ComponentNode, parent int) {
		e := parent
		if parent == 3 || local[n] == 3 {
			e = 3
		} else if local[n] != 0 {
			e = local[n]
		}
		if floor > e {
			e = floor
		}
		if e > 0 {
			privacy[n] = names[e]
		}
		for _, c := range n.Children {
			walk(c, e)
		}
	}
	walk(root, 0)
	return privacy, watched
}

// TestCapture_EqualsOracle: across seeded pages and corpora with random privacy
// and watch, Privacy and Watched equal the oracle on every node, resolving from
// each node's ancestor chain alone gives the same answer, and Match reports the
// same on every node it names.
func TestCapture_EqualsOracle(t *testing.T) {
	seen := map[string]int{}
	nodes, watchedNodes := 0, 0
	for seed := uint64(1); seed <= 40; seed++ {
		for _, exotic := range []bool{false, true} {
			corpus := randomCaptureCorpus(seed, 10+int(seed)*3, exotic)
			root := benchTree(seed, 300, 10+int(seed)*3, exotic)
			m := match.NewMatcher(corpus)
			privacy, watched := m.Privacy(root, ""), m.Watched(root, "")
			wantP, wantW := oracle(root, corpus.GlobalComponents)
			var walk func(n *sightmap.ComponentNode, chain []sightmap.Element)
			walk = func(n *sightmap.ComponentNode, chain []sightmap.Element) {
				chain = append(slices.Clip(chain), *n.Element)
				if privacy[n] != wantP[n] || !slices.Equal(watched[n], wantW[n]) {
					t.Fatalf("seed %d exotic %v: privacy %q watched %v, oracle %q %v", seed, exotic, privacy[n], watched[n], wantP[n], wantW[n])
				}
				cp, cw := m.PrivacyForChain(chain, ""), m.WatchedForChain(chain, "")
				if cp[len(cp)-1] != privacy[n] || !slices.Equal(cw[len(cw)-1], watched[n]) {
					t.Fatalf("seed %d exotic %v: chain %q %v, tree %q %v", seed, exotic, cp[len(cp)-1], cw[len(cw)-1], privacy[n], watched[n])
				}
				seen[privacy[n]]++
				nodes++
				if len(watched[n]) > 0 {
					watchedNodes++
				}
				for _, c := range n.Children {
					walk(c, chain)
				}
			}
			walk(root, nil)
			for node, cm := range m.Match(root, "") {
				if cm.Privacy != privacy[node] || !slices.Equal(cm.Watched, watched[node]) {
					t.Fatalf("seed %d exotic %v: Match %q %v, Privacy/Watched %q %v", seed, exotic, cm.Privacy, cm.Watched, privacy[node], watched[node])
				}
			}
		}
	}
	for _, p := range []string{"", "unmask", "mask", "block"} {
		if seen[p] == 0 {
			t.Errorf("no node resolved to %q", p)
		}
	}
	if watchedNodes == 0 {
		t.Error("no node was watched")
	}
	t.Logf("%d nodes agree with the oracle (%d watched); privacy %v", nodes, watchedNodes, seen)
}
