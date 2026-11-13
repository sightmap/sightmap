package match_test

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"sync"
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
	fixtures, err := filepath.Glob("../conformance/fixtures/*")
	if err != nil || len(fixtures) == 0 {
		t.Fatalf("no conformance fixtures found (err %v)", err)
	}
	for _, dir := range fixtures {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			treeData, err := os.ReadFile(filepath.Join(dir, "component-tree.json"))
			if err != nil {
				t.Fatalf("read tree: %v", err)
			}
			var root sightmap.ComponentNode
			if err := json.Unmarshal(treeData, &root); err != nil {
				t.Fatal(err)
			}
			smData, err := os.ReadFile(filepath.Join(dir, "sightmap.yaml"))
			if err != nil {
				t.Fatalf("read sightmap: %v", err)
			}
			var defs []sightmap.ComponentDef
			if err := yaml.Unmarshal(smData, &defs); err != nil {
				t.Fatalf("parse sightmap: %v", err)
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

// FindAll and Claims from the cache report what FindAllMatches does, cold,
// warm and with part of the set already cached, whichever queries have lost
// their Key: a keyless query is never cached, so it is walked on every call and
// must come back in its own place among the keyed ones.
func TestClaimCacheFindAll(t *testing.T) {
	root := node("root", "div", []string{"page"},
		node("c1", "div", []string{"card"}, node("l1", "span", []string{"label"})),
		node("c2", "div", []string{"card"}, node("l2", "span", nil)),
	)
	defs := []sightmap.ComponentDef{
		{Name: "Card", Selectors: []string{".card"}},
		{Name: "Label", Selectors: []string{".card > span", "span.label"}},
	}
	parse := func(blank ...int) []match.MatchQuery {
		qs, _ := match.ParseQueries(defs)
		if len(qs) != 3 {
			t.Fatalf("got %d queries, want 3", len(qs))
		}
		for _, i := range blank {
			qs[i].Key = ""
		}
		return qs
	}
	// per query index, the ids of the nodes it matches, in document order.
	index := func(qs []match.MatchQuery, q *match.MatchQuery) int {
		for i := range qs {
			if &qs[i] == q {
				return i
			}
		}
		t.Fatalf("match reported for a query outside the set")
		return -1
	}
	perQuery := func(qs []match.MatchQuery, f func(func(*sightmap.ComponentNode, *match.MatchQuery))) [][]string {
		out := make([][]string, len(qs))
		f(func(n *sightmap.ComponentNode, q *match.MatchQuery) {
			i := index(qs, q)
			out[i] = append(out[i], n.Id)
		})
		return out
	}
	for _, tc := range []struct {
		name  string
		blank []int
		only  int // when >= 0, the set is just this query
	}{
		{"keyed", nil, -1},
		{"keyless alone", []int{1}, 1},
		{"first keyless", []int{0}, -1},
		{"middle keyless", []int{1}, -1},
		{"last keyless", []int{2}, -1},
		{"two keyless", []int{0, 2}, -1},
		{"all keyless", []int{0, 1, 2}, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			qs := parse(tc.blank...)
			if tc.only >= 0 {
				qs = qs[tc.only : tc.only+1]
			}
			want := perQuery(qs, func(cb func(*sightmap.ComponentNode, *match.MatchQuery)) { match.FindAllMatches(root, qs, cb) })
			cache := match.NewClaimCache()
			for _, label := range []string{"cold", "warm", "partial", "partial warm"} {
				if label == "partial" {
					// Only the first query's key (if it has one) is known.
					cache = match.NewClaimCache()
					cache.Prepare(qs[:1]).FindAll(root, func(*sightmap.ComponentNode, *match.MatchQuery) {})
				}
				prepared := cache.Prepare(qs)
				if got := perQuery(qs, func(cb func(*sightmap.ComponentNode, *match.MatchQuery)) { prepared.FindAll(root, cb) }); !reflect.DeepEqual(got, want) {
					t.Errorf("%s FindAll: got %v, want %v", label, got, want)
				}
				claims := prepared.Claims(root)
				got := make([][]string, len(qs))
				for i, ns := range claims {
					for _, n := range ns {
						got[i] = append(got[i], n.Id)
					}
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("%s Claims: got %v, want %v", label, got, want)
				}
			}
		})
	}
}

// shopTree is a small page of cards, labels and prices, shaped by n so that
// several trees differ.
func shopTree(n int) *sightmap.ComponentNode {
	root := node(fmt.Sprintf("root%d", n), "div", []string{"page"})
	for i := 0; i < 3+n%3; i++ {
		card := node(fmt.Sprintf("card%d-%d", n, i), "div", []string{"card"},
			node(fmt.Sprintf("label%d-%d", n, i), "span", []string{"label"}),
			node(fmt.Sprintf("price%d-%d", n, i), "span", []string{"price"}))
		if (n+i)%2 == 0 {
			card.Children = append(card.Children, node(fmt.Sprintf("btn%d-%d", n, i), "button", []string{"add"}))
		}
		root.Children = append(root.Children, card)
	}
	return root
}

// Per-view components, reached through URLs, share keys with the globals and
// with each other: two URLs of one view, a second view, and one no view routes.
// A privacy rule on a selector a definition also names repeats that key in the
// combined set.
func TestClaimCacheViews(t *testing.T) {
	corpus := &sightmap.Corpus{
		GlobalComponents: []sightmap.ComponentDef{
			{Name: "Card", Selectors: []string{".card"}},
			{Name: "Price", Selectors: []string{"span.price"}, Watch: true},
		},
		Views: []sightmap.ViewDef{
			{Name: "Item", Route: "/item/*", Components: []sightmap.ComponentDef{
				{Name: "Label", Selectors: []string{".card > span.label"}, Privacy: "mask"},
				{Name: "Add", Selectors: []string{"button.add"}},
			}},
			{Name: "Cart", Route: "/cart", Components: []sightmap.ComponentDef{
				{Name: "Total", Selectors: []string{"span.price"}},
				{Name: "Row", Selectors: []string{".card"}},
			}},
		},
	}
	urls := []string{"https://shop.test/item/1", "https://shop.test/item/2", "https://shop.test/cart", "https://shop.test/about", "https://shop.test/%zz"}
	cache := match.NewClaimCache()
	// A run, so later calls are served partly or wholly from the cache.
	for round := 0; round < 3; round++ {
		for n := 0; n < 3; n++ {
			root := shopTree(n)
			for _, url := range urls {
				sameResult(t, fmt.Sprintf("round %d tree %d %s", round, n, url), root, corpus, cache, url)
			}
		}
	}
}

// Two definitions with one selector share a key: the cache walks it once and
// both claim the same nodes, as the uncached match does (a conflict, ordered
// as Match orders it).
func TestClaimCacheSharedKey(t *testing.T) {
	corpus := &sightmap.Corpus{GlobalComponents: []sightmap.ComponentDef{
		{Name: "Card", Selectors: []string{".card"}},
		{Name: "Tile", Selectors: []string{".card", "div.card"}},
		{Name: "Label", Selectors: []string{"span.label"}},
		{Name: "Name", Selectors: []string{"span.label"}},
	}}
	cache := match.NewClaimCache()
	for _, label := range []string{"cold", "warm"} {
		sameResult(t, label, shopTree(0), corpus, cache, "")
	}

	qs, _ := match.ParseQueries(corpus.GlobalComponents)
	root := shopTree(1)
	want := make([][]string, len(qs))
	match.FindAllMatches(root, qs, func(n *sightmap.ComponentNode, q *match.MatchQuery) {
		for i := range qs {
			if &qs[i] == q {
				want[i] = append(want[i], n.Id)
			}
		}
	})
	for _, label := range []string{"cold", "warm"} {
		got := make([][]string, len(qs))
		for i, ns := range cache.Prepare(qs).Claims(root) {
			for _, n := range ns {
				got[i] = append(got[i], n.Id)
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: claims %v, want %v", label, got, want)
		}
	}
}

// The doc promises concurrent use: goroutines over the same trees and over
// different ones, through shared and separate Matchers, all get the uncached
// answer. Run under -race.
func TestClaimCacheConcurrent(t *testing.T) {
	corpus := &sightmap.Corpus{GlobalComponents: []sightmap.ComponentDef{
		{Name: "Card", Selectors: []string{".card"}},
		{Name: "Tile", Selectors: []string{"div.card"}},
		{Name: "Label", Selectors: []string{".card > span.label"}, Privacy: "mask"},
		{Name: "Price", Selectors: []string{"span.price"}, Watch: true},
		{Name: "Add", Selectors: []string{"button.add"}},
	}}
	roots := make([]*sightmap.ComponentNode, 5)
	want := make([]string, len(roots))
	for i := range roots {
		roots[i] = shopTree(i)
		n, c := match.NewMatcher(corpus).MatchConflicts(roots[i], "")
		want[i] = render(roots[i], n, c)
	}
	qs, _ := match.ParseQueries(corpus.GlobalComponents)
	cache := match.NewClaimCache()
	shared := match.NewMatcher(corpus)
	prepared := cache.Prepare(qs)
	errs := make(chan string, 64)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < 10; k++ {
				i := (g + k) % len(roots)
				m := shared
				if g%2 == 1 {
					m = match.NewMatcher(corpus)
				}
				n, c := m.MatchConflictsCached(cache, roots[i], "")
				if got := render(roots[i], n, c); got != want[i] {
					errs <- fmt.Sprintf("goroutine %d tree %d: cached result differs", g, i)
					return
				}
				prepared.Claims(roots[i])
				cache.Stats()
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}

// Stats counts what the cache holds: one tree per root, one key per distinct
// selector, a list per (tree, key) that matched something, and their nodes.
func TestClaimCacheStats(t *testing.T) {
	if st := match.NewClaimCache().Stats(); st != (match.ClaimStats{}) {
		t.Fatalf("empty cache: %+v", st)
	}
	root := node("root", "div", []string{"page"},
		node("c1", "div", []string{"card"}),
		node("c2", "div", []string{"card"}),
		node("b", "button", nil),
	)
	qs, _ := match.ParseQueries([]sightmap.ComponentDef{
		{Name: "Card", Selectors: []string{".card"}},
		{Name: "Tile", Selectors: []string{".card"}}, // same key as Card
		{Name: "Nothing", Selectors: []string{"table"}},
	})
	cache := match.NewClaimCache()
	cache.Prepare(qs).Claims(root)
	// Keys: ".card", "table". Lists: ".card" matched, "table" did not.
	want := match.ClaimStats{Trees: 1, Keys: 2, Lists: 1, Nodes: 2}
	if st := cache.Stats(); st != want {
		t.Errorf("after one tree: %+v, want %+v", st, want)
	}
	cache.Prepare(qs).Claims(root) // warm: nothing new
	if st := cache.Stats(); st != want {
		t.Errorf("after a warm call: %+v, want %+v", st, want)
	}
	cache.Prepare(qs).Claims(node("other", "div", []string{"card"}))
	want = match.ClaimStats{Trees: 2, Keys: 2, Lists: 2, Nodes: 3}
	if st := cache.Stats(); st != want {
		t.Errorf("after a second tree: %+v, want %+v", st, want)
	}
}
