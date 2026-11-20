// match runs saved captures through the corpus the way a session-matching
// pipeline would and prints one JSON record per capture: the view its URL
// resolves to (none, or several tied), how many nodes each component named,
// the interactive and content nodes nothing names, grouped by their nearest
// hooked ancestor, and the nodes more than one definition claims. It is the
// per-session signal a sampler reads to decide what to capture next: unmatched
// regions are gaps, conflicts are ambiguities, components never matched across
// many records are candidates to retire.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/sightmap/sightmap/go/coverage"
	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
	"github.com/sightmap/sightmap/go/viewset"
)

// matchRecord is one capture's match result. Field names are the JSON contract.
type matchRecord struct {
	Capture    string         `json:"capture"`
	URL        string         `json:"url"`
	View       string         `json:"view,omitempty"` // "" when no view's route matches
	TiedViews  []string       `json:"tiedViews,omitempty"`
	Nodes      int            `json:"nodes"`      // visible nodes considered
	Named      int            `json:"named"`      // visible nodes a component names directly
	Inside     int            `json:"inside"`     // visible nodes whose nearest named ancestor holds <= 20 nodes
	Components map[string]int `json:"components"` // component name -> nodes it names anywhere in the DOM
	Unmatched  []matchGap     `json:"unmatched"`  // regions nothing names or encloses
	Conflicts  []matchClash   `json:"conflicts"`  // nodes claimed by more than one definition
	Error      string         `json:"error,omitempty"`
}

// matchGap is a region of nodes no small component names, keyed by the nearest
// ancestor that carries a test hook (or the page root), prefixed with the large
// component enclosing them when there is one ("MainContent > thead[...]").
type matchGap struct {
	Anchor      string   `json:"anchor"`
	Interactive int      `json:"interactive"`
	Content     int      `json:"content"` // non-interactive nodes with an accessible name
	Samples     []string `json:"samples,omitempty"`
}

type matchClash struct {
	Node  string   `json:"node"`
	Names []string `json:"names"`
}

func runMatch(args []string) error { return runMatchOut(args, os.Stdout) }

func runMatchOut(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("match", flag.ContinueOnError)
	sightmapDir := flags.String("sightmap-dir", ".sightmap", "Path to the .sightmap/ dir whose corpus is matched")
	includeHidden := flags.Bool("include-hidden", false, "Count hidden and AX-ignored nodes too")
	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: sightmap match [--sightmap-dir DIR] [--include-hidden] FILE.snap|DIR ...\n\n"+
			"Matches each capture against the corpus by its URL (not the route header a\n"+
			"capture was filed with), as a session pipeline would, and prints one JSON\n"+
			"record per line: view (or tiedViews), components matched, unmatched regions\n"+
			"by nearest hooked ancestor, and nodes several definitions claim.\n\n")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() == 0 {
		flags.Usage()
		return fmt.Errorf("match: no captures given")
	}
	corpus, err := sightmap.Load(*sightmapDir)
	if err != nil {
		return fmt.Errorf("match: load corpus: %w", err)
	}
	var snaps []string
	for _, a := range flags.Args() {
		info, err := os.Stat(a)
		if err != nil {
			return fmt.Errorf("match: %w", err)
		}
		if !info.IsDir() {
			snaps = append(snaps, a)
			continue
		}
		err = filepath.WalkDir(a, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(p, ".snap") {
				snaps = append(snaps, p)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("match: %w", err)
		}
	}
	if len(snaps) == 0 {
		return fmt.Errorf("match: no .snap captures found")
	}
	slices.Sort(snaps)
	m := match.NewMatcher(corpus)
	enc := json.NewEncoder(out)
	for _, s := range snaps {
		if err := enc.Encode(matchOne(s, corpus, m, !*includeHidden)); err != nil {
			return err
		}
	}
	return nil
}

const matchGapSamples = 3

// matchSmallComponent is the most nodes a named ancestor may hold for the nodes
// under it to count as named by it. A larger one is a page-level container
// that encloses nearly everything and says nothing about what is under it, so
// those nodes are reported unmatched with its name in the anchor.
const matchSmallComponent = 20

func matchOne(snap string, corpus *sightmap.Corpus, m *match.Matcher, visibleOnly bool) matchRecord {
	rec := matchRecord{Capture: snap, URL: viewset.URLOf(snap), Components: map[string]int{},
		Unmatched: []matchGap{}, Conflicts: []matchClash{}}
	data, err := os.ReadFile(snap + ".tree.json")
	if err != nil {
		rec.Error = err.Error()
		return rec
	}
	root := &sightmap.ComponentNode{}
	if err := json.Unmarshal(data, root); err != nil {
		rec.Error = err.Error()
		return rec
	}
	if rec.URL == "" {
		rec.Error = "capture has no url: header"
		return rec
	}
	if v := corpus.ViewForURL(rec.URL); v != nil {
		rec.View = v.Name
	}
	if tied := corpus.TiedViews(rec.URL); len(tied) > 1 {
		rec.TiedViews = tied
	}
	matches := m.Match(root, rec.URL)
	parents := coverage.BuildParentMap(root)
	// A node counts as inside a component only when its nearest named ancestor
	// is small enough to say what the node is: a page-wide container
	// (MainContent, a layout pane) encloses everything and names nothing.
	size := map[*sightmap.ComponentNode]int{}
	var count func(n *sightmap.ComponentNode) int
	count = func(n *sightmap.ComponentNode) int {
		c := 1
		for _, k := range n.Children {
			c += count(k)
		}
		size[n] = c
		return c
	}
	count(root)
	nearestNamed := func(n *sightmap.ComponentNode) *sightmap.ComponentNode {
		for p := parents[n]; p != nil; p = parents[p] {
			if matches[p] != nil {
				return p
			}
		}
		return nil
	}
	gaps := map[string]*matchGap{}
	sightmap.Walk(root, func(n *sightmap.ComponentNode, _ int) bool {
		if n == root {
			return true
		}
		mm := matches[n]
		if mm != nil {
			// Every node a component names counts toward its presence, hidden
			// and AX-ignored ones and containers included: an icon is usually
			// AX-ignored, a closed menu is in the DOM but hidden, and a header
			// holds no text of its own; all of them are still matched.
			rec.Components[mm.Name]++
		}
		if visibleOnly && (!n.IsVisible || n.IsIgnored) {
			return true
		}
		content := !n.IsInteractive && strings.TrimSpace(n.Name) != ""
		if !n.IsInteractive && !content {
			return true // structure: neither touched nor read
		}
		rec.Nodes++
		if mm != nil {
			rec.Named++
			return true
		}
		holder := nearestNamed(n)
		if holder != nil && size[holder] <= matchSmallComponent {
			rec.Inside++
			return true
		}
		anchor := "(page)"
		if a := coverage.NearestDataAttrAncestor(n, parents); a != nil {
			anchor = coverage.DataAttrSelector(a)
		}
		if holder != nil {
			anchor = matches[holder].Name + " > " + anchor
		}
		g := gaps[anchor]
		if g == nil {
			g = &matchGap{Anchor: anchor}
			gaps[anchor] = g
		}
		if n.IsInteractive {
			g.Interactive++
		} else {
			g.Content++
		}
		if len(g.Samples) < matchGapSamples {
			label := n.Role
			if name := strings.TrimSpace(n.Name); name != "" {
				if r := []rune(name); len(r) > 40 {
					name = string(r[:40]) + "…"
				}
				label += fmt.Sprintf(" %q", name)
			}
			g.Samples = append(g.Samples, label)
		}
		return true
	})
	for _, g := range gaps {
		rec.Unmatched = append(rec.Unmatched, *g)
	}
	sort.Slice(rec.Unmatched, func(i, j int) bool {
		a, b := rec.Unmatched[i], rec.Unmatched[j]
		if sa, sb := a.Interactive+a.Content, b.Interactive+b.Content; sa != sb {
			return sa > sb
		}
		return a.Anchor < b.Anchor
	})
	for _, c := range m.Conflicts(root, rec.URL) {
		node := c.Node.Role
		if c.Node.Element != nil {
			node = c.Node.Element.SelectorString()
		}
		rec.Conflicts = append(rec.Conflicts, matchClash{Node: node, Names: c.Names})
	}
	return rec
}
