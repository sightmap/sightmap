// snapshot connects to Chrome (or launches it), runs the full
// component-extraction pipeline, applies the .sightmap/ corpus, and emits an
// annotated ARIA tree together with coverage statistics.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/render"
	"github.com/sightmap/sightmap/go/sightmap"
)

// printOpts carries the display-filter flags through the tree-printing functions.
type printOpts struct {
	interactive    bool
	compact        bool // kept for compat; now a no-op (render is always compact)
	maxDepth       int
	trace          bool
	visibleOnly    bool
	propValues     map[string]map[string]string // nodeID → {propName → value}; nil in offline mode
	selectors      bool                         // show tag #id [data-testid] selector hints
	viewComponents []match.SightmapComponent    // for zero-match warnings
	globalNames    map[string]bool              // corpus-level global component names; used by page-check
}

func runSnapshot(args []string) error {
	fs := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	addrFlag := fs.String("addr", browser.DefaultAddr(), "CDP address (host:port)")
	launchFlag := fs.Bool("launch", false, "Auto-launch Chrome if unreachable")
	urlFlag := fs.String("url", "", "Navigate to this URL before snapping")
	waitFlag := fs.Float64("wait", 0, "Extra seconds to wait after DOM stability is detected (default 0; use for unusually slow pages)")
	outFlag := fs.String("out", "", "Write to file instead of stdout")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir")
	interactiveFlag := fs.Bool("interactive", false, "Show interactive nodes only")
	depthFlag := fs.Int("depth", 0, "Max tree depth (0 = unlimited)")
	compactFlag := fs.Bool("compact", false, "Omit hidden/ignored nodes from output")
	traceFlag := fs.Bool("trace", false, "Include selector hints for unlabeled interactive clusters")
	visibleFlag := fs.Bool("visible", true, "Count only visible nodes (default: true; use --include-hidden to disable)")
	includeHiddenFlag := fs.Bool("include-hidden", false, "Include hidden/off-screen nodes in analysis")
	selectorsFlag := fs.Bool("selectors", false, "Show tag #id [data-testid] selector hints (no CSS classes)")
	treeOutFlag := fs.String("tree-out", "", "Write raw ComponentNode tree JSON to this file (enables offline coverage/multi-coverage)")
	jsonOutFlag := fs.String("json", "", "Write annotated tree JSON to this file (superset of --tree-out: includes component name, memory, extracted props)")
	screenshotFlag := fs.String("screenshot", "", "Save a PNG screenshot to this file alongside the tree")
	allFlag := fs.Bool("all", false, "Snap every view URL declared in views/*.yaml")
	forceFlag := fs.Bool("force", false, "Write the capture even if it adds no new component/slot vs the view set (skip the novelty gate)")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Apply .sightmap/config.yaml defaults for flags not explicitly set.
	{
		explicit := map[string]bool{}
		fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
		cfg := loadSiteConfig(*sightmapDirFlag)
		if !explicit["wait"] && cfg.Snapshot.Wait > 0 {
			*waitFlag = cfg.Snapshot.Wait
		}
		if !explicit["include-hidden"] && cfg.Snapshot.IncludeHidden {
			*includeHiddenFlag = true
		}
		if !explicit["trace"] && cfg.Snapshot.Trace {
			*traceFlag = true
		}
	}

	visible := *visibleFlag
	if *includeHiddenFlag {
		visible = false
	}

	if *allFlag {
		return runSnapshotAll(args, *sightmapDirFlag, *addrFlag, *tabFlag, *waitFlag,
			*interactiveFlag, *depthFlag, visible, *selectorsFlag, *jsonOutFlag, *forceFlag)
	}

	ctx := context.Background()

	// ── Connect to Chrome ────────────────────────────────────────────────────
	cleanup := func() {}
	defer func() { cleanup() }()

	conn, dialErr := dialAddrTab(*addrFlag, *tabFlag)
	if dialErr != nil {
		if !*launchFlag {
			return fmt.Errorf(
				"snapshot: cannot connect to Chrome at %s\n"+
					"Start a session first:\n"+
					"  snapshot --launch --url https://...    (auto-launch Chrome)\n"+
					"  /path/to/chrome --remote-debugging-port=7892 --user-data-dir=/tmp/cdp",
				*addrFlag,
			)
		}
		var launchCleanup func()
		var launchErr error
		conn, launchCleanup, launchErr = browser.Launch(ctx, browser.LaunchOptions{})
		if launchErr != nil {
			return fmt.Errorf("cannot launch Chrome: %v", launchErr)
		}
		if launchCleanup != nil {
			cleanup = launchCleanup
		}
	} else {
		cleanup = func() { conn.Close() }
	}

	// ── Navigate ─────────────────────────────────────────────────────────────
	if *urlFlag != "" {
		// NavigateAndWaitIdle waits for loadEventFired then networkIdle — the
		// correct signal for React/SPA pages where the a11y tree is populated
		// only after async data fetches complete.
		if err := browser.NavigateAndWaitIdle(ctx, conn, *urlFlag, 8*time.Second); err != nil {
			return fmt.Errorf("navigate: %v", err)
		}
	}
	if *waitFlag > 0 {
		time.Sleep(time.Duration(*waitFlag * float64(time.Second)))
	}

	// ── Extract ───────────────────────────────────────────────────────────────
	pageURL, err := browser.GetURL(ctx, conn)
	if err != nil {
		return fmt.Errorf("get URL: %v", err)
	}

	page, err := conn.DefaultPage()
	if err != nil {
		return fmt.Errorf("default page: %v", err)
	}
	root, err := browser.ExtractComponents(ctx, page)
	if err != nil {
		return fmt.Errorf("extract components: %v", err)
	}

	// ── Load sightmap (optional) ──────────────────────────────────────────────
	var snapshotMatches map[*comps.ComponentNode]*match.SightmapMatch
	var view *sightmap.View
	var propValues map[string]map[string]string
	var viewComponents []match.SightmapComponent
	var globalNames map[string]bool
	if _, statErr := os.Stat(*sightmapDirFlag); statErr == nil {
		sess := sightmap.NewSession(sightmap.DirLoader(*sightmapDirFlag))
		if m, mErr := sess.MatchTree(root, pageURL); mErr == nil {
			snapshotMatches = m
		} else {
			fmt.Fprintf(os.Stderr, "snapshot: sightmap match: %v\n", mErr)
		}
		if v, vErr := sess.ViewForURL(pageURL); vErr == nil {
			view = v
		} else {
			fmt.Fprintf(os.Stderr, "snapshot: view lookup: %v\n", vErr)
		}
		if components, cErr := sess.Components(pageURL); cErr == nil {
			viewComponents = components
		}
		if gn, gnErr := sess.GlobalComponentNames(); gnErr == nil {
			globalNames = gn
		}
		// Extract property values from the live DOM (skipped in offline mode).
		if conn != nil && len(snapshotMatches) > 0 && len(viewComponents) > 0 {
			compByName := make(map[string]match.SightmapComponent, len(viewComponents))
			for _, c := range viewComponents {
				compByName[c.Name] = c
			}
			propValues = extractProperties(ctx, conn, snapshotMatches, compByName)
		}
	}

	opts := printOpts{
		interactive:    *interactiveFlag,
		compact:        *compactFlag,
		maxDepth:       *depthFlag,
		trace:          *traceFlag,
		visibleOnly:    visible,
		propValues:     propValues,
		selectors:      *selectorsFlag,
		viewComponents: viewComponents,
		globalNames:    globalNames,
	}

	// ── Screenshot (alongside the tree, same page state) ───────────────────────
	if *screenshotFlag != "" {
		png, sErr := browser.Screenshot(ctx, conn)
		if sErr != nil {
			fmt.Fprintf(os.Stderr, "snapshot: screenshot: %v\n", sErr)
		} else if wErr := os.WriteFile(*screenshotFlag, png, 0o644); wErr != nil {
			fmt.Fprintf(os.Stderr, "snapshot: write screenshot: %v\n", wErr)
		} else {
			fmt.Fprintf(os.Stderr, "screenshot saved: %s\n", *screenshotFlag)
		}
	}

	// ── Write component tree JSON (for offline coverage tools) ──────────────
	if *treeOutFlag != "" {
		if err := writeTreeJSON(root, *treeOutFlag); err != nil {
			fmt.Fprintf(os.Stderr, "snapshot: tree-out: %v\n", err)
		}
	}
	if *jsonOutFlag != "" {
		if err := writeAnnotatedJSON(root, *jsonOutFlag, view, snapshotMatches, propValues); err != nil {
			fmt.Fprintf(os.Stderr, "snapshot: json-out: %v\n", err)
		}
	}

	// If --out is not specified but we have a view, append a timestamped capture to
	// the view's set: snapshots/<view>/<stamp>.snap. An explicit
	// --out still writes exactly there.
	if *outFlag == "" && view != nil {
		viewBasename := view.SnapBasename()
		// Novelty gate: skip writing a capture that adds no new component
		// type or orphan slot vs the existing set. First capture always writes; --force
		// bypasses. Only applies to the auto-organized set-append path, never --out.
		if snapshotMatches != nil {
			parentMap := buildParentMap(root)
			_, _, _, _, t3nodes, _, _ := computeCoverage(root, snapshotMatches, parentMap, visible)
			cand := slotsFromMatch(snapshotMatches, t3nodes, parentMap)
			if res, write := noveltyGate(*sightmapDirFlag, viewBasename, cand, *forceFlag); !write {
				fmt.Fprintf(os.Stderr, "snapshot: nothing new vs %d capture(s) in %s — not saved (use --force to keep)\n",
					res.ComparedTo, viewBasename)
				return nil
			}
		}
		*outFlag = viewSnapshotPath(*sightmapDirFlag, viewBasename, time.Now())
		if *treeOutFlag == "" {
			*treeOutFlag = snapshotTreePath(*outFlag)
		}
	}

	// ── Write output ───────────────────────────────────────────────────────
	if *outFlag != "" {
		// Create parent directory if needed
		if dir := filepath.Dir(*outFlag); dir != "." {
			if mkdirErr := os.MkdirAll(dir, 0o755); mkdirErr != nil {
				return fmt.Errorf("create output directory: %v", mkdirErr)
			}
		}

		tmpPath := *outFlag + ".tmp"
		f, ferr := os.Create(tmpPath)
		if ferr != nil {
			return fmt.Errorf("create output file: %v", ferr)
		}
		writeOutput(f, root, view, snapshotMatches, opts)
		if cerr := f.Close(); cerr != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("close output file: %v", cerr)
		}
		if rerr := os.Rename(tmpPath, *outFlag); rerr != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("rename output file: %v", rerr)
		}

		// Write tree JSON alongside if not explicitly specified
		if *treeOutFlag != "" {
			if err := writeTreeJSON(root, *treeOutFlag); err != nil {
				fmt.Fprintf(os.Stderr, "snapshot: tree-out: %v\n", err)
			}
		}
	} else {
		writeOutput(os.Stdout, root, view, snapshotMatches, opts)
	}
	return nil
}

// runSnapshotAll navigates to each view URL and writes a snap + tree JSON file
// per view, then prints a summary table.
func runSnapshotAll(
	_ []string,
	sightmapDir, addr, tabID string,
	wait float64,
	interactive bool,
	depth int,
	visible bool,
	selectors bool,
	jsonOut string,
	force bool,
) error {
	targets, err := corpusProbeTargets(sightmapDir)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return fmt.Errorf("no view URLs to snapshot — add url: (and optional snapshots[].url) to views/*.yaml")
	}

	ctx := context.Background()
	conn, err := dialAddrTab(addr, tabID)
	if err != nil {
		return fmt.Errorf("snapshot --all: %w", err)
	}
	defer conn.Close()

	type result struct {
		name              string
		t1, t2, t3, total int
		err               error
		skipped           bool // novelty gate: nothing new, not written
		skippedVs         int  // captures compared against when skipped
	}
	var results []result

	for _, v := range targets {
		label := v.ViewName
		if v.SnapName != "base" {
			label = v.ViewName + "/" + v.SnapName
		}
		fmt.Fprintf(os.Stderr, "snapping %s → %s\n", label, v.URL)

		if err := browser.NavigateAndWaitIdle(ctx, conn, v.URL, 8*time.Second); err != nil {
			results = append(results, result{name: label, err: err})
			continue
		}
		if wait > 0 {
			time.Sleep(time.Duration(wait * float64(time.Second)))
		}

		pageURL, _ := browser.GetURL(ctx, conn)
		page, err := conn.DefaultPage()
		if err != nil {
			results = append(results, result{name: label, err: err})
			continue
		}
		root, err := browser.ExtractComponents(ctx, page)
		if err != nil {
			results = append(results, result{name: label, err: err})
			continue
		}

		sess := sightmap.NewSession(sightmap.DirLoader(sightmapDir))
		snapshotMatches, _ := sess.MatchTree(root, pageURL)
		view, _ := sess.ViewForURL(pageURL)
		var viewComponents []match.SightmapComponent
		if components, cErr := sess.Components(pageURL); cErr == nil {
			viewComponents = components
		}
		globalNames, _ := sess.GlobalComponentNames()

		opts := printOpts{
			interactive:    interactive,
			maxDepth:       depth,
			visibleOnly:    visible,
			selectors:      selectors,
			viewComponents: viewComponents,
			globalNames:    globalNames,
		}

		// Novelty gate: don't append a redundant capture to the view set.
		if !force {
			parentMap := buildParentMap(root)
			_, _, _, _, t3nodes, _, _ := computeCoverage(root, snapshotMatches, parentMap, visible)
			cand := slotsFromMatch(snapshotMatches, t3nodes, parentMap)
			if res, write := noveltyGate(sightmapDir, v.ViewDir, cand, false); !write {
				results = append(results, result{name: label, skipped: true, skippedVs: res.ComparedTo})
				continue
			}
		}

		// Append a timestamped capture to the view set.
		snapPath := viewSnapshotPath(sightmapDir, v.ViewDir, time.Now())
		treeJSONPath := snapshotTreePath(snapPath)

		// Create snapshots directory if needed
		snapDir := filepath.Dir(snapPath)
		if mkdirErr := os.MkdirAll(snapDir, 0o755); mkdirErr != nil {
			results = append(results, result{name: label, err: mkdirErr})
			continue
		}

		tmpPath := snapPath + ".tmp"
		f, ferr := os.Create(tmpPath)
		if ferr != nil {
			results = append(results, result{name: label, err: ferr})
			continue
		}
		writeOutput(f, root, view, snapshotMatches, opts)
		f.Close()
		os.Rename(tmpPath, snapPath)

		writeTreeJSON(root, treeJSONPath)
		if jsonOut != "" {
			jsonPath := strings.TrimSuffix(snapPath, ".snap") + ".snap.annotated.json"
			writeAnnotatedJSON(root, jsonPath, view, snapshotMatches, nil) // no propValues in --all mode
		}

		parentMap := buildParentMap(root)
		t1, t2, t3, total, _, _, _ := computeCoverage(root, snapshotMatches, parentMap, visible)
		results = append(results, result{name: label, t1: t1, t2: t2, t3: t3, total: total})
	}

	fmt.Fprintln(os.Stderr)
	for _, r := range results {
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "%-20s  ERROR: %v\n", r.name, r.err)
			continue
		}
		if r.skipped {
			fmt.Fprintf(os.Stderr, "%-20s  = nothing new vs %d snap(s) — not saved\n", r.name, r.skippedVs)
			continue
		}
		check := "✓"
		if r.t3 > 0 {
			check = "✗"
		}
		fmt.Fprintf(os.Stderr, "%-20s  %d interactive · T1 %d%% · T2 %d%% · T3 %d %s\n",
			r.name, r.total,
			coveragePct(r.t1, r.total),
			coveragePct(r.t2, r.total),
			r.t3, check,
		)
	}
	return nil
}

// ── Output sections ───────────────────────────────────────────────────────────

// writeOutput writes all sections in order to w.
func writeOutput(
	w io.Writer,
	root *comps.ComponentNode,
	view *sightmap.View,
	matches map[*comps.ComponentNode]*match.SightmapMatch,
	opts printOpts,
) {
	// Pre-compute tree structures once so we can share them across sections.
	var parentMap map[*comps.ComponentNode]*comps.ComponentNode
	if root != nil {
		parentMap = buildParentMap(root)
	}

	// ── [View: ...] ──────────────────────────────────────────────────────────
	if view != nil {
		fmt.Fprintf(w, "[View: %s]\n", view.Name)
		fmt.Fprintf(w, "route: %s\n", view.Route)
		if len(view.Memory) > 0 {
			fmt.Fprintf(w, "memory:\n")
			for _, m := range view.Memory {
				fmt.Fprintf(w, "  - %s\n", m)
			}
		}
		fmt.Fprintln(w)
	}

	// ── [Guide] ───────────────────────────────────────────────────────────────
	if len(matches) > 0 {
		writeGuide(w, matches)
		fmt.Fprintln(w)
	}

	// ── component tree ────────────────────────────────────────────────────────
	fmt.Fprintf(w, "--- component tree ---\n\n")
	if root != nil {
		comp := render.Filter(root, matches)
		if comp != nil {
			comp.Format(w, "", render.FormatOpts{
				Selectors:   opts.selectors,
				MaxDepth:    opts.maxDepth,
				Interactive: opts.interactive,
				PropValues:  opts.propValues,
			})
		}
	}

	// ── [Warnings] ────────────────────────────────────────────────────────────
	if len(opts.viewComponents) > 0 && matches != nil {
		writeZeroMatchWarnings(w, opts.viewComponents, matches, view, opts.globalNames)
	}

	// ── [Coverage] ────────────────────────────────────────────────────────────
	if matches != nil {
		fmt.Fprintln(w)
		t1, t2, t3, total, t3nodes, t2clusters, t2children := computeCoverage(root, matches, parentMap, opts.visibleOnly)
		writeCoverage(w, t1, t2, t3, total, opts.visibleOnly)

		// ── Unlabeled clusters (--trace) ─────────────────────────────────────
		if opts.trace {
			if len(t3nodes) > 0 {
				fmt.Fprintln(w)
				writeTrace(w, t3nodes, parentMap)
			}
			if len(t2clusters) > 0 {
				fmt.Fprintln(w)
				writeT2Trace(w, t2clusters, t2children, matches, view)
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
		checkRootComponents(w, view.Components, globalNames, counts, viewName)
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
func writeCoverage(w io.Writer, t1, t2, t3, total int, visibleOnly bool) {
	pct := func(n, d int) int {
		if d == 0 {
			return 0
		}
		return int(math.Round(float64(n) / float64(d) * 100))
	}
	check := "✗"
	if t3 == 0 {
		check = "✓"
	}
	if visibleOnly {
		fmt.Fprintf(w, "[Coverage] (visible only)\n")
	} else {
		fmt.Fprintf(w, "[Coverage]\n")
	}
	fmt.Fprintf(w, "%d interactive · %d direct T1 (%d%%) · %d scoped T2 (%d%%) · %d orphaned T3 %s\n",
		total, t1, pct(t1, total), t2, pct(t2, total), t3, check)
}

// writeTrace prints the "Unlabeled clusters" section for T3 (orphaned) nodes.
// Clusters are grouped by (role, name, nearest-data-attr-ancestor selector)
// and sorted by count descending.
func writeTrace(
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
		anc := nearestDataAttrAncestor(n, parentMap)
		inside := ""
		if anc != nil {
			inside = dataAttrSel(anc)
		}

		nameStr := n.Name
		if nameStr == "" {
			nameStr = "(no text)"
		} else {
			nameStr = truncateStr(nameStr, 40)
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
			if hint := arrowHint(c.rep, parentMap); hint != "" {
				fmt.Fprintf(w, "       \u2192 %s\n", hint)
			}
		}
	}
}

// writeT2Trace prints T2 scopes (interactive children without direct matches).
// Enumerates each child node (role + name) with suppression rules:
//   - Single-child T2 scopes are omitted (simple wrapper, acceptable)
//   - Components with memory notes show suppressed summary
//   - Large lists truncate at 8 items with "… (N more)"
func writeT2Trace(
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

// ── Property extraction ──────────────────────────────────────────────────────

// extractProperties runs a single batched JS evaluation against the live DOM
// to extract property values for all matched nodes that have property
// definitions. Returns a map from nodeID to {propName → value}.
// At most 200 nodes are evaluated to avoid JS timeout.
func extractProperties(
	ctx context.Context,
	conn *browser.CDPConn,
	matches map[*comps.ComponentNode]*match.SightmapMatch,
	compByName map[string]match.SightmapComponent,
) map[string]map[string]string {
	type specProp struct {
		Name      string `json:"name"`
		Extract   string `json:"extract"`
		Transform string `json:"transform"`
	}
	type spec struct {
		ID       string     `json:"id"`
		Selector string     `json:"selector"`
		Props    []specProp `json:"props"`
	}

	var specs []spec
	const maxNodes = 200
	for node, m := range matches {
		if len(specs) >= maxNodes {
			break
		}
		comp, ok := compByName[m.Name]
		if !ok || len(comp.Properties) == 0 || len(comp.Selectors) == 0 {
			continue
		}
		sp := spec{
			ID:       node.Id,
			Selector: comp.Selectors[0],
			Props:    make([]specProp, len(comp.Properties)),
		}
		for i, p := range comp.Properties {
			sp.Props[i] = specProp{Name: p.Name, Extract: p.Extract, Transform: p.Transform}
		}
		specs = append(specs, sp)
	}
	if len(specs) == 0 {
		return nil
	}

	specsJSON, err := json.Marshal(specs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "snapshot: marshal property specs: %v\n", err)
		return nil
	}

	const jsTemplate = `(function(specs) {
  // Canonical extractor — mirrored in cmd/sightmap/extension/{content,resolver}.js
  // and (transforms only) sightmap/property.go. Returns the RAW value; the caller
  // normalizes whitespace, applies the transform, and caps length uniformly.
  function extractValue(el, extract) {
    if (!extract) return null;
    if (extract === 'text') return el.textContent;
    if (extract === 'inner_text') return el.innerText;
    if (extract === 'text_only') {
      const clone = el.cloneNode(true);
      clone.querySelectorAll('img,svg,[alt]').forEach(e => e.remove());
      return clone.textContent;
    }
    if (extract === 'inner_html') return el.innerHTML;
    if (extract.startsWith('attr=')) return el.getAttribute(extract.slice(5));
    if (extract.startsWith('exists:')) {
      return el.querySelector(extract.slice(7)) ? 'true' : null;
    }
    const sub = el.querySelector(extract);
    return sub ? (sub.innerText != null ? sub.innerText : sub.textContent) : null;
  }
  function applyTransform(val, transform) {
    if (!transform || !val) return val;
    if (transform.indexOf('match:') === 0) {
      try {
        const m = val.match(new RegExp(transform.slice(6)));
        if (!m) return val;
        return m[1] != null ? m[1] : m[0];
      } catch (e) { return val; }
    }
    const words = val.trim().split(/\s+/);
    switch(transform) {
      case 'first_word': return words[0] || val;
      case 'last_word':  return words[words.length-1] || val;
      case 'first_number': { const m = val.match(/\d[\d,.]*/); return m ? m[0] : val; }
      case 'first_dollar': { const m = val.match(/\$[\d,.]+/); return m ? m[0] : val; }
      case 'number':     return val.replace(/[^\d.]/g, '');
      case 'slug':       return val.toLowerCase().replace(/\s+/g, '-').replace(/[^a-z0-9-]/g, '');
      default: return val;
    }
  }
  const results = {};
  for (const {id, selector, props} of specs) {
    // Anchor to the exact matched element via its sightmap ID attribute (set by
    // probe.js as data-sightmap-id). This ensures child components like
    // BreadcrumbLink get their own text, not the first match on the page.
    const el = (id ? document.querySelector('[data-sightmap-id="' + id + '"]') : null)
               || document.querySelector(selector);
    if (!el) continue;
    const vals = {};
    for (const {name, extract, transform} of props) {
      let val = extractValue(el, extract);
      if (val == null) continue;
      val = String(val).trim().replace(/\s+/g, ' ');
      if (val === '') continue;
      val = applyTransform(val, transform);
      if (val) vals[name] = String(val).slice(0, 120);
    }
    if (Object.keys(vals).length > 0) results[id] = vals;
  }
  return results;
})(SPECS_JSON)`

	script := strings.Replace(jsTemplate, "SPECS_JSON", string(specsJSON), 1)

	resultJSON, evalErr := browser.EvalJSON(ctx, conn, script)
	if evalErr != nil {
		fmt.Fprintf(os.Stderr, "snapshot: property extraction: %v\n", evalErr)
		return nil
	}

	var propValues map[string]map[string]string
	if err := json.Unmarshal(resultJSON, &propValues); err != nil {
		fmt.Fprintf(os.Stderr, "snapshot: property extraction unmarshal: %v\n", err)
		return nil
	}
	return propValues
}

// ── Tree pre-passes ───────────────────────────────────────────────────────────

// buildParentMap returns a map from each non-root node to its parent.
func buildParentMap(root *comps.ComponentNode) map[*comps.ComponentNode]*comps.ComponentNode {
	pm := make(map[*comps.ComponentNode]*comps.ComponentNode)
	if root == nil {
		return pm
	}
	comps.Walk(root, func(n *comps.ComponentNode, _ int) bool {
		for _, child := range n.Children {
			pm[child] = n
		}
		return true
	})
	return pm
}

// ── Coverage computation ──────────────────────────────────────────────────────

// computeCoverage classifies every interactive node into T1/T2/T3.
//
//   - T1 (direct): the node itself has a sightmap match.
//   - T2 (scoped): an ancestor has a sightmap match.
//   - T3 (orphan): no sightmap context at any depth.
func computeCoverage(
	root *comps.ComponentNode,
	matches map[*comps.ComponentNode]*match.SightmapMatch,
	parentMap map[*comps.ComponentNode]*comps.ComponentNode,
	visibleOnly bool,
) (t1, t2, t3, total int, t3nodes []*comps.ComponentNode, t2clusters map[*comps.ComponentNode]int, t2children map[*comps.ComponentNode][]*comps.ComponentNode) {
	t2clusters = make(map[*comps.ComponentNode]int)
	t2children = make(map[*comps.ComponentNode][]*comps.ComponentNode)
	if root == nil {
		return
	}
	comps.Walk(root, func(n *comps.ComponentNode, _ int) bool {
		if !n.IsInteractive {
			return true
		}
		if visibleOnly && (!n.IsVisible || n.IsIgnored) {
			return true // skip hidden/ignored interactive nodes
		}
		total++
		if matches[n] != nil {
			t1++
		} else if anc := nearestMatchedAncestor(n, parentMap, matches); anc != nil {
			t2++
			t2clusters[anc]++
			t2children[anc] = append(t2children[anc], n)
		} else {
			t3++
			t3nodes = append(t3nodes, n)
		}
		return true
	})
	return
}

// nearestMatchedAncestor returns the nearest ancestor of node that has a
// sightmap match, or nil if none exists.
func nearestMatchedAncestor(
	node *comps.ComponentNode,
	parentMap map[*comps.ComponentNode]*comps.ComponentNode,
	matches map[*comps.ComponentNode]*match.SightmapMatch,
) *comps.ComponentNode {
	curr := parentMap[node]
	for curr != nil {
		if matches[curr] != nil {
			return curr
		}
		curr = parentMap[curr]
	}
	return nil
}

// ── Trace helpers ─────────────────────────────────────────────────────────────

// nearestDataAttrAncestor walks up parentMap to find the nearest ancestor
// whose selector carries a data-testid or data-component attribute.
func nearestDataAttrAncestor(
	node *comps.ComponentNode,
	parentMap map[*comps.ComponentNode]*comps.ComponentNode,
) *comps.ComponentNode {
	curr := parentMap[node]
	for curr != nil {
		if curr.Selector != nil {
			a := curr.Selector.Attrs
			if a["data-testid"] != "" || a["data-component"] != "" {
				return curr
			}
		}
		curr = parentMap[curr]
	}
	return nil
}

// dataAttrSel formats an ancestor node for the "inside:" display line.
// Prefers data-testid, then data-component, then falls back to basic tag#id.cls.
func dataAttrSel(node *comps.ComponentNode) string {
	if node.Selector == nil {
		return node.Role
	}
	tag := node.Selector.Tag
	if tag == "" {
		tag = node.Role
	}
	a := node.Selector.Attrs
	if dt := a["data-testid"]; dt != "" {
		return fmt.Sprintf(`%s[data-testid="%s"]`, tag, dt)
	}
	if dc := a["data-component"]; dc != "" {
		return fmt.Sprintf(`%s[data-component^="%s"]`, tag, dc)
	}
	return selectorStr(node.Selector)
}

// orphanSlotKey is the cross-snapshot STRUCTURAL identity of an uncovered
// interactive node, used by snapshot novelty to decide whether a
// capture introduces a new "slot". It is the node's role plus the selector of
// its nearest data-attr ancestor — deliberately WITHOUT the node's instance text,
// so different instances of the same widget (e.g. each product link in a
// carousel) collapse to one slot and don't read as endless novelty.
//
// It shares the same anchor primitives (nearestDataAttrAncestor / dataAttrSel) as
// the coverage "Unlabeled clusters" trace and `gap`, so changing the anchoring
// heuristic (the role+attr scheme) only needs to touch those helpers.
func orphanSlotKey(
	node *comps.ComponentNode,
	parentMap map[*comps.ComponentNode]*comps.ComponentNode,
) string {
	role := node.Role
	if role == "" {
		role = "?"
	}
	inside := "(no stable ancestor)"
	if anc := nearestDataAttrAncestor(node, parentMap); anc != nil {
		inside = dataAttrSel(anc)
	}
	return role + " @ " + inside
}

// arrowHint builds the "→ selector" suggestion line for a T3 node.
// It uses the nearest data-attr ancestor as the scope and appends the
// T3 node's own tag as the final selector part.
func arrowHint(
	node *comps.ComponentNode,
	parentMap map[*comps.ComponentNode]*comps.ComponentNode,
) string {
	nodeTag := ""
	if node.Selector != nil {
		nodeTag = node.Selector.Tag
	}
	if nodeTag == "" {
		nodeTag = node.Role
		if nodeTag == "" {
			nodeTag = "?"
		}
	}

	anc := nearestDataAttrAncestor(node, parentMap)
	if anc == nil || anc.Selector == nil {
		return ""
	}
	a := anc.Selector.Attrs
	if dt := a["data-testid"]; dt != "" {
		return fmt.Sprintf(`[data-testid="%s"] %s`, dt, nodeTag)
	}
	if dc := a["data-component"]; dc != "" {
		return fmt.Sprintf(`[data-component^="%s"] %s`, dc, nodeTag)
	}
	return selectorStr(anc.Selector) + " " + nodeTag
}

// ── Formatting helpers ────────────────────────────────────────────────────────

// displayName truncates s to 60 Unicode code points and wraps it in double
// quotes. Only backslash and double-quote are escaped; all other Unicode is
// preserved as-is so the output stays human-readable.
func displayName(s string) string {
	const maxRunes = 60
	if utf8.RuneCountInString(s) > maxRunes {
		runes := []rune(s)
		s = string(runes[:maxRunes]) + "…"
	}
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// selectorStr formats a SelectorPart as tag[#id][.cls1.cls2].
// Attributes (data-testid etc.) are not included in the tree display;
// they appear only in the trace section.
func selectorStr(s *comps.SelectorPart) string {
	if s == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(s.Tag)
	if s.Id != "" {
		b.WriteByte('#')
		b.WriteString(s.Id)
	}
	for _, c := range s.Classes {
		b.WriteByte('.')
		b.WriteString(c)
	}
	return b.String()
}

// truncateStr truncates s to at most maxRunes Unicode code points,
// appending "…" when truncation occurs.
func truncateStr(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "…"
}

// writeTreeJSON serialises root as JSON to path atomically.
func writeTreeJSON(root *comps.ComponentNode, path string) error {
	data, err := json.Marshal(root)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
