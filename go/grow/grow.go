// Package grow builds a sightmap corpus while a page is being explored. On
// every observed page it groups the unmapped interactive nodes by their nearest
// stable ancestor selector, asks a typed model what kind of control each group
// is, names it from a template, verifies a scoped selector with the offline
// matcher, and appends the component to the view's YAML. Selectors that turn
// up on more than one view are promoted to global components. Every write is
// followed by a corpus reload and rolled back if the corpus stops loading.
package grow

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sightmap/sightmap/go/coverage"
	"github.com/sightmap/sightmap/go/explore"
	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// Kinds a group of controls can be classified as. "noise" means not worth naming.
var Kinds = []explore.Criterion{
	{Key: "button", Desc: "buttons that trigger an action"},
	{Key: "link", Desc: "navigation links"},
	{Key: "input", Desc: "text inputs or search fields"},
	{Key: "select", Desc: "select or dropdown controls"},
	{Key: "nav", Desc: "a navigation menu, tab strip, or pager"},
	{Key: "card", Desc: "repeated cards, rows, or list items"},
	{Key: "noise", Desc: "decorative, hidden, or not worth naming"},
}

// Added records one component or view the grower wrote.
type Added struct {
	Name     string `json:"name"`
	View     string `json:"view,omitempty"`
	Selector string `json:"selector,omitempty"`
	Kind     string `json:"kind"`
	Count    int    `json:"count"`
}

// Stats summarises a grower's work.
type Stats struct {
	Added    int      `json:"added"`
	Views    int      `json:"views"`
	Promoted int      `json:"promoted_global"`
	Skipped  int      `json:"skipped"`
	Rejected int      `json:"rejected"`
	Pages    int      `json:"pages"`
	Calls    int      `json:"model_calls"`
	Ms       int      `json:"ms"`
	Names    []string `json:"names"`
}

// Grower is an explore.PageHook that writes into the corpus at Dir.
type Grower struct {
	Dir        string
	Picker     explore.Picker
	MaxPerPage int // orphans examined per page visit (default 40)
	MaxVisits  int // times one route pattern is grown (default 2)
	// Reload is called after every successful write. It should reload and
	// validate the corpus and hand it to the driver; an error rolls the write back.
	Reload func() error
	Log    func(string)

	pages    map[string]int
	added    []Added
	promoted int
	skipped  int
	rejected int
	calls    int
	ms       int
}

// New returns a grower for the corpus at dir that classifies with picker.
func New(dir string, picker explore.Picker) *Grower {
	return &Grower{Dir: dir, Picker: picker, MaxPerPage: 40, MaxVisits: 2, pages: map[string]int{}}
}

// Stats reports what the grower has written.
func (g *Grower) Stats() Stats {
	st := Stats{Promoted: g.promoted, Skipped: g.skipped, Rejected: g.rejected, Pages: len(g.pages), Calls: g.calls, Ms: g.ms}
	for _, a := range g.added {
		st.Names = append(st.Names, a.Name)
		if a.Kind == "view" {
			st.Views++
		} else {
			st.Added++
		}
	}
	return st
}

func (g *Grower) log(format string, args ...interface{}) {
	if g.Log != nil {
		g.Log(fmt.Sprintf(format, args...))
	}
}

// group is a set of orphans that share a container hook, tag, and role.
type group struct {
	hook       string
	tag        string
	role       string
	members    []*explore.Node
	candidates []string // the first member's own ranked selector candidates
}

// OnPage grows the corpus from one observed page.
func (g *Grower) OnPage(ctx context.Context, page *explore.Page) error {
	t0 := time.Now()
	defer func() { g.ms += int(time.Since(t0).Milliseconds()) }()
	if page.Result == nil || page.Result.Root == nil {
		return nil
	}
	orphans := orphansOf(page)
	key := RoutePattern(page.URL)
	visits := g.pages[key]
	maxVisits := g.MaxVisits
	if maxVisits <= 0 {
		maxVisits = 2
	}
	if len(orphans) == 0 || visits >= maxVisits || (visits > 0 && len(orphans) < 3) {
		return nil
	}
	g.pages[key] = visits + 1

	corpus, err := sightmap.Load(g.Dir)
	if err != nil {
		return fmt.Errorf("grow: load corpus: %w", err)
	}
	viewName := page.View
	if viewName == "" {
		viewName, err = g.ensureView(corpus, page.URL)
		if err != nil {
			return err
		}
		if corpus, err = sightmap.Load(g.Dir); err != nil {
			return fmt.Errorf("grow: reload corpus: %w", err)
		}
	}
	names := existingNames(corpus)
	bySelector := existingBySelector(corpus)
	parentMap := page.Result.Coverage.ParentMap
	if parentMap == nil {
		parentMap = coverage.BuildParentMap(page.Result.Root)
	}

	maxPer := g.MaxPerPage
	if maxPer <= 0 {
		maxPer = 40
	}
	groups := map[string]*group{}
	var order []string
	for i, n := range orphans {
		if i >= maxPer {
			break
		}
		_, hook := coverage.NearestHookAncestor(n.Raw, parentMap)
		gk := hook + "|" + n.Tag + "|" + n.Role
		gr, ok := groups[gk]
		if !ok {
			gr = &group{hook: hook, tag: n.Tag, role: n.Role}
			if n.Raw.Element != nil {
				gr.candidates = coverage.SelectorCandidates(n.Raw.Element)
			}
			groups[gk] = gr
			order = append(order, gk)
		}
		gr.members = append(gr.members, n)
	}

	written := 0
	for _, gk := range order {
		gr := groups[gk]
		sel, count := g.selectorFor(page, gr)
		if sel == "" {
			g.skipped += len(gr.members)
			continue
		}
		if dup, ok := bySelector[sel]; ok {
			if dup.view != "" && dup.view != viewName {
				if err := g.promoteToGlobal(dup); err == nil {
					bySelector[sel] = owner{name: dup.name}
				}
			}
			continue
		}
		kind, err := g.classify(ctx, gr)
		if err != nil {
			return err
		}
		if kind == "noise" {
			g.skipped += len(gr.members)
			continue
		}
		name := uniqueName(GroupName(gr.hook, gr.tag, gr.role, gr.members, kind, count), names)
		comp := component{
			Name:        name,
			Selector:    sel,
			Description: fmt.Sprintf("%s; auto-grown: %d %s in %s e.g. %s", kind, len(gr.members), gr.tag, orDefault(gr.hook, "page"), sampleNames(gr.members, 3)),
			Properties:  []property{{Name: "label", Extract: "text"}},
		}
		switch gr.tag {
		case "a":
			comp.Properties = append(comp.Properties, property{Name: "href", Extract: "attr=href"})
		case "input":
			comp.Properties = []property{{Name: "placeholder", Extract: "attr=placeholder"}}
		}
		if err := g.writeComponent(viewName, comp); err != nil {
			g.rejected++
			g.log("grow: rejected %s (%s): %v", name, sel, err)
			continue
		}
		names[name] = true
		bySelector[sel] = owner{name: name, view: viewName}
		g.added = append(g.added, Added{Name: name, View: viewName, Selector: sel, Kind: kind, Count: count})
		written++
	}
	g.log("grow: %d components added on %s (%d ms)", written, key, int(time.Since(t0).Milliseconds()))
	return nil
}

func orphansOf(page *explore.Page) []*explore.Node {
	var out []*explore.Node
	for _, n := range page.Nodes {
		if n.Interactive && n.Visible && n.Comp == "" && n.ParentComp == nil && n.Role != "image" {
			out = append(out, n)
		}
	}
	return out
}

// selectorFor tries the scoped container selector first, then a single node's
// own hooks, and returns the first one the offline matcher accepts on this page.
func (g *Grower) selectorFor(page *explore.Page, gr *group) (string, int) {
	var tries []string
	if gr.hook != "" {
		tries = append(tries, gr.hook+" "+gr.tag)
	}
	if len(gr.members) == 1 {
		m := gr.members[0]
		cands := append([]string{}, gr.candidates...)
		sort.SliceStable(cands, func(i, j int) bool { return Stability(cands[i]) > Stability(cands[j]) })
		if len(cands) > 3 {
			cands = cands[:3]
		}
		tries = append(tries, cands...)
		if m.Tag == "a" && m.Attrs["href"] != "" {
			tries = append(tries, fmt.Sprintf(`a[href="%s"]`, m.Attrs["href"]))
		}
		if v := m.Attrs["aria-label"]; v != "" {
			tries = append(tries, fmt.Sprintf(`%s[aria-label="%s"]`, m.Tag, v))
		}
		if v := m.Attrs["name"]; v != "" {
			tries = append(tries, fmt.Sprintf(`%s[name="%s"]`, m.Tag, v))
		}
	}
	for _, sel := range tries {
		sel = strings.TrimSpace(sel)
		if sel == "" || strings.Count(sel, `"`)%2 != 0 {
			continue
		}
		n := OfflineCount(page.Result.Root, sel)
		if n >= 1 && n <= 200 {
			return sel, n
		}
	}
	return "", 0
}

// OfflineCount reports how many nodes of root the offline matcher assigns to
// selector, which is exactly what snapshot, coverage, and capture will see.
func OfflineCount(root *sightmap.ComponentNode, selector string) int {
	defs := []sightmap.ComponentDef{{Name: "__grow__", Selectors: []string{selector}}}
	return len(match.NewMatcher(&sightmap.Corpus{GlobalComponents: defs}).Match(root, ""))
}

func (g *Grower) classify(ctx context.Context, gr *group) (string, error) {
	var examples []string
	for i, m := range gr.members {
		if i >= 5 {
			break
		}
		e := fmt.Sprintf("%q", truncate(m.Name, 40))
		if h := m.Attrs["href"]; h != "" {
			e += " → " + truncate(h, 40)
		}
		examples = append(examples, e)
	}
	desc := fmt.Sprintf("%d × <%s> role=%s inside %s; examples: %s", len(gr.members), gr.tag, gr.role, orDefault(gr.hook, "(no stable container)"), strings.Join(examples, ", "))
	g.calls++
	kind, err := g.Picker.Choose(ctx, "ELEMENTS: "+desc, explore.Criteria{Options: Kinds}, "What kind of UI elements are these? Answer noise if they are not worth naming as a component.")
	if err != nil {
		return "", fmt.Errorf("grow: classify: %w", err)
	}
	if kind == "" {
		kind = "noise"
	}
	return kind, nil
}

// owner records where a selector already lives in the corpus.
type owner struct {
	name string
	view string // "" for a global component
}

func existingNames(c *sightmap.Corpus) map[string]bool {
	names := map[string]bool{}
	for _, comp := range c.GlobalComponents {
		names[comp.Name] = true
	}
	for _, v := range c.Views {
		names[v.Name] = true
		for _, comp := range v.Components {
			names[comp.Name] = true
		}
	}
	return names
}

func existingBySelector(c *sightmap.Corpus) map[string]owner {
	out := map[string]owner{}
	for _, comp := range c.GlobalComponents {
		for _, s := range comp.Selectors {
			if _, ok := out[s]; !ok {
				out[s] = owner{name: comp.Name}
			}
		}
	}
	for _, v := range c.Views {
		for _, comp := range v.Components {
			if len(comp.ParentChain) > 0 {
				continue // a child's selector is scoped by its parent; not a site-wide duplicate
			}
			for _, s := range comp.Selectors {
				if _, ok := out[s]; !ok {
					out[s] = owner{name: comp.Name, view: v.Name}
				}
			}
		}
	}
	return out
}

func uniqueName(name string, existing map[string]bool) string {
	if !existing[name] {
		return name
	}
	for i := 2; ; i++ {
		cand := fmt.Sprintf("%s%d", name, i)
		if !existing[cand] {
			return cand
		}
	}
}

// ensureView returns the name of the view whose route pattern covers pageURL,
// creating views/<name>.yaml when none does.
func (g *Grower) ensureView(corpus *sightmap.Corpus, pageURL string) (string, error) {
	route := RoutePattern(pageURL)
	for _, v := range corpus.Views {
		if v.Route == route {
			return v.Name, nil
		}
	}
	name := uniqueName(RouteName(route), existingNames(corpus))
	u, err := url.Parse(pageURL)
	if err != nil {
		return "", err
	}
	file := filepath.Join(g.Dir, "views", strings.ToLower(name)+".yaml")
	if _, err := os.Stat(file); err == nil {
		file = filepath.Join(g.Dir, "views", strings.ToLower(name)+"-"+fmt.Sprint(time.Now().Unix())+".yaml")
	}
	body := fmt.Sprintf("version: 1\nviews:\n  - name: %s\n    route: %q\n    url: %s\n    description: auto-grown view for %s\n    components: []\n", name, route, u.Scheme+"://"+u.Host+u.Path, route)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		return "", err
	}
	if err := g.reload(); err != nil {
		os.Remove(file)
		return "", fmt.Errorf("grow: new view %s rejected: %w", name, err)
	}
	g.added = append(g.added, Added{Name: name, View: name, Kind: "view"})
	return name, nil
}

// reload validates the corpus after a write, through the caller's Reload when
// set, otherwise by loading and validating it here.
func (g *Grower) reload() error {
	if g.Reload != nil {
		return g.Reload()
	}
	c, err := sightmap.Load(g.Dir)
	if err != nil {
		return err
	}
	if errs := sightmap.Validate(c); len(errs) > 0 {
		return fmt.Errorf("%d validation errors: %v", len(errs), errs[0])
	}
	return nil
}

func sampleNames(members []*explore.Node, k int) string {
	var parts []string
	for i, m := range members {
		if i >= k {
			break
		}
		parts = append(parts, fmt.Sprintf("%q", truncate(m.Name, 30)))
	}
	return strings.Join(parts, ", ")
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
