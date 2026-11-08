package match_test

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"testing"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// Real generated corpora are deep: a view's definitions are flattened from a
// nested tree, so selectors run six or more compounds root to leaf, each a
// [data-component^="X"] prefix plus exact attributes, and a capture keeps
// hundreds of those chains in flight through a subtree. The matcher skips an
// in-flight state at a node that lacks one of its pending part's required
// keys; these cases check that skip against the reference walker, which tests
// every state at every node.

var deepComps = []string{"Main", "Home", "Section", "Grid", "Item", "Card", "Header", "Body", "Text", "TextLabel", "Box", "Button", "ButtonBase", "Icon", "Flex"}

var deepTags = []string{"div", "div", "div", "span", "button", "a", "svg", "path", "input"}

// deepTree builds a tree whose nodes carry real-capture attributes:
// data-component values with "_hash" suffixes (so ^= is the only way to
// select them), data-testid, a case-insensitive type, camelCase viewBox on
// svg, and id/class either in fields or mirrored into Attrs.
func deepTree(seed uint64, nodes int) *sightmap.ComponentNode {
	r := rand.New(rand.NewPCG(seed, 11))
	count := 0
	newNode := func() *sightmap.ComponentNode {
		count++
		el := &sightmap.Element{Tag: deepTags[r.IntN(len(deepTags))], Attrs: map[string]string{}}
		if r.IntN(100) < 70 {
			el.Attrs["data-component"] = fmt.Sprintf("%s_%x", deepComps[r.IntN(len(deepComps))], r.IntN(4))
			el.Attrs["data-testid"] = []string{"box", "flex", "pane", "x-button"}[r.IntN(4)]
		}
		for range r.IntN(3) {
			el.Classes = append(el.Classes, fmt.Sprintf("k%d", r.IntN(12)))
		}
		switch p := r.IntN(100); {
		case p < 10:
			el.Id = fmt.Sprintf("n%d", r.IntN(8))
		case p < 20:
			el.Attrs["id"] = fmt.Sprintf("n%d", r.IntN(8))
		case p < 30 && len(el.Classes) > 0:
			el.Attrs["class"] = strings.Join(el.Classes, " ")
		case p < 36:
			el.Attrs["type"] = []string{"text", "TEXT", "Password"}[r.IntN(3)]
		case p < 40:
			el.Tag = strings.ToUpper(el.Tag)
		}
		if el.Tag == "svg" {
			el.Attrs["viewBox"] = "0 0 24 24"
		}
		return &sightmap.ComponentNode{Id: fmt.Sprint(count), Element: el}
	}
	root := newNode()
	queue := []*sightmap.ComponentNode{root}
	for len(queue) > 0 && count < nodes {
		n := queue[0]
		queue = queue[1:]
		k := 1
		if r.IntN(10) < 3 {
			k = 2 + r.IntN(4)
		}
		for range k {
			if count >= nodes {
				break
			}
			c := newNode()
			n.Children = append(n.Children, c)
			queue = append(queue, c)
		}
	}
	return root
}

// deepCompound returns one compound of the kinds generated corpora write,
// plus the operators and pseudo-classes whose keys need care.
func deepCompound(r *rand.Rand) string {
	c := deepComps[r.IntN(len(deepComps))]
	switch p := r.IntN(100); {
	case p < 45:
		return fmt.Sprintf(`%s[data-component^="%s"][data-testid="%s"]`, deepTags[r.IntN(3)], c, []string{"box", "flex", "pane"}[r.IntN(3)])
	case p < 60:
		return fmt.Sprintf(`[data-component^="%s"]`, c)
	case p < 64:
		// A prefix of another component's name: Text and TextLabel, Button
		// and ButtonBase share prefixes, so one node meets several keys.
		return fmt.Sprintf(`[data-component^="%s"]`, c[:1+r.IntN(len(c))])
	case p < 67:
		return fmt.Sprintf(`[data-component^="%s"][data-component^="%s"]`, c, c[:1]) // a repeated test
	case p < 70:
		return `[data-testid$="-button"]`
	case p < 73:
		return fmt.Sprintf(`[type="%s"]`, []string{"text", "password"}[r.IntN(2)]) // case-insensitive value
	case p < 76:
		return fmt.Sprintf(`[type^="%s"]`, []string{"te", "Pa"}[r.IntN(2)]) // case-insensitive prefix: no key
	case p < 79:
		return fmt.Sprintf(`#n%d`, r.IntN(8))
	case p < 81:
		return fmt.Sprintf(`[id^="n%d"]`, r.IntN(8))
	case p < 84:
		return fmt.Sprintf(`.k%d`, r.IntN(12))
	case p < 86:
		return fmt.Sprintf(`[class^="k%d"]`, r.IntN(12))
	case p < 88:
		return `svg[viewBox="0 0 24 24"]`
	case p < 90:
		return `DIV`
	case p < 92:
		return `*`
	case p < 94:
		return fmt.Sprintf(`[data-component^="%s"]:not([data-testid="box"])`, c)
	case p < 96:
		return fmt.Sprintf(`div:has(> [data-component^="%s"])`, c)
	case p < 98:
		return fmt.Sprintf(`:is([data-component^="%s"], .k%d)`, c, r.IntN(12))
	default:
		return fmt.Sprintf(`[data-component^="%s"]:not([data-component^="Main"] *)`, c)
	}
}

// deepCorpus builds comps definitions, each a chain of one to eight
// compounds joined by descendant and child combinators.
func deepCorpus(seed uint64, comps int) []sightmap.ComponentDef {
	r := rand.New(rand.NewPCG(seed, 12))
	defs := make([]sightmap.ComponentDef, 0, comps)
	for k := range comps {
		parts := 1 + r.IntN(8)
		var b strings.Builder
		for i := range parts {
			if i > 0 {
				if r.IntN(4) == 0 {
					b.WriteString(" > ")
				} else {
					b.WriteString(" ")
				}
			}
			b.WriteString(deepCompound(r))
		}
		defs = append(defs, sightmap.ComponentDef{Name: fmt.Sprintf("Def%d", k), Selectors: []string{b.String()}})
	}
	return defs
}

// TestDeepCorpusEquivalentToReference checks the matcher makes exactly the
// reference walker's onMatch calls, in order, on deep corpora.
func TestDeepCorpusEquivalentToReference(t *testing.T) {
	for seed := uint64(1); seed <= 30; seed++ {
		root := deepTree(seed, 400)
		queries, errs := match.ParseQueries(deepCorpus(seed, 60+int(seed)*5))
		if len(errs) > 0 {
			t.Fatal(errs)
		}
		want := collectCalls(referenceFindAllMatches, root, queries)
		got := collectCalls(match.FindAllMatches, root, queries)
		if len(want) < 50 {
			t.Fatalf("seed %d: reference made only %d matches; the generator is not exercising the matcher", seed, len(want))
		}
		if len(got) != len(want) {
			t.Fatalf("seed %d: got %d calls, want %d", seed, len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("seed %d call %d: got (%s, %s), want (%s, %s)",
					seed, i, got[i].node.Id, got[i].q.Name, want[i].node.Id, want[i].q.Name)
			}
		}
	}
}

// TestInheritedStateSkip covers, by hand, cases where wrongly skipping an
// in-flight state would change the result. Each pending part has a key the
// node does meet, through a path the key lookup must follow.
func TestInheritedStateSkip(t *testing.T) {
	el := func(tag string, attrs map[string]string, classes ...string) *sightmap.Element {
		return &sightmap.Element{Tag: tag, Attrs: attrs, Classes: classes}
	}
	nd := func(id string, e *sightmap.Element, kids ...*sightmap.ComponentNode) *sightmap.ComponentNode {
		return &sightmap.ComponentNode{Id: id, Element: e, Children: kids}
	}
	dc := func(v string) map[string]string { return map[string]string{"data-component": v} }

	// A scoped definition beats a global one on the node both match, through
	// several wrappers between the scope and the subject.
	scoped := nd("root", el("div", dc("Main_1")),
		nd("w1", el("div", nil),
			nd("w2", el("div", dc("Box_0")),
				nd("t", el("span", dc("Text_2"))))))

	// id and class carried in fields, not Attrs, still meet [id=]/[class^=].
	fields := nd("root", el("div", dc("Card_1")),
		nd("a", &sightmap.Element{Tag: "a", Id: "go"}),
		nd("b", el("b", nil, "lead", "x")))

	// A case-insensitive value, a camelCase SVG attribute, an uppercase tag.
	folded := nd("root", el("form", dc("Form_1")),
		nd("in", el("input", map[string]string{"type": "PASSWORD"})),
		nd("svg", el("svg", map[string]string{"viewBox": "0 0 1 1"})),
		nd("up", el("BUTTON", nil)))

	cases := []struct {
		name string
		root *sightmap.ComponentNode
		defs []sightmap.ComponentDef
		want map[string]string // node id -> name
	}{
		{"scoped beats global", scoped, []sightmap.ComponentDef{
			{Name: "Global", Selectors: []string{`[data-component^="Text"]`}},
			{Name: "Scoped", Selectors: []string{`[data-component^="Main"] [data-component^="Box"] [data-component^="Text"]`}},
		}, map[string]string{"t": "Scoped"}},
		{"child combinator needs the parent", scoped, []sightmap.ComponentDef{
			{Name: "Child", Selectors: []string{`[data-component^="Main"] > [data-component^="Box"]`}},
			{Name: "Desc", Selectors: []string{`[data-component^="Main"] [data-component^="Box"] > span`}},
		}, map[string]string{"t": "Desc"}},
		{"descendant spans wrappers", scoped, []sightmap.ComponentDef{
			{Name: "Desc", Selectors: []string{`[data-component^="Main"] [data-component^="Box"]`}},
		}, map[string]string{"w2": "Desc"}},
		{"prefix of a longer value", scoped, []sightmap.ComponentDef{
			{Name: "T", Selectors: []string{`[data-component^="Main"] [data-component^="T"]`}},
			{Name: "Te", Selectors: []string{`[data-component^="Main"] div [data-component^="Text_"]`}},
		}, map[string]string{"t": "Te"}},
		{":not() on a later part", scoped, []sightmap.ComponentDef{
			{Name: "NotBox", Selectors: []string{`[data-component^="Main"] div:not([data-component^="Box"])`}},
			{Name: "NotUnderMain", Selectors: []string{`[data-component^="Main"] span:not([data-component^="Main"] > *)`}},
		}, map[string]string{"w1": "NotBox", "t": "NotUnderMain"}},
		{":has() on a later part", scoped, []sightmap.ComponentDef{
			{Name: "HasText", Selectors: []string{`[data-component^="Main"] div:has(> [data-component^="Text"])`}},
		}, map[string]string{"w2": "HasText"}},
		{"id and class from fields", fields, []sightmap.ComponentDef{
			{Name: "ById", Selectors: []string{`[data-component^="Card"] [id="go"]`}},
			{Name: "ByClass", Selectors: []string{`[data-component^="Card"] [class^="lead"]`}},
		}, map[string]string{"a": "ById", "b": "ByClass"}},
		{"class list from fields", fields, []sightmap.ComponentDef{
			{Name: "ByClassList", Selectors: []string{`[data-component^="Card"] > [class="lead x"]`}},
		}, map[string]string{"b": "ByClassList"}},
		{"folded values and tags", folded, []sightmap.ComponentDef{
			{Name: "Pw", Selectors: []string{`[data-component^="Form"] [type="password"]`}},
			{Name: "Svg", Selectors: []string{`[data-component^="Form"] svg[viewbox="0 0 1 1"]`}},
			{Name: "Btn", Selectors: []string{`[data-component^="Form"] > button`}},
		}, map[string]string{"in": "Pw", "svg": "Svg", "up": "Btn"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := map[string]string{}
			res := match.NewMatcher(&sightmap.Corpus{GlobalComponents: c.defs}).Match(c.root, "")
			for n, m := range res {
				if m.Name != "" && !strings.HasPrefix(n.Id, "root") {
					got[n.Id] = m.Name
				}
			}
			if fmt.Sprint(got) != fmt.Sprint(c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
			// And the raw calls agree with the reference walker.
			queries, _ := match.ParseQueries(c.defs)
			want := collectCalls(referenceFindAllMatches, c.root, queries)
			gotCalls := collectCalls(match.FindAllMatches, c.root, queries)
			if fmt.Sprint(gotCalls) != fmt.Sprint(want) {
				t.Errorf("calls differ from the reference walker")
			}
		})
	}
}

// TestMatcherReuseAcrossWalks runs one Matcher over many trees, concurrently,
// so pooled walker scratch carries over between walks, and checks every
// result against a fresh Matcher's.
func TestMatcherReuseAcrossWalks(t *testing.T) {
	defs := deepCorpus(5, 200)
	shared := match.NewMatcher(&sightmap.Corpus{GlobalComponents: defs})
	trees := make([]*sightmap.ComponentNode, 24)
	for i := range trees {
		trees[i] = deepTree(uint64(100+i), 150+i*20)
	}
	summary := func(m *match.Matcher, root *sightmap.ComponentNode) string {
		res := m.Match(root, "")
		var b strings.Builder
		sightmap.Walk(root, func(n *sightmap.ComponentNode, _ int) bool {
			if cm := res[n]; cm != nil {
				fmt.Fprintf(&b, "%s=%s ", n.Id, cm.Name)
			}
			return true
		})
		for _, c := range m.Conflicts(root, "") {
			fmt.Fprintf(&b, "!%s%v ", c.Node.Id, c.Names)
		}
		return b.String()
	}
	want := make([]string, len(trees))
	for i, root := range trees {
		want[i] = summary(match.NewMatcher(&sightmap.Corpus{GlobalComponents: defs}), root)
	}
	var wg sync.WaitGroup
	for w := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for rep := range 3 {
				for i := range trees {
					j := (i + w*5 + rep) % len(trees)
					if got := summary(shared, trees[j]); got != want[j] {
						t.Errorf("tree %d differs on reuse", j)
					}
				}
			}
		}()
	}
	wg.Wait()
}

// BenchmarkMatcherDeep matches and checks conflicts on a deep corpus, the
// shape of a large generated view: hundreds of long [data-component^=]
// chains over a few-thousand-node capture.
func BenchmarkMatcherDeep(b *testing.B) {
	for _, s := range []struct{ nodes, comps int }{{2_000, 400}, {5_000, 1_300}} {
		root := deepTree(1, s.nodes)
		m := match.NewMatcher(&sightmap.Corpus{GlobalComponents: deepCorpus(1, s.comps)})
		m.Match(root, "") // compile and cache the queries outside the timed loop
		b.Run(fmt.Sprintf("nodes=%d/comps=%d", s.nodes, s.comps), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				m.Match(root, "")
				m.Conflicts(root, "")
			}
		})
	}
}
