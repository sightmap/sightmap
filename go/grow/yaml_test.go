package grow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/explore"
	"github.com/sightmap/sightmap/go/observe"
	"github.com/sightmap/sightmap/go/sightmap"
)

const viewYAML = `version: 1
# hand-written comment that must survive
views:
  - name: Home
    route: "/"
    url: https://b/
    components: []
  - name: Catalogue
    route: /catalogue/*
    components:
      - name: Existing
        selector: '.keep'
`

func newCorpus(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "views"), 0o755)
	os.WriteFile(filepath.Join(dir, "components.yaml"), []byte("version: 1\ncomponents: []\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "views", "home.yaml"), []byte(viewYAML), 0o644)
	return dir
}

func TestWriteComponentAppendsAndLoads(t *testing.T) {
	dir := newCorpus(t)
	g := New(dir, nil)
	comp := component{Name: "NavListLink", Selector: "ul.nav-list a", Description: "grown", Properties: []property{{Name: "label", Extract: "text"}, {Name: "href", Extract: "attr=href"}}}
	if err := g.writeComponent("Home", comp); err != nil {
		t.Fatal(err)
	}
	if err := g.writeComponent("Catalogue", component{Name: "PagerLink", Selector: "ul.pager a"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "views", "home.yaml"))
	if !strings.Contains(string(data), "hand-written comment that must survive") {
		t.Fatalf("comment lost:\n%s", data)
	}
	c, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if errs := sightmap.Validate(c); len(errs) > 0 {
		t.Fatalf("validate: %v", errs)
	}
	home := c.ViewByName("Home")
	if len(home.Components) != 1 || home.Components[0].Name != "NavListLink" || home.Components[0].Selectors[0] != "ul.nav-list a" || len(home.Components[0].Properties) != 2 {
		t.Fatalf("home = %+v", home.Components)
	}
	cat := c.ViewByName("Catalogue")
	if len(cat.Components) != 2 || cat.Components[0].Name != "Existing" || cat.Components[1].Name != "PagerLink" {
		t.Fatalf("catalogue = %+v", cat.Components)
	}
}

func TestWriteComponentRollsBackWhenCorpusBreaks(t *testing.T) {
	dir := newCorpus(t)
	g := New(dir, nil)
	before, _ := os.ReadFile(filepath.Join(dir, "views", "home.yaml"))
	g.Reload = func() error { return os.ErrInvalid }
	if err := g.writeComponent("Home", component{Name: "Bad", Selector: "a"}); err == nil {
		t.Fatal("expected the reload error")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "views", "home.yaml"))
	if string(after) != string(before) {
		t.Fatalf("file not restored:\n%s", after)
	}
}

func TestPromoteToGlobal(t *testing.T) {
	dir := newCorpus(t)
	g := New(dir, nil)
	if err := g.writeComponent("Home", component{Name: "HeaderLink", Selector: "header a", Description: "d"}); err != nil {
		t.Fatal(err)
	}
	if err := g.promoteToGlobal(owner{name: "HeaderLink", view: "Home"}); err != nil {
		t.Fatal(err)
	}
	c, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.ViewByName("Home").Components) != 0 {
		t.Fatal("component still on the view")
	}
	if len(c.GlobalComponents) != 1 || c.GlobalComponents[0].Name != "HeaderLink" || !strings.Contains(c.GlobalComponents[0].Source+" "+strings.Join(c.GlobalComponents[0].Memory, ""), "") {
		t.Fatalf("globals = %+v", c.GlobalComponents)
	}
	if g.Stats().Promoted != 1 {
		t.Fatal("promoted count")
	}
	// promoting again is a no-op
	if err := g.promoteToGlobal(owner{name: "HeaderLink", view: "Home"}); err == nil {
		t.Fatal("expected error: no longer on the view")
	}
}

func TestEnsureViewCreatesRouteView(t *testing.T) {
	dir := newCorpus(t)
	g := New(dir, nil)
	c, _ := sightmap.Load(dir)
	name, err := g.ensureView(c, "https://b/catalogue/category/books/travel_2/index.html")
	if err != nil || name != "CategoryBooksDetail" {
		t.Fatalf("name = %q err = %v", name, err)
	}
	c, _ = sightmap.Load(dir)
	v := c.ViewForURL("https://b/catalogue/category/books/mystery_3/index.html")
	if v == nil || v.Name != "CategoryBooksDetail" {
		t.Fatalf("route did not generalise: %+v", v)
	}
	// an existing route pattern is reused, not duplicated
	again, _ := g.ensureView(c, "https://b/catalogue/category/books/poetry_23/index.html")
	if again != "CategoryBooksDetail" {
		t.Fatalf("again = %q", again)
	}
	if g.Stats().Views != 1 {
		t.Fatalf("views = %d", g.Stats().Views)
	}
}

// classifyAll is a picker that calls everything a link.
type classifyAll struct{ kind string }

func (c classifyAll) Name() string { return "fake" }
func (c classifyAll) Pick(context.Context, string, explore.Criteria) (explore.Pick, error) {
	return explore.Pick{}, nil
}
func (c classifyAll) Choose(context.Context, string, explore.Criteria, string) (string, error) {
	return c.kind, nil
}
func (c classifyAll) Stats() explore.Stats { return explore.Stats{} }

func TestOnPageGrowsGroupedLinks(t *testing.T) {
	dir := newCorpus(t)
	// A page: <ul class="nav-list"><li><a>Travel</a></li><li><a>Mystery</a></li></ul><a href="/x" id="logo">Logo</a>
	link := func(id, name, href string) *sightmap.ComponentNode {
		return &sightmap.ComponentNode{Id: id, Role: "link", Name: name, IsInteractive: true, IsVisible: true, Element: &sightmap.Element{Tag: "a", Attrs: map[string]string{"href": href}}}
	}
	list := &sightmap.ComponentNode{Id: "ul", Role: "list", IsVisible: true, Element: &sightmap.Element{Tag: "ul", Classes: []string{"nav-list"}}}
	li1 := &sightmap.ComponentNode{Id: "li1", Role: "listitem", IsVisible: true, Element: &sightmap.Element{Tag: "li"}, Children: []*sightmap.ComponentNode{link("a1", "Travel", "/travel")}}
	li2 := &sightmap.ComponentNode{Id: "li2", Role: "listitem", IsVisible: true, Element: &sightmap.Element{Tag: "li"}, Children: []*sightmap.ComponentNode{link("a2", "Mystery", "/mystery")}}
	list.Children = []*sightmap.ComponentNode{li1, li2}
	logo := link("a3", "Logo", "/")
	logo.Element.Id = "logo"
	root := &sightmap.ComponentNode{Id: "root", Role: "none", IsVisible: true, Element: &sightmap.Element{Tag: "body"}, Children: []*sightmap.ComponentNode{list, logo}}

	res := &observe.Result{Root: root, Matches: map[*sightmap.ComponentNode]*sightmap.ComponentMatch{}}
	page := explore.NewPage(res, "https://b/")
	g := New(dir, classifyAll{kind: "link"})
	if err := g.OnPage(context.Background(), page); err != nil {
		t.Fatal(err)
	}
	st := g.Stats()
	if st.Added != 2 || st.Calls != 2 {
		t.Fatalf("stats = %+v", st)
	}
	c, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	home := c.ViewByName("Home")
	var names, sels []string
	for _, comp := range home.Components {
		names = append(names, comp.Name)
		sels = append(sels, comp.Selectors[0])
	}
	if strings.Join(names, ",") != "NavListLink,LogoLink" || strings.Join(sels, ",") != "ul.nav-list a,#logo" {
		t.Fatalf("names = %v sels = %v", names, sels)
	}
	// the new corpus covers the page
	if OfflineCount(root, "ul.nav-list a") != 2 {
		t.Fatal("offline count")
	}
	// a second visit with the page now covered adds nothing
	res2 := &observe.Result{Root: root, Matches: map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		li1.Children[0]: {Name: "NavListLink"}, li2.Children[0]: {Name: "NavListLink"}, logo: {Name: "LogoLink"},
	}}
	if err := g.OnPage(context.Background(), explore.NewPage(res2, "https://b/")); err != nil {
		t.Fatal(err)
	}
	if g.Stats().Added != 2 {
		t.Fatal("grew again on a covered page")
	}
}

func TestOnPageSkipsNoise(t *testing.T) {
	dir := newCorpus(t)
	btn := &sightmap.ComponentNode{Id: "b", Role: "button", Name: "x", IsInteractive: true, IsVisible: true, Element: &sightmap.Element{Tag: "button", Classes: []string{"btn"}}}
	root := &sightmap.ComponentNode{Id: "root", Role: "none", IsVisible: true, Element: &sightmap.Element{Tag: "body"}, Children: []*sightmap.ComponentNode{btn}}
	page := explore.NewPage(&observe.Result{Root: root, Matches: map[*sightmap.ComponentNode]*sightmap.ComponentMatch{}}, "https://b/")
	g := New(dir, classifyAll{kind: "noise"})
	if err := g.OnPage(context.Background(), page); err != nil {
		t.Fatal(err)
	}
	if st := g.Stats(); st.Added != 0 || st.Skipped != 1 {
		t.Fatalf("stats = %+v", st)
	}
}
