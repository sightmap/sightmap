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
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/coverage"
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
	coverageOnly   bool // suppress the tree + guide; print View/Coverage/traces only
	visibleOnly    bool
	propValues     map[string]map[string]string // nodeID → {propName → value}; nil in offline mode
	selectors      bool                         // show tag #id [data-testid] selector hints
	viewComponents []match.SightmapComponent    // for zero-match warnings
	globalNames    map[string]bool              // corpus-level global component names; used by page-check
}

// snapshot is a PURE OBSERVE command: it connects, navigates, extracts, matches
// the corpus, and renders the annotated tree + [Coverage] to stdout (or --out
// FILE). It never writes into the corpus's capture set and never novelty-gates —
// persisting a capture is the separate `capture` command's job.
func runSnapshot(args []string) error {
	fs := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	addrFlag := fs.String("addr", browser.DefaultAddr(), "CDP address (host:port)")
	launchFlag := fs.Bool("launch", false, "Auto-launch Chrome if unreachable")
	urlFlag := fs.String("url", "", "Navigate to this URL before snapping")
	waitFlag := fs.Float64("wait", 0, "Extra seconds to wait after DOM stability is detected (default 0; use for unusually slow pages)")
	outFlag := fs.String("out", "", "Write the annotated output to this file instead of stdout")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir")
	interactiveFlag := fs.Bool("interactive", false, "Show interactive nodes only")
	depthFlag := fs.Int("depth", 0, "Max tree depth (0 = unlimited)")
	compactFlag := fs.Bool("compact", false, "Omit hidden/ignored nodes from output")
	traceFlag := fs.Bool("trace", false, "Include selector hints for unlabeled interactive clusters")
	coverageOnlyFlag := fs.Bool("coverage", false, "Print [View] + [Coverage] + cluster traces only, suppressing the component tree")
	visibleFlag := fs.Bool("visible", true, "Count only visible nodes (default: true; use --include-hidden to disable)")
	includeHiddenFlag := fs.Bool("include-hidden", false, "Include hidden/off-screen nodes in analysis")
	selectorsFlag := fs.Bool("selectors", false, "Show tag #id [data-testid] selector hints (no CSS classes)")
	treeOutFlag := fs.String("tree-out", "", "Write raw ComponentNode tree JSON to this file (enables offline coverage/multi-coverage)")
	jsonOutFlag := fs.String("json", "", "Write annotated tree JSON to this file (superset of --tree-out: includes component name, memory, extracted props)")
	screenshotFlag := fs.String("screenshot", "", "Save a PNG screenshot to this file alongside the tree")
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
		coverageOnly:   *coverageOnlyFlag,
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

	// ── Write output ───────────────────────────────────────────────────────
	// snapshot only ever writes the rendered output to stdout or an explicit
	// --out FILE. It never appends to the corpus capture set (that is `capture`).
	if *outFlag != "" {
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
	} else {
		writeOutput(os.Stdout, root, view, snapshotMatches, opts)
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
	// Skipped in coverage-only mode, which is meant to be a terse feedback view.
	if !opts.coverageOnly && len(matches) > 0 {
		writeGuide(w, matches)
		fmt.Fprintln(w)
	}

	// ── component tree ────────────────────────────────────────────────────────
	// Coverage-only mode suppresses the (potentially huge) tree entirely.
	if !opts.coverageOnly {
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
	}

	// ── [Warnings] ────────────────────────────────────────────────────────────
	if len(opts.viewComponents) > 0 && matches != nil {
		writeZeroMatchWarnings(w, opts.viewComponents, matches, view, opts.globalNames)
	}

	// ── [Coverage] ────────────────────────────────────────────────────────────
	if matches != nil {
		fmt.Fprintln(w)
		cov := coverage.Score(root, matches, coverage.Options{VisibleOnly: opts.visibleOnly})
		writeCoverage(w, cov)

		// ── Unlabeled clusters (--trace) ─────────────────────────────────────
		// Coverage-only mode always shows the cluster traces — they are the point
		// of the terse view (which orphans/ambiguous clusters still need work).
		if opts.trace || opts.coverageOnly {
			if len(cov.Orphans) > 0 {
				fmt.Fprintln(w)
				writeTrace(w, cov.Orphans, cov.ParentMap)
			}
			if len(cov.Scopes) > 0 {
				fmt.Fprintln(w)
				writeT2Trace(w, cov.Scopes, cov.ScopeChildren, matches, view)
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
func writeCoverage(w io.Writer, cov coverage.Result) {
	check := "✗"
	if cov.T3 == 0 {
		check = "✓"
	}
	if cov.VisibleOnly {
		fmt.Fprintf(w, "[Coverage] (visible only)\n")
	} else {
		fmt.Fprintf(w, "[Coverage]\n")
	}
	fmt.Fprintf(w, "%d interactive · %d direct T1 (%d%%) · %d scoped T2 (%d%%) · %d orphaned T3 %s\n",
		cov.Total, cov.T1, coverage.Pct(cov.T1, cov.Total), cov.T2, coverage.Pct(cov.T2, cov.Total), cov.T3, check)
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
		anc := coverage.NearestDataAttrAncestor(n, parentMap)
		inside := ""
		if anc != nil {
			inside = coverage.DataAttrSelector(anc)
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
			if hint := coverage.ArrowHint(c.rep, parentMap); hint != "" {
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
