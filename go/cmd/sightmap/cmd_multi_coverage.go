package main

import (
	"flag"
	"fmt"
	"sort"
	"strings"

	"github.com/sightmap/sightmap/go/sightmap"
)

// runMultiCoverage shows a cross-page component coverage matrix and surfaces
// global promotion candidates. Each matrix column is a VIEW, not a single capture
// file: a view is a SET of timestamped captures (go-snap.6/.10), and each cell is
// the MAX matched count across that view's set (go-snap.8). Grouping by view keeps
// the matrix one-column-per-page even when a view carries many captures, and makes
// "global candidate" mean "appears on 2+ pages" rather than "appears in 2+ files".
func runMultiCoverage(args []string) error {
	fs := flag.NewFlagSet("multi-coverage", flag.ContinueOnError)
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir")
	if err := fs.Parse(args); err != nil {
		return err
	}

	snapFiles, err := findSnapshots(*sightmapDirFlag, fs.Args())
	if err != nil {
		return err
	}

	loader := sightmap.DirLoader(*sightmapDirFlag)
	sess := sightmap.NewSession(loader)

	// Load corpus directly to check which names are already global.
	corpus, err := loader.Load()
	if err != nil {
		return fmt.Errorf("load corpus: %v", err)
	}
	globalNames := make(map[string]bool, len(corpus.GlobalComponents))
	for _, gc := range corpus.GlobalComponents {
		globalNames[gc.Name] = true
	}

	// Group captures into per-view sets, then fold each view's captures into one
	// column whose cell value is the per-component MAX across the set.
	sets := groupSnapshotsByView(snapFiles)
	viewNames := make([]string, 0, len(sets))
	for v := range sets {
		viewNames = append(viewNames, v)
	}
	sort.Strings(viewNames)

	var cols []viewColumn
	for _, v := range viewNames {
		var caps []map[string]int
		for _, e := range sets[v] {
			_, matches, _, ok := matchCapture(e.Path, sess)
			if !ok {
				continue
			}
			counts := make(map[string]int)
			for _, m := range matches {
				if m != nil {
					counts[m.Name]++
				}
			}
			caps = append(caps, counts)
		}
		if len(caps) == 0 {
			continue
		}
		cols = append(cols, unionViewColumn(v, caps))
	}

	if len(cols) == 0 {
		return fmt.Errorf("no tree files found")
	}

	printMatrix(cols)
	printGlobalCandidates(globalCandidatesAcrossViews(cols, globalNames))
	return nil
}

// viewColumn is one matrix column in multi-coverage: a view and the per-component
// MAX matched count across that view's capture set (go-snap.8). Snaps is the set
// size, surfaced in the column header (e.g. "home·3") so the reader can tell a
// stable single-capture view from a unioned multi-capture one.
type viewColumn struct {
	View   string
	Snaps  int
	Counts map[string]int
}

// label renders the column header for a view: "view·N" when the view carries more
// than one capture, else just the view name.
func (c viewColumn) label() string {
	if c.Snaps > 1 {
		return fmt.Sprintf("%s\u00b7%d", c.View, c.Snaps)
	}
	return c.View
}

// unionViewColumn folds a view's per-capture component counts into one column,
// taking the MAX count per component across the set (union-honest "renders up to
// K of these"). caps is one component→count map per capture.
func unionViewColumn(view string, caps []map[string]int) viewColumn {
	col := viewColumn{View: view, Snaps: len(caps), Counts: map[string]int{}}
	for _, c := range caps {
		for name, n := range c {
			if n > col.Counts[name] {
				col.Counts[name] = n
			}
		}
	}
	return col
}

// printMatrix renders the Component × view matrix; `-` marks a component the view
// never matched in any capture (union-dead for that view).
func printMatrix(cols []viewColumn) {
	// All component names across all views, sorted.
	nameSet := map[string]bool{}
	for _, c := range cols {
		for name := range c.Counts {
			nameSet[name] = true
		}
	}
	names := make([]string, 0, len(nameSet))
	for name := range nameSet {
		names = append(names, name)
	}
	sort.Strings(names)

	// Column widths: each view column is max(len(label)+2, 6).
	colWidths := make([]int, len(cols))
	for i, c := range cols {
		w := len(c.label()) + 2
		if w < 6 {
			w = 6
		}
		colWidths[i] = w
	}

	nameColWidth := len("Component")
	for _, name := range names {
		if len(name) > nameColWidth {
			nameColWidth = len(name)
		}
	}

	header := fmt.Sprintf("%-*s", nameColWidth, "Component")
	for i, c := range cols {
		header += fmt.Sprintf("%*s", colWidths[i], c.label())
	}
	fmt.Println(header)

	totalWidth := nameColWidth
	for _, w := range colWidths {
		totalWidth += w
	}
	fmt.Println(strings.Repeat("─", totalWidth))

	for _, name := range names {
		row := fmt.Sprintf("%-*s", nameColWidth, name)
		for i, c := range cols {
			if count := c.Counts[name]; count == 0 {
				row += fmt.Sprintf("%*s", colWidths[i], "-")
			} else {
				row += fmt.Sprintf("%*d", colWidths[i], count)
			}
		}
		fmt.Println(row)
	}
}

// viewHit is one (view, max-count) cell where a global candidate appeared.
type viewHit struct {
	View  string
	Count int
}

// globalCandidate is a component that appears in 2+ views and isn't yet global.
type globalCandidate struct {
	Name string
	Hits []viewHit
}

// globalCandidatesAcrossViews returns components that match (count > 0) in 2 or
// more VIEWS and aren't already global. Counting views (not capture files) is the
// go-snap.8 fix: a single view snapped many times no longer trips the threshold.
// Result is sorted by name; each candidate's hits follow column order.
func globalCandidatesAcrossViews(cols []viewColumn, globalNames map[string]bool) []globalCandidate {
	nameSet := map[string]bool{}
	for _, c := range cols {
		for name := range c.Counts {
			nameSet[name] = true
		}
	}
	names := make([]string, 0, len(nameSet))
	for name := range nameSet {
		names = append(names, name)
	}
	sort.Strings(names)

	var candidates []globalCandidate
	for _, name := range names {
		if globalNames[name] {
			continue
		}
		var hits []viewHit
		for _, c := range cols {
			if count := c.Counts[name]; count > 0 {
				hits = append(hits, viewHit{View: c.View, Count: count})
			}
		}
		if len(hits) >= 2 {
			candidates = append(candidates, globalCandidate{Name: name, Hits: hits})
		}
	}
	return candidates
}

// printGlobalCandidates prints the promotion-candidate block (nothing if empty).
func printGlobalCandidates(candidates []globalCandidate) {
	if len(candidates) == 0 {
		return
	}
	fmt.Println()
	fmt.Println("Global candidates (appear in 2+ views, not yet in global components):")
	maxCandWidth := 0
	for _, c := range candidates {
		if len(c.Name) > maxCandWidth {
			maxCandWidth = len(c.Name)
		}
	}
	for _, c := range candidates {
		var parts []string
		for _, h := range c.Hits {
			parts = append(parts, fmt.Sprintf("%s(%d)", h.View, h.Count))
		}
		fmt.Printf("  %-*s  %s   → add to components.yaml\n",
			maxCandWidth, c.Name, strings.Join(parts, " "))
	}
}
