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
	"os"
	"path/filepath"
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
	Inside     int            `json:"inside"`     // visible nodes under a named ancestor (not named themselves)
	Components map[string]int `json:"components"` // component name -> visible nodes it names, containers included
	Unmatched  []matchGap     `json:"unmatched"`  // regions nothing names or encloses
	Conflicts  []matchClash   `json:"conflicts"`  // nodes claimed by more than one definition
	Error      string         `json:"error,omitempty"`
}

// matchGap is a region of nodes with no component on or above them, keyed by
// the nearest ancestor that carries a test hook (or the page root).
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
	fs := flag.NewFlagSet("match", flag.ContinueOnError)
	sightmapDir := fs.String("sightmap-dir", ".sightmap", "Path to the .sightmap/ dir whose corpus is matched")
	includeHidden := fs.Bool("include-hidden", false, "Count hidden and AX-ignored nodes too")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: sightmap match [--sightmap-dir DIR] [--include-hidden] FILE.snap|DIR ...\n\n"+
			"Matches each capture against the corpus by its URL (not the route header a\n"+
			"capture was filed with), as a session pipeline would, and prints one JSON\n"+
			"record per line: view (or tiedViews), components matched, unmatched regions\n"+
			"by nearest hooked ancestor, and nodes several definitions claim.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return fmt.Errorf("match: no captures given")
	}
	corpus, err := sightmap.Load(*sightmapDir)
	if err != nil {
		return fmt.Errorf("match: load corpus: %w", err)
	}
	var snaps []string
	for _, a := range fs.Args() {
		info, err := os.Stat(a)
		if err != nil {
			return fmt.Errorf("match: %w", err)
		}
		if !info.IsDir() {
			snaps = append(snaps, a)
			continue
		}
		_ = filepath.Walk(a, func(p string, fi os.FileInfo, err error) error {
			if err == nil && !fi.IsDir() && strings.HasSuffix(p, ".snap") {
				snaps = append(snaps, p)
			}
			return nil
		})
	}
	sort.Strings(snaps)
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

func matchOne(snap string, corpus *sightmap.Corpus, m *match.Matcher, visibleOnly bool) matchRecord {
	rec := matchRecord{Capture: snap, URL: viewset.URLOf(snap), Components: map[string]int{}}
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
	namedUnder := func(n *sightmap.ComponentNode) bool {
		for p := parents[n]; p != nil; p = parents[p] {
			if matches[p] != nil {
				return true
			}
		}
		return false
	}
	gaps := map[string]*matchGap{}
	sightmap.Walk(root, func(n *sightmap.ComponentNode, _ int) bool {
		if n == root {
			return true
		}
		if visibleOnly && (!n.IsVisible || n.IsIgnored) {
			return true
		}
		mm := matches[n]
		if mm != nil {
			// Every visible node a component names counts toward its presence,
			// containers included: a header or panel is matched even when it
			// holds no text of its own.
			rec.Components[mm.Name]++
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
		if namedUnder(n) {
			rec.Inside++
			return true
		}
		anchor := "(page)"
		if a := coverage.NearestDataAttrAncestor(n, parents); a != nil {
			anchor = coverage.DataAttrSelector(a)
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
				if len(name) > 40 {
					name = name[:40] + "…"
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
	if rec.Unmatched == nil {
		rec.Unmatched = []matchGap{}
	}
	if rec.Conflicts == nil {
		rec.Conflicts = []matchClash{}
	}
	return rec
}
