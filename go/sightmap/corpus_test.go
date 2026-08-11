package sightmap_test

import (
	"fmt"
	"github.com/sightmap/sightmap/go/match"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/sightmap"
)

// ---- 1. DirLoader: global + view files --------------------------------------

func TestDirLoaderParsesGlobalAndViewFiles(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "components.yaml"), `
version: 1
components:
  - name: SiteHeader
    selector: "#header"
    memory:
      - "global header"
    children:
      - name: Logo
        selector: ".logo"
`)

	if err := os.MkdirAll(filepath.Join(dir, "views"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "views", "page.yaml"), `
version: 1
views:
  - name: ProductPage
    route: "/product/*"
    memory:
      - "product page"
    components:
      - $ref: SiteHeader
      - name: ProductTitle
        selector: "h1.product-title"
`)

	corpus, err := sightmap.DirLoader(dir).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Global components: SiteHeader + Logo (child, flattened).
	if len(corpus.GlobalComponents) != 2 {
		t.Errorf("GlobalComponents: want 2, got %d: %v",
			len(corpus.GlobalComponents), compNames(corpus.GlobalComponents))
	}
	globalByName := indexByName(corpus.GlobalComponents)

	header, ok := globalByName["SiteHeader"]
	if !ok {
		t.Fatal("SiteHeader not found in GlobalComponents")
	}
	if !equalSels(header.Selectors, []string{"#header"}) {
		t.Errorf("SiteHeader.Selectors: want [\"#header\"], got %v", header.Selectors)
	}
	if len(header.Memory) != 1 || header.Memory[0] != "global header" {
		t.Errorf("SiteHeader.Memory: want [\"global header\"], got %v", header.Memory)
	}

	logo, ok := globalByName["Logo"]
	if !ok {
		t.Fatal("Logo not found in GlobalComponents")
	}
	if !equalSels(logo.Selectors, []string{"#header .logo"}) {
		t.Errorf("Logo.Selectors: want [\"#header .logo\"], got %v", logo.Selectors)
	}

	// One view with correct metadata.
	if len(corpus.Views) != 1 {
		t.Fatalf("Views: want 1, got %d", len(corpus.Views))
	}
	v := corpus.Views[0]
	if v.Name != "ProductPage" {
		t.Errorf("View.Name: want ProductPage, got %q", v.Name)
	}
	if v.Route != "/product/*" {
		t.Errorf("View.Route: want /product/*, got %q", v.Route)
	}
	if len(v.Memory) != 1 || v.Memory[0] != "product page" {
		t.Errorf("View.Memory: want [\"product page\"], got %v", v.Memory)
	}

	// View expands $ref SiteHeader → SiteHeader + Logo + ProductTitle = 3 entries.
	if len(v.Components) != 3 {
		t.Errorf("View.Components: want 3, got %d: %v", len(v.Components), compNames(v.Components))
	}
}

// ---- 2. Flattening: parent selector prepended to children ------------------

func TestFlattening(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "components.yaml"), `
version: 1
components:
  - name: SearchBox
    selector: "form#header-search"
    children:
      - name: SearchInput
        selector: "input, combobox"
      - name: SearchSubmit
        selector: "button"
`)

	corpus, err := sightmap.DirLoader(dir).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	byName := indexByName(corpus.GlobalComponents)

	// SearchBox: unchanged.
	sb := mustGet(t, byName, "SearchBox")
	if !equalSels(sb.Selectors, []string{"form#header-search"}) {
		t.Errorf("SearchBox.Selectors: want [form#header-search], got %v", sb.Selectors)
	}

	// SearchInput: comma-separated → both alternatives prefixed.
	si := mustGet(t, byName, "SearchInput")
	wantSI := []string{"form#header-search input", "form#header-search combobox"}
	if !equalSels(si.Selectors, wantSI) {
		t.Errorf("SearchInput.Selectors:\n  want %v\n   got %v", wantSI, si.Selectors)
	}

	// SearchSubmit: single child selector prefixed.
	ss := mustGet(t, byName, "SearchSubmit")
	if !equalSels(ss.Selectors, []string{"form#header-search button"}) {
		t.Errorf("SearchSubmit.Selectors: want [form#header-search button], got %v", ss.Selectors)
	}
}

// Deep nesting: grandchild selectors carry all ancestors.
func TestFlatteningDeep(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "components.yaml"), `
version: 1
components:
  - name: Nav
    selector: "nav"
    children:
      - name: NavList
        selector: "ul"
        children:
          - name: NavItem
            selector: "li"
`)

	corpus, err := sightmap.DirLoader(dir).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	byName := indexByName(corpus.GlobalComponents)

	navItem := mustGet(t, byName, "NavItem")
	if !equalSels(navItem.Selectors, []string{"nav ul li"}) {
		t.Errorf("NavItem.Selectors: want [nav ul li], got %v", navItem.Selectors)
	}
}

// ---- 3. $ref expansion ------------------------------------------------------

func TestRefExpansion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "components.yaml"), `
version: 1
components:
  - name: GlobalNav
    selector: "nav#global"
    memory:
      - "global navigation"
    children:
      - name: NavLogo
        selector: ".logo"
`)
	if err := os.MkdirAll(filepath.Join(dir, "views"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "views", "home.yaml"), `
version: 1
views:
  - name: HomePage
    route: "/"
    components:
      - $ref: GlobalNav
      - name: Hero
        selector: ".hero"
`)

	corpus, err := sightmap.DirLoader(dir).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(corpus.Views) != 1 {
		t.Fatalf("Views: want 1, got %d", len(corpus.Views))
	}
	byName := indexByName(corpus.Views[0].Components)

	// $ref GlobalNav is expanded.
	gn := mustGet(t, byName, "GlobalNav")
	if !equalSels(gn.Selectors, []string{"nav#global"}) {
		t.Errorf("GlobalNav.Selectors: want [nav#global], got %v", gn.Selectors)
	}
	if len(gn.Memory) != 1 || gn.Memory[0] != "global navigation" {
		t.Errorf("GlobalNav.Memory: want [global navigation], got %v", gn.Memory)
	}

	// NavLogo (child of GlobalNav) is also expanded.
	nl := mustGet(t, byName, "NavLogo")
	if !equalSels(nl.Selectors, []string{"nav#global .logo"}) {
		t.Errorf("NavLogo.Selectors: want [nav#global .logo], got %v", nl.Selectors)
	}

	// Hero is a normal inline component.
	hero := mustGet(t, byName, "Hero")
	if !equalSels(hero.Selectors, []string{".hero"}) {
		t.Errorf("Hero.Selectors: want [.hero], got %v", hero.Selectors)
	}
}

// Unknown $ref is skipped silently (no panic, no error).
func TestRefExpansionUnknownRef(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "views", "p.yaml"), `
version: 1
views:
  - name: P
    route: "/p"
    components:
      - $ref: DoesNotExist
      - name: Known
        selector: ".known"
`)
	if err := os.MkdirAll(filepath.Join(dir, "views"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Re-write inside the created dir.
	writeFile(t, filepath.Join(dir, "views", "p.yaml"), `
version: 1
views:
  - name: P
    route: "/p"
    components:
      - $ref: DoesNotExist
      - name: Known
        selector: ".known"
`)

	corpus, err := sightmap.DirLoader(dir).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(corpus.Views) != 1 {
		t.Fatalf("Views: want 1, got %d", len(corpus.Views))
	}
	byName := indexByName(corpus.Views[0].Components)
	if _, ok := byName["DoesNotExist"]; ok {
		t.Error("DoesNotExist should have been skipped")
	}
	if _, ok := byName["Known"]; !ok {
		t.Error("Known component should be present")
	}
}

// ---- 4. Route matching ------------------------------------------------------

func TestRouteMatching(t *testing.T) {
	corpus := &sightmap.Corpus{
		GlobalComponents: []sightmap.ComponentDef{
			{Name: "Global", Selectors: []string{"body"}},
		},
		Views: []sightmap.View{
			{Name: "B", Route: "/b/**", Components: []sightmap.ComponentDef{
				{Name: "BComp", Selectors: []string{".b"}},
			}},
			{Name: "Product", Route: "/product/*", Components: []sightmap.ComponentDef{
				{Name: "PComp", Selectors: []string{".p"}},
			}},
			{Name: "Home", Route: "/", Components: []sightmap.ComponentDef{
				{Name: "HomeComp", Selectors: []string{".home"}},
			}},
		},
	}

	matchCases := []struct {
		url  string
		want string
	}{
		{"https://example.com/b/Tools", "BComp"},
		{"https://example.com/b/Tools/Drills/N-123", "BComp"},
		{"https://example.com/product/123", "PComp"},
		{"https://example.com/", "HomeComp"},
	}
	for _, tc := range matchCases {
		t.Run(tc.url, func(t *testing.T) {
			list := corpus.ComponentsForURL(tc.url)
			if !hasComp(list, tc.want) {
				t.Errorf("expected %q in result %v", tc.want, compNames(list))
			}
		})
	}

	noMatchCases := []string{
		// * does not match multi-segment paths
		"https://example.com/product/123/reviews",
		// no view registered
		"https://example.com/other",
	}
	for _, u := range noMatchCases {
		t.Run(u, func(t *testing.T) {
			list := corpus.ComponentsForURL(u)
			// Should return only globals.
			if len(list) != 1 || list[0].Name != "Global" {
				t.Errorf("expected only Global, got %v", compNames(list))
			}
		})
	}
}

// ---- 5. ComponentsForURL merging --------------------------------------------

func TestComponentsForURL(t *testing.T) {
	globalA := sightmap.ComponentDef{Name: "A", Selectors: []string{"#a"}}
	globalB := sightmap.ComponentDef{Name: "B", Selectors: []string{"#b-global"}}
	viewB := sightmap.ComponentDef{Name: "B", Selectors: []string{"#b-view"}}
	viewC := sightmap.ComponentDef{Name: "C", Selectors: []string{"#c"}}

	corpus := &sightmap.Corpus{
		GlobalComponents: []sightmap.ComponentDef{globalA, globalB},
		Views: []sightmap.View{
			{Route: "/page", Components: []sightmap.ComponentDef{viewB, viewC}},
		},
	}

	// No match → returns GlobalComponents unchanged.
	t.Run("no_match", func(t *testing.T) {
		list := corpus.ComponentsForURL("https://example.com/other")
		if len(list) != 2 {
			t.Errorf("want 2 comps, got %d: %v", len(list), compNames(list))
		}
	})

	// View match → view components + non-colliding globals.
	t.Run("match_merge", func(t *testing.T) {
		list := corpus.ComponentsForURL("https://example.com/page")
		byName := indexByName(list)
		if len(byName) != 3 {
			t.Errorf("want 3 unique comps, got %d: %v", len(byName), compNames(list))
		}
		// A from globals (no collision).
		if _, ok := byName["A"]; !ok {
			t.Error("A missing from merged result")
		}
		// C from view.
		if _, ok := byName["C"]; !ok {
			t.Error("C missing from merged result")
		}
		// B: view wins — selector should be #b-view.
		b := byName["B"]
		if len(b.Selectors) == 0 || b.Selectors[0] != "#b-view" {
			t.Errorf("B: view should win; got selectors %v", b.Selectors)
		}
	})
}

func TestAllComponents(t *testing.T) {
	globalA := sightmap.ComponentDef{Name: "A", Selectors: []string{"#a"}}
	globalB := sightmap.ComponentDef{Name: "B", Selectors: []string{"#b-global"}}
	viewB := sightmap.ComponentDef{Name: "B", Selectors: []string{"#b-view"}} // $ref-expanded global, same name
	viewC := sightmap.ComponentDef{Name: "C", Selectors: []string{"#c"}}

	corpus := &sightmap.Corpus{
		GlobalComponents: []sightmap.ComponentDef{globalA, globalB},
		Views: []sightmap.View{
			{Route: "/page", Components: []sightmap.ComponentDef{viewB, viewC}},
		},
	}

	list := corpus.AllComponents()
	byName := indexByName(list)
	if len(byName) != 3 {
		t.Fatalf("want 3 unique comps, got %d: %v", len(byName), compNames(list))
	}
	// B appears in both GlobalComponents and a view; the global (first-seen) wins.
	if b := byName["B"]; len(b.Selectors) == 0 || b.Selectors[0] != "#b-global" {
		t.Errorf("B: first-seen (global) should win; got selectors %v", byName["B"].Selectors)
	}
	if _, ok := byName["A"]; !ok {
		t.Error("A missing from AllComponents")
	}
	if _, ok := byName["C"]; !ok {
		t.Error("C missing from AllComponents")
	}
}

// ---- 6. MatchTree end-to-end ------------------------------------------------

func TestMatchTreeEndToEnd(t *testing.T) {
	// Tree: root → form#search → {input, button}
	inputNode := &comps.ComponentNode{
		Id:      "input",
		Element: &comps.Element{Tag: "input"},
	}
	buttonNode := &comps.ComponentNode{
		Id:      "button",
		Element: &comps.Element{Tag: "button"},
	}
	formNode := &comps.ComponentNode{
		Id:       "form",
		Element:  &comps.Element{Tag: "form", Id: "search-form"},
		Children: []*comps.ComponentNode{inputNode, buttonNode},
	}
	root := &comps.ComponentNode{
		Id:       "root",
		Children: []*comps.ComponentNode{formNode},
	}

	corpus := &sightmap.Corpus{
		GlobalComponents: []sightmap.ComponentDef{
			{Name: "SearchForm", Selectors: []string{"form#search-form"}, Memory: []string{"search form hint"}},
			{Name: "SearchInput", Selectors: []string{"form#search-form input"}},
			{Name: "SearchSubmit", Selectors: []string{"form#search-form button"}, Memory: []string{"submit hint"}},
		},
	}

	matches := match.NewMatcher(corpus).MatchTree(root, "https://example.com/search")

	// formNode → SearchForm
	m := mustMatch(t, matches, formNode, "formNode")
	if m.Name != "SearchForm" {
		t.Errorf("formNode: want SearchForm, got %q", m.Name)
	}
	if len(m.Memory) == 0 || m.Memory[0] != "search form hint" {
		t.Errorf("formNode memory: want [search form hint], got %v", m.Memory)
	}

	// inputNode → SearchInput
	m = mustMatch(t, matches, inputNode, "inputNode")
	if m.Name != "SearchInput" {
		t.Errorf("inputNode: want SearchInput, got %q", m.Name)
	}

	// buttonNode → SearchSubmit
	m = mustMatch(t, matches, buttonNode, "buttonNode")
	if m.Name != "SearchSubmit" {
		t.Errorf("buttonNode: want SearchSubmit, got %q", m.Name)
	}
	if len(m.Memory) == 0 || m.Memory[0] != "submit hint" {
		t.Errorf("buttonNode memory: want [submit hint], got %v", m.Memory)
	}

	// root has no matching selector — must not be in the result map.
	if _, ok := matches[root]; ok {
		t.Error("root should not be matched")
	}
}

// ---- 8. Concurrent safety (run with -race) ----------------------------------

func TestConcurrentSafety(t *testing.T) {
	buttonNode := &comps.ComponentNode{
		Id:      "btn",
		Element: &comps.Element{Tag: "button"},
	}
	root := &comps.ComponentNode{
		Id:       "root",
		Children: []*comps.ComponentNode{buttonNode},
	}

	corpus := &sightmap.Corpus{
		GlobalComponents: []sightmap.ComponentDef{
			{Name: "Button", Selectors: []string{"button"}},
		},
	}

	const goroutines = 30
	var wg sync.WaitGroup
	wg.Add(goroutines)

	errs := make([]error, goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			// Use a few distinct URLs to exercise the multi-key cache path.
			url := fmt.Sprintf("https://example.com/page/%d", i%5)
			matches := match.NewMatcher(corpus).MatchTree(root, url)
			if _, ok := matches[buttonNode]; !ok {
				errs[i] = fmt.Errorf("goroutine %d: button not matched", i)
			}
		}(i)
	}

	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
}

// ---- test helpers -----------------------------------------------------------

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

func indexByName(cs []sightmap.ComponentDef) map[string]sightmap.ComponentDef {
	m := make(map[string]sightmap.ComponentDef, len(cs))
	for _, c := range cs {
		m[c.Name] = c
	}
	return m
}

func compNames(cs []sightmap.ComponentDef) []string {
	names := make([]string, len(cs))
	for i, c := range cs {
		names[i] = c.Name
	}
	return names
}

func equalSels(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hasComp(cs []sightmap.ComponentDef, name string) bool {
	for _, c := range cs {
		if c.Name == name {
			return true
		}
	}
	return false
}

func mustGet(t *testing.T, m map[string]sightmap.ComponentDef, name string) sightmap.ComponentDef {
	t.Helper()
	c, ok := m[name]
	if !ok {
		t.Fatalf("component %q not found in map (have: %v)", name, func() []string {
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			return keys
		}())
	}
	return c
}

func mustMatch(
	t *testing.T,
	matches map[*comps.ComponentNode]*sightmap.ComponentMatch,
	node *comps.ComponentNode,
	label string,
) *sightmap.ComponentMatch {
	t.Helper()
	m, ok := matches[node]
	if !ok {
		t.Fatalf("%s: expected a match but none found", label)
	}
	return m
}
