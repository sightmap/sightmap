package main

import (
	"flag"
	"fmt"
	"sort"
	"strings"

	"github.com/sightmap/sightmap/go/sightmap"
	"github.com/sightmap/sightmap/go/viewset"
)

// runMultiCoverage shows a cross-page component coverage matrix and surfaces
// global promotion candidates. Each matrix column is a VIEW, not a single capture
// file: a view is a SET of timestamped captures, and each cell is
// the MAX matched count across that view's set. Grouping by view keeps
// the matrix one-column-per-page even when a view carries many captures, and makes
// "global candidate" mean "appears on 2+ pages" rather than "appears in 2+ files".
func runMultiCoverage(args []string) error {
	fs := flag.NewFlagSet("multi-coverage", flag.ContinueOnError)
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir")
	if err := fs.Parse(args); err != nil {
		return err
	}

	snapFiles, err := viewset.Find(*sightmapDirFlag, fs.Args())
	if err != nil {
		return err
	}

	corpus, err := sightmap.Load(*sightmapDirFlag)
	if err != nil {
		return fmt.Errorf("load corpus: %v", err)
	}
	globalNames := make(map[string]bool, len(corpus.GlobalComponents))
	for _, gc := range corpus.GlobalComponents {
		globalNames[gc.Name] = true
	}

	// A capture dir is only a real VIEW if it matches a view in the CURRENT
	// corpus (by SnapBasename, the name capture writes the dir under). A stale or
	// renamed dir — snapshots/<old-name>/ left over after the view was renamed —
	// otherwise becomes a phantom extra column and manufactures bogus "appears in
	// 2+ views" promotion evidence for that page's own components.
	currentBasenames := make(map[string]bool, len(corpus.Views))
	for i := range corpus.Views {
		currentBasenames[corpus.Views[i].SnapBasename()] = true
	}

	// Group captures into per-view sets, then fold each view's captures into one
	// column whose cell value is the per-component MAX across the set.
	sets := viewset.GroupByView(snapFiles)
	viewNames := make([]string, 0, len(sets))
	for v := range sets {
		viewNames = append(viewNames, v)
	}
	sort.Strings(viewNames)

	var cols []viewColumn
	for _, v := range viewNames {
		var caps []map[string]int
		recorded := ""
		for _, e := range sets[v] {
			if recorded == "" {
				recorded = viewset.ViewNameOf(e.Path)
			}
			_, matches, _, ok := matchCapture(e.Path, corpus)
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
		col := unionViewColumn(v, caps)
		col.Current = currentBasenames[v]
		col.Recorded = recorded
		cols = append(cols, col)
	}

	if len(cols) == 0 {
		return fmt.Errorf("no tree files found")
	}

	printMatrix(cols)
	printGlobalCandidates(globalCandidatesAcrossViews(cols, globalNames))
	printStaleColumns(cols)
	return nil
}

// viewColumn is one matrix column in multi-coverage: a view and the per-component
// MAX matched count across that view's capture set. Snaps is the set
// size, surfaced in the column header (e.g. "home·3") so the reader can tell a
// stable single-capture view from a unioned multi-capture one.
type viewColumn struct {
	View   string
	Snaps  int
	Counts map[string]int
	// Current is true when this capture dir maps to a view in the current corpus
	// (by SnapBasename). Only current columns count toward global promotion; a
	// non-current column is a stale/renamed dir shown for context but excluded.
	Current bool
	// Recorded is the "[View: NAME]" header the captures carry, surfaced only in
	// the stale-column warning to help the author identify what the dir was.
	Recorded string
}

// label renders the column header for a view: "view·N" when the view carries more
// than one capture, else just the view name. A non-current (stale/renamed) dir
// is suffixed with "*", tying it to the warning printed below the matrix.
func (c viewColumn) label() string {
	name := c.View
	if c.Snaps > 1 {
		name = fmt.Sprintf("%s\u00b7%d", c.View, c.Snaps)
	}
	if !c.Current {
		name += "*"
	}
	return name
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
// more CURRENT VIEWS and aren't already global. Counting views (not capture
// files) means a single view snapped many times no longer trips the threshold;
// restricting to CURRENT columns (Current == true) means a stale or renamed
// capture dir can't manufacture phantom cross-view evidence for a page's own
// components. Result is sorted by name; each candidate's hits follow column order.
func globalCandidatesAcrossViews(cols []viewColumn, globalNames map[string]bool) []globalCandidate {
	nameSet := map[string]bool{}
	for _, c := range cols {
		if !c.Current {
			continue
		}
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
			if !c.Current {
				continue
			}
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

// staleColumns returns the columns that don't map to a current corpus view, in
// column order. These are the phantom-evidence sources #248 is about.
func staleColumns(cols []viewColumn) []viewColumn {
	var stale []viewColumn
	for _, c := range cols {
		if !c.Current {
			stale = append(stale, c)
		}
	}
	return stale
}

// printStaleColumns warns about capture dirs that match no current view. They
// still appear in the matrix (marked "*") for context, but are excluded from the
// global-candidate analysis so a stale or renamed dir can't mis-advise a
// promotion. Prints nothing when every column maps to a current view.
func printStaleColumns(cols []viewColumn) {
	stale := staleColumns(cols)
	if len(stale) == 0 {
		return
	}
	fmt.Println()
	fmt.Printf("⚠ %d capture dir(s) match no view in the current corpus (stale or renamed?) —\n", len(stale))
	fmt.Println("  marked \"*\" above and EXCLUDED from the global-candidate analysis:")
	for _, c := range stale {
		recorded := ""
		if c.Recorded != "" {
			recorded = fmt.Sprintf(" (recorded [View: %s])", c.Recorded)
		}
		fmt.Printf("  %s%s   → delete the stale dir, or re-capture the view under its current name\n", c.View, recorded)
	}
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
