package explore

import (
	"context"
	"fmt"
	"strings"

	"github.com/sightmap/sightmap/go/sightmap"
)

// mk builds a Node for tests. attrs is "k=v,k=v".
func mk(id, role, name, tag, attrs, comp string, interactive bool) *Node {
	n := &Node{ID: id, Role: role, Name: name, Tag: tag, Comp: comp, Interactive: interactive, Visible: true, InViewport: true, Attrs: map[string]string{}, Props: map[string]string{}}
	for _, kv := range strings.Split(attrs, ",") {
		if k, v, ok := strings.Cut(kv, "="); ok {
			n.Attrs[k] = v
		}
	}
	n.Raw = &sightmap.ComponentNode{Id: id, Role: role, Name: name, IsInteractive: interactive, IsVisible: true, Element: &sightmap.Element{Tag: tag, Attrs: n.Attrs}}
	return n
}

func withProps(n *Node, kv ...string) *Node {
	for i := 0; i+1 < len(kv); i += 2 {
		n.Props[kv[i]] = kv[i+1]
	}
	return n
}

func under(parent *Node, children ...*Node) {
	for _, c := range children {
		c.Parent = parent
		c.Ancestors = append(append([]*Node{}, parent.Ancestors...), parent)
		if parent.Comp != "" {
			c.ParentComp = parent
		} else {
			c.ParentComp = parent.ParentComp
		}
		if landmarkRoles[parent.Role] {
			c.Landmark = parent
		} else {
			c.Landmark = parent.Landmark
		}
	}
}

// fakePage is one state of the fake site.
type fakePage struct {
	url   string
	view  string
	nodes []*Node
	// edges maps a node id to the URL a click leads to.
	edges map[string]string
}

// fakeDriver plays a tiny site: each page lists nodes and where clicks go.
type fakeDriver struct {
	pages   map[string]*fakePage
	cur     string
	clicks  []string
	fills   map[string]string
	failIDs map[string]int // click on this id fails this many times with a stale error
	scrolls int
}

func newFakeDriver(start string, pages ...*fakePage) *fakeDriver {
	d := &fakeDriver{pages: map[string]*fakePage{}, cur: start, fills: map[string]string{}, failIDs: map[string]int{}}
	for _, p := range pages {
		d.pages[p.url] = p
	}
	return d
}

func (d *fakeDriver) page() *fakePage { return d.pages[d.cur] }

func (d *fakeDriver) Observe(ctx context.Context) (*Page, error) {
	p := d.page()
	if p == nil {
		return nil, fmt.Errorf("no page at %s", d.cur)
	}
	return &Page{URL: p.url, View: p.view, Nodes: p.nodes}, nil
}
func (d *fakeDriver) URL(ctx context.Context) (string, error) { return d.cur, nil }
func (d *fakeDriver) Click(ctx context.Context, n *Node) error {
	if d.failIDs[n.ID] > 0 {
		d.failIDs[n.ID]--
		return fmt.Errorf("element %s not found in live DOM", n.ID)
	}
	d.clicks = append(d.clicks, n.ID)
	if to, ok := d.page().edges[n.ID]; ok {
		d.cur = to
	}
	return nil
}
func (d *fakeDriver) Fill(ctx context.Context, n *Node, v string) error {
	d.fills[n.ID] = v
	return nil
}
func (d *fakeDriver) SelectOptions(ctx context.Context, n *Node) ([]string, error) {
	return []string{"Name (A to Z)", "Price (low to high)"}, nil
}
func (d *fakeDriver) Select(ctx context.Context, n *Node, i int) error {
	d.fills[n.ID] = fmt.Sprintf("option %d", i)
	return nil
}
func (d *fakeDriver) Back(ctx context.Context) error   { d.clicks = append(d.clicks, "back"); return nil }
func (d *fakeDriver) Scroll(ctx context.Context) error { d.scrolls++; return nil }
func (d *fakeDriver) Settle(ctx context.Context, before string) SettleInfo {
	return SettleInfo{URL: d.cur, Navigated: d.cur != before, Ms: 1}
}
func (d *fakeDriver) Navigate(ctx context.Context, u string) error { d.cur = u; return nil }
func (d *fakeDriver) ClearStorage(ctx context.Context) error       { return nil }

// fakePicker answers picks from a script (each entry used once, in order) and
// then from sticky preferences; either matches an option by key or by a
// substring of its description. With nothing matching it takes the first option.
type fakePicker struct {
	script  []string
	prefer  []string
	done    float64
	calls   int
	chooses []string
	picks   []string
}

func (p *fakePicker) Name() string { return "fake" }
func (p *fakePicker) Pick(ctx context.Context, state string, crit Criteria) (Pick, error) {
	p.calls++
	if len(p.script) > 0 {
		want := p.script[0]
		for _, o := range crit.Options {
			if strings.Contains(o.Desc, want) || o.Key == want {
				p.script = p.script[1:]
				p.picks = append(p.picks, o.Key)
				return Pick{Next: o.Key, Done: p.done, Probs: map[string]float64{o.Key: 1}}, nil
			}
		}
	}
	for _, want := range p.prefer {
		for _, o := range crit.Options {
			if strings.Contains(o.Desc, want) || o.Key == want {
				p.picks = append(p.picks, o.Key)
				return Pick{Next: o.Key, Done: p.done, Probs: map[string]float64{o.Key: 1}}, nil
			}
		}
	}
	k := crit.Options[0].Key
	p.picks = append(p.picks, k)
	return Pick{Next: k, Done: p.done, Probs: map[string]float64{k: 1}}, nil
}
func (p *fakePicker) Choose(ctx context.Context, state string, crit Criteria, instructions string) (string, error) {
	p.chooses = append(p.chooses, instructions)
	for _, want := range p.prefer {
		for _, o := range crit.Options {
			if strings.Contains(o.Desc, want) || o.Key == want {
				return o.Key, nil
			}
		}
	}
	return crit.Options[0].Key, nil
}
func (p *fakePicker) Stats() Stats { return Stats{Calls: p.calls} }
