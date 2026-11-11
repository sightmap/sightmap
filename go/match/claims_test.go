package match_test

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
	"gopkg.in/yaml.v3"
)

// render is a match result as comparable text: per node in document order,
// its name, privacy, watch and properties, then the conflicts in order.
func render(root *sightmap.ComponentNode, named map[*sightmap.ComponentNode]*sightmap.ComponentMatch, cfs []sightmap.Conflict) string {
	var out []string
	sightmap.Walk(root, func(n *sightmap.ComponentNode, _ int) bool {
		if m := named[n]; m != nil {
			b, _ := json.Marshal(m)
			out = append(out, n.Id+" "+string(b))
		}
		return true
	})
	for _, c := range cfs {
		out = append(out, fmt.Sprintf("conflict %s %v", c.Node.Id, c.Names))
	}
	b, _ := json.MarshalIndent(out, "", " ")
	return string(b)
}

func sameResult(t *testing.T, label string, root *sightmap.ComponentNode, corpus *sightmap.Corpus, cache *match.ClaimCache, url string) {
	t.Helper()
	wantN, wantC := match.NewMatcher(corpus).MatchConflicts(root, url)
	gotN, gotC := match.NewMatcher(corpus).MatchConflictsCached(cache, root, url)
	if w, g := render(root, wantN, wantC), render(root, gotN, gotC); w != g {
		t.Fatalf("%s: cached result differs\nwant %s\ngot  %s", label, w, g)
	}
	want := render(root, nil, match.NewMatcher(corpus).Conflicts(root, url))
	if got := render(root, nil, match.NewMatcher(corpus).ConflictsCached(cache, root, url)); got != want {
		t.Fatalf("%s: cached conflicts differ\nwant %s\ngot  %s", label, want, got)
	}
}

// The cached match is the match, on every conformance fixture, cold and then
// warm (the second call is served from the cache).
func TestClaimCacheConformance(t *testing.T) {
	fixtures, _ := filepath.Glob("../conformance/fixtures/*")
	if len(fixtures) == 0 {
		t.Fatal("no conformance fixtures found")
	}
	for _, dir := range fixtures {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			treeData, err := os.ReadFile(filepath.Join(dir, "component-tree.json"))
			if err != nil {
				t.Skip("no tree")
			}
			var root sightmap.ComponentNode
			if err := json.Unmarshal(treeData, &root); err != nil {
				t.Fatal(err)
			}
			smData, err := os.ReadFile(filepath.Join(dir, "sightmap.yaml"))
			if err != nil {
				t.Skip("no sightmap")
			}
			var defs []sightmap.ComponentDef
			if err := yaml.Unmarshal(smData, &defs); err != nil {
				t.Skip("not a flat definition list")
			}
			corpus := &sightmap.Corpus{GlobalComponents: defs}
			cache := match.NewClaimCache()
			sameResult(t, "cold", &root, corpus, cache, "")
			sameResult(t, "warm", &root, corpus, cache, "")
		})
	}
}

// A run of corpora that each differ a little from the one before, matched
// through one cache, gives each corpus's own result: the case the cache is
// for. The corpora mix chains, child combinators, :not(), same-named
// definitions, privacy and watch, so names, conflicts (and their order, where
// specificity cannot order the claims), privacy and watch are all exercised.
func TestClaimCacheCorpusSequence(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	tags := []string{"div", "span", "a", "li", "button"}
	classes := []string{"card", "item", "label", "price", "x"}
	var mk func(id string, depth int) *sightmap.ComponentNode
	n := 0
	mk = func(id string, depth int) *sightmap.ComponentNode {
		n++
		var cls []string
		for _, c := range classes {
			if rng.Intn(4) == 0 {
				cls = append(cls, c)
			}
		}
		attrs := map[string]string{}
		if rng.Intn(3) == 0 {
			attrs["data-testid"] = fmt.Sprintf("t%d", rng.Intn(3))
		}
		node := &sightmap.ComponentNode{Id: fmt.Sprintf("%s-%d", id, n), Element: &sightmap.Element{Tag: tags[rng.Intn(len(tags))], Classes: cls, Attrs: attrs}, IsVisible: true}
		if depth < 5 {
			for i := 0; i < 1+rng.Intn(3); i++ {
				node.Children = append(node.Children, mk(id, depth+1))
			}
		}
		return node
	}
	roots := []*sightmap.ComponentNode{mk("a", 0), mk("b", 0), mk("c", 0)}

	sel := func() string {
		part := func() string {
			s := tags[rng.Intn(len(tags))]
			if rng.Intn(2) == 0 {
				s += "." + classes[rng.Intn(len(classes))]
			}
			if rng.Intn(4) == 0 {
				s += fmt.Sprintf(`[data-testid="t%d"]`, rng.Intn(3))
			}
			if rng.Intn(5) == 0 {
				s += ":not(." + classes[rng.Intn(len(classes))] + ")"
			}
			return s
		}
		s := part()
		for rng.Intn(3) == 0 {
			if rng.Intn(2) == 0 {
				s = part() + " > " + s
			} else {
				s = part() + " " + s
			}
		}
		return s
	}
	defs := make([]sightmap.ComponentDef, 30)
	for i := range defs {
		defs[i] = sightmap.ComponentDef{Name: fmt.Sprintf("C%d", i%22), Selectors: []string{sel()}}
	}
	cache := match.NewClaimCache()
	for step := 0; step < 40; step++ {
		// Change a few definitions: a new selector, a privacy or a watch flag.
		next := make([]sightmap.ComponentDef, len(defs))
		copy(next, defs)
		for k := 0; k < 3; k++ {
			i := rng.Intn(len(next))
			switch rng.Intn(4) {
			case 0:
				next[i].Privacy = []string{"", "mask", "block", "unmask"}[rng.Intn(4)]
			case 1:
				next[i].Watch = !next[i].Watch
			default:
				next[i].Selectors = []string{sel()}
			}
		}
		defs = next
		corpus := &sightmap.Corpus{GlobalComponents: defs}
		for _, root := range roots {
			sameResult(t, fmt.Sprintf("step %d %s", step, root.Id), root, corpus, cache, "")
		}
	}
}

// FindAll from the cache reports the same (node, query) pairs as
// FindAllMatches, cold and warm.
func TestClaimCacheFindAll(t *testing.T) {
	root := node("root", "div", []string{"page"},
		node("c1", "div", []string{"card"}, node("l1", "span", []string{"label"})),
		node("c2", "div", []string{"card"}, node("l2", "span", nil)),
	)
	qs, _ := match.ParseQueries([]sightmap.ComponentDef{
		{Name: "Card", Selectors: []string{".card"}},
		{Name: "Label", Selectors: []string{".card > span", "span.label"}},
	})
	pairs := func(f func(func(*sightmap.ComponentNode, *match.MatchQuery))) map[string]bool {
		out := map[string]bool{}
		f(func(n *sightmap.ComponentNode, q *match.MatchQuery) { out[n.Id+" "+q.Key] = true })
		return out
	}
	want := pairs(func(cb func(*sightmap.ComponentNode, *match.MatchQuery)) { match.FindAllMatches(root, qs, cb) })
	cache := match.NewClaimCache()
	for _, label := range []string{"cold", "warm", "partial"} {
		if label == "partial" {
			cache = match.NewClaimCache()
			cache.Prepare(qs[:1]).FindAll(root, func(*sightmap.ComponentNode, *match.MatchQuery) {})
		}
		got := pairs(func(cb func(*sightmap.ComponentNode, *match.MatchQuery)) { cache.Prepare(qs).FindAll(root, cb) })
		if !reflect.DeepEqual(want, got) {
			t.Errorf("%s: got %v, want %v", label, got, want)
		}
	}
}
