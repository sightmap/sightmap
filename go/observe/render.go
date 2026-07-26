// Package observe acquires and renders an annotated snapshot of a live page: it
// runs the component-extraction pipeline, applies a corpus, extracts property
// values, and scores coverage (observe.Page), then formats the result as the
// annotated tree + section output (observe.Format). It is the shared read path
// behind the snapshot and capture commands, and the entrypoint external callers
// use to observe a page against a corpus.
package observe

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/coverage"
	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/render"
	"github.com/sightmap/sightmap/go/sightmap"
)

// FormatOpts carries the display toggles for Format.
type FormatOpts struct {
	Interactive  bool // show interactive nodes only
	MaxDepth     int  // max tree depth (0 = unlimited)
	Trace        bool // include unlabeled-cluster / T2-scope traces
	CoverageOnly bool // suppress the guide + tree; show View/Coverage/traces only
	Selectors    bool // show tag #id [data-testid] selector hints
}

// Format writes the annotated snapshot sections (View, Guide, tree, Warnings,
// Coverage) for a Result to w. FormatOpts carries only display toggles; all data
// comes from the Result.
func Format(w io.Writer, r *Result, opts FormatOpts) {
	// ── [View: ...] ──────────────────────────────────────────────────────────
	if r.View != nil {
		fmt.Fprintf(w, "[View: %s]\n", r.View.Name)
		fmt.Fprintf(w, "route: %s\n", r.View.Route)
		if len(r.View.Memory) > 0 {
			fmt.Fprintf(w, "memory:\n")
			for _, m := range r.View.Memory {
				fmt.Fprintf(w, "  - %s\n", m)
			}
		}
		fmt.Fprintln(w)
	}

	// ── [Guide] ───────────────────────────────────────────────────────────────
	// Skipped in coverage-only mode, which is meant to be a terse feedback r.View.
	if !opts.CoverageOnly && len(r.Matches) > 0 {
		writeGuide(w, r.Matches)
		fmt.Fprintln(w)
	}

	// ── component tree ────────────────────────────────────────────────────────
	// Coverage-only mode suppresses the (potentially huge) tree entirely.
	if !opts.CoverageOnly {
		fmt.Fprintf(w, "--- component tree ---\n\n")
		if r.Root != nil {
			comp := render.Filter(r.Root, r.Matches)
			if comp != nil {
				comp.Format(w, "", render.FormatOpts{
					Selectors:   opts.Selectors,
					MaxDepth:    opts.MaxDepth,
					Interactive: opts.Interactive,
					PropValues:  r.Props,
				})
			}
		}
	}

	// ── [Warnings] ────────────────────────────────────────────────────────────
	if len(r.Components) > 0 && r.Matches != nil {
		writeZeroMatchWarnings(w, r.Components, r.Matches, r.View, r.GlobalNames)
	}

	// ── [Coverage] ────────────────────────────────────────────────────────────
	if r.Matches != nil {
		fmt.Fprintln(w)
		cov := r.Coverage
		writeCoverage(w, cov)

		// ── Unlabeled clusters (--trace) ─────────────────────────────────────
		// Coverage-only mode always shows the cluster traces — they are the point
		// of the terse r.View (which orphans/ambiguous clusters still need work).
		if opts.Trace || opts.CoverageOnly {
			if len(cov.Orphans) > 0 {
				fmt.Fprintln(w)
				WriteTrace(w, cov.Orphans, cov.ParentMap)
			}
			if len(cov.Scopes) > 0 {
				fmt.Fprintln(w)
				WriteT2Trace(w, cov.Scopes, cov.ScopeChildren, r.Matches, r.View)
			}
		}
	}
}

// writeGuide prints the [Guide] section: component names and match counts,
// sorted by count descending then name ascending.
func writeGuide(w io.Writer, matches map[*comps.ComponentNode]*match.SightmapMatch) {
	counts := make(map[string]int)
	for _, m := range matches {
		if m != nil {
			counts[m.Name]++
		}
	}

	type nc struct {
		name  string
		count int
	}
	guide := make([]nc, 0, len(counts))
	for name, cnt := range counts {
		guide = append(guide, nc{name, cnt})
	}
	sort.Slice(guide, func(i, j int) bool {
		if guide[i].count != guide[j].count {
			return guide[i].count > guide[j].count
		}
		return guide[i].name < guide[j].name
	})
	if len(guide) == 0 {
		return
	}

	maxNameLen := 0
	for _, g := range guide {
		if len(g.name) > maxNameLen {
			maxNameLen = len(g.name)
		}
	}
	countWidth := len(fmt.Sprintf("%d", guide[0].count))
	nameWidth := maxNameLen + 2 // minimum two-space gap before count column

	fmt.Fprintf(w, "[Guide]\n")
	for _, g := range guide {
		fmt.Fprintf(w, "%-*s%*d\n", nameWidth, g.name, countWidth, g.count)
	}
}

// writeZeroMatchWarnings emits a [Warnings] section listing view components
// that matched 0 nodes in the extracted tree, and a [Snapshot check] line
// when every top-level component missed \u2014 a strong signal for wrong-page capture.
// globalNames is the set of corpus-level global component names; these are
// excluded from the page-check since they match on any page.
func writeZeroMatchWarnings(
	w io.Writer,
	viewComponents []match.SightmapComponent,
	matches map[*comps.ComponentNode]*match.SightmapMatch,
	view *sightmap.View,
	globalNames map[string]bool,
) {
	// Count matches per component name.
	counts := make(map[string]int)
	for _, m := range matches {
		if m != nil {
			counts[m.Name]++
		}
	}

	viewName := ""
	if view != nil {
		viewName = view.Name
	}

	// Page-check: if all top-level view-specific components (excluding globals)
	// matched 0 nodes, the snapshot likely captured the wrong page.
	if view != nil {
		CheckRootComponents(w, view.Components, globalNames, counts, viewName)
	}

	var warnings []string
	for _, comp := range viewComponents {
		if comp.Name == "" || len(comp.Selectors) == 0 {
			continue // skip anonymous/empty
		}
		if counts[comp.Name] == 0 {
			if viewName != "" {
				warnings = append(warnings, fmt.Sprintf("%s: %s \u2014 0 matches", viewName, comp.Name))
			} else {
				warnings = append(warnings, fmt.Sprintf("%s \u2014 0 matches", comp.Name))
			}
		}
	}

	if len(warnings) == 0 {
		return
	}
	fmt.Fprintln(w, "[Warnings]")
	for _, w2 := range warnings {
		fmt.Fprintln(w, w2)
	}
	fmt.Fprintln(w)
}

// writeCoverage prints the [Coverage] section.
func writeCoverage(w io.Writer, cov coverage.Result) {
	if cov.VisibleOnly {
		fmt.Fprintf(w, "[Coverage] (visible only)\n")
	} else {
		fmt.Fprintf(w, "[Coverage]\n")
	}
	fmt.Fprintf(w, "%d interactive · %d direct T1 (%d%%) · %d scoped T2 (%d%%) · %d orphaned T3 %s\n",
		cov.Total, cov.T1, coverage.Pct(cov.T1, cov.Total), cov.T2, coverage.Pct(cov.T2, cov.Total), cov.T3, cov.Mark())
	if cov.Empty() {
		fmt.Fprintf(w, "⚠ no interactive nodes — the page may be blank or still loading (no coverage signal)\n")
	}
}

// WriteTrace prints the "Unlabeled clusters" section for T3 (orphaned) nodes.
// Clusters are grouped by (role, name, nearest-data-attr-ancestor selector)
// and sorted by count descending.
func WriteTrace(
	w io.Writer,
	t3nodes []*comps.ComponentNode,
	parentMap map[*comps.ComponentNode]*comps.ComponentNode,
) {
	type clusterKey struct {
		role   string
		name   string // "(no text)" when empty
		inside string // selector of nearest data-attr ancestor, or ""
	}
	type cluster struct {
		count int
		rep   *comps.ComponentNode
	}

	var order []clusterKey
	clusters := make(map[clusterKey]*cluster)

	for _, n := range t3nodes {
		anc := coverage.NearestDataAttrAncestor(n, parentMap)
		inside := ""
		if anc != nil {
			inside = coverage.DataAttrSelector(anc)
		}

		nameStr := n.Name
		if nameStr == "" {
			nameStr = "(no text)"
		} else {
			nameStr = render.TruncateRunes(nameStr, 40)
		}

		role := n.Role
		if role == "" {
			role = "?"
		}

		key := clusterKey{role: role, name: nameStr, inside: inside}
		if c, ok := clusters[key]; ok {
			c.count++
		} else {
			clusters[key] = &cluster{count: 1, rep: n}
			order = append(order, key)
		}
	}

	sort.Slice(order, func(i, j int) bool {
		ci, cj := clusters[order[i]], clusters[order[j]]
		if ci.count != cj.count {
			return ci.count > cj.count
		}
		if order[i].role != order[j].role {
			return order[i].role < order[j].role
		}
		return order[i].name < order[j].name
	})

	fmt.Fprintf(w, "Unlabeled clusters:\n")
	for _, key := range order {
		c := clusters[key]
		nameDisp := key.name
		if key.name != "(no text)" {
			nameDisp = `"` + key.name + `"`
		}
		fmt.Fprintf(w, "  %d\u00d7 %s %s\n", c.count, key.role, nameDisp)
		if key.inside != "" {
			fmt.Fprintf(w, "       inside: %s\n", key.inside)
			if hint := coverage.ArrowHint(c.rep, parentMap); hint != "" {
				fmt.Fprintf(w, "       \u2192 %s\n", hint)
			}
		}
	}
}

// WriteT2Trace prints T2 scopes (interactive children without direct matches).
// Enumerates each child node (role + name) with suppression rules:
//   - Single-child T2 scopes are omitted (simple wrapper, acceptable)
//   - Components with memory notes show suppressed summary
//   - Large lists truncate at 8 items with "… (N more)"
func WriteT2Trace(
	w io.Writer,
	t2clusters map[*comps.ComponentNode]int,
	t2children map[*comps.ComponentNode][]*comps.ComponentNode,
	matches map[*comps.ComponentNode]*match.SightmapMatch,
	view *sightmap.View,
) {
	const minT2Count = 2 // ≥2 children required (single-child = simple wrapper)
	const truncateAt = 8 // show first 8, then "… (N more)"

	type entry struct {
		name      string
		count     int
		children  []*comps.ComponentNode
		hasMemory bool
	}
	var entries []entry
	for node, count := range t2clusters {
		if count < minT2Count {
			continue
		}
		m := matches[node]
		if m == nil || m.Name == "" {
			continue
		}
		// Check if component has memory note
		hasMemory := false
		if view != nil {
			for _, c := range view.Components {
				if c.Name == m.Name && len(c.Memory) > 0 {
					hasMemory = true
					break
				}
			}
		}
		entries = append(entries, entry{
			name:      m.Name,
			count:     count,
			children:  t2children[node],
			hasMemory: hasMemory,
		})
	}
	if len(entries) == 0 {
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count != entries[j].count {
			return entries[i].count > entries[j].count
		}
		return entries[i].name < entries[j].name
	})
	if len(entries) > 10 {
		entries = entries[:10]
	}

	fmt.Fprintf(w, "T2 scopes (interactive children without direct component matches):\n")
	for _, e := range entries {
		fmt.Fprintf(w, "  [%s] (%d)", e.name, e.count)
		if e.hasMemory {
			fmt.Fprintf(w, " — exhausted, see component memory note\n")
			continue
		}
		fmt.Fprintln(w)
		// Enumerate children
		shown := e.count
		if shown > truncateAt {
			shown = truncateAt
		}
		for i := 0; i < shown && i < len(e.children); i++ {
			child := e.children[i]
			fmt.Fprintf(w, "    %s %q\n", child.Role, child.Name)
		}
		if e.count > truncateAt {
			fmt.Fprintf(w, "    … (%d more)\n", e.count-truncateAt)
		}
	}
}

// CheckRootComponents writes a [Snapshot check] warning to w when every
// view-specific top-level component (empty ParentChain, not in globalNames)
// matched 0 nodes. This is a strong signal that the snapshot captured the
// wrong page — e.g. a direct URL load silently fell back to a different
// route (the topwork wizard returning the Dashboard). globalNames excludes
// shared components (Header, TopworkApp, etc.) that match any page and would
// suppress the warning spuriously. Pass nil to skip the exclusion.
func CheckRootComponents(w io.Writer, viewComponents []match.SightmapComponent, globalNames map[string]bool, counts map[string]int, viewName string) {
	var topNames []string
	for _, comp := range viewComponents {
		if comp.Name == "" || len(comp.Selectors) == 0 || len(comp.ParentChain) > 0 {
			continue
		}
		if globalNames[comp.Name] {
			continue // skip globals — they match on any page
		}
		topNames = append(topNames, comp.Name)
	}
	if len(topNames) == 0 {
		return // no view-specific top-level components to check
	}
	for _, name := range topNames {
		if counts[name] > 0 {
			return // at least one view-specific root matched — page is plausibly correct
		}
	}
	label := ""
	if viewName != "" {
		label = viewName + ": "
	}
	fmt.Fprintf(w, "\n[Snapshot check] %sview root components all unmatched — wrong page captured? (expected: %s)\n\n",
		label, strings.Join(topNames, ", "))
}
