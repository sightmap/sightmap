package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/coverage"
	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/observe"
	"github.com/sightmap/sightmap/go/sel"
	"github.com/sightmap/sightmap/go/sightmap"
	"github.com/sightmap/sightmap/go/viewset"
)

// runCoverage re-runs T1/T2/T3 coverage on saved snap .tree.json files.
func runCoverage(args []string) error {
	fs := flag.NewFlagSet("coverage", flag.ContinueOnError)
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir")
	visibleFlag := fs.Bool("visible", true, "Count only visible nodes (default: true; use --include-hidden to disable)")
	includeHiddenFlag := fs.Bool("include-hidden", false, "Include hidden/off-screen nodes in coverage stats")
	traceFlag := fs.Bool("trace", false, "Print unlabeled cluster details for T3 orphaned nodes")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Apply .sightmap/config.yaml defaults for flags not explicitly set.
	{
		explicit := map[string]bool{}
		fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
		cfg := sightmap.LoadConfig(*sightmapDirFlag)
		if !explicit["include-hidden"] && cfg.Snapshot.IncludeHidden {
			*includeHiddenFlag = true
		}
		if !explicit["trace"] && cfg.Coverage.Trace {
			*traceFlag = true
		}
	}

	visible := *visibleFlag
	if *includeHiddenFlag {
		visible = false
	}

	var err error
	snapFiles, err := viewset.Find(*sightmapDirFlag, fs.Args())
	if err != nil {
		return err
	}

	corpus, err := sightmap.Load(*sightmapDirFlag)
	if err != nil {
		return fmt.Errorf("load corpus: %v", err)
	}

	// Group captures into their per-VIEW sets so dead-component detection runs over
	// the UNION of the view's snapshots, not a single (possibly reduced) load
	//. T1/T2/T3 stats stay per-capture — they describe each DOM — but
	// the [Warnings] / [Presence] sections are computed across the set.
	sets := viewset.GroupByView(snapFiles)
	views := make([]string, 0, len(sets))
	for v := range sets {
		views = append(views, v)
	}
	sort.Strings(views)

	failures := 0
	for _, v := range views {
		entries := sets[v]
		var caps []viewset.MatchCounts
		var capGaps [][]coverage.ContentGap
		headerView := ""
		for _, e := range entries {
			counts, leafCounts, vName, t3, gaps, ok := coverCapture(e.Path, corpus, visible, *traceFlag)
			if !ok {
				failures++
				continue
			}
			if t3 > 0 {
				failures++
			}
			if vName != "" && headerView == "" {
				headerView = vName
			}
			caps = append(caps, viewset.MatchCounts{Stamp: e.Stamp, Counts: counts, LeafCounts: leafCounts})
			capGaps = append(capGaps, gaps)
		}

		if headerView != "" && corpus != nil && len(caps) > 0 {
			inv := viewComponentNames(corpus, headerView)
			if len(inv) > 0 {
				emitUnionWarnings(headerView, viewset.UnionPresence(inv, caps))
			}
		}

		// Annotation-completeness advisory: named content nodes with no
		// component context, deduped across the view's union. Pure nudge — never
		// touches `failures`. Fall back to the directory view name when no capture
		// carried a [View:] header.
		if len(capGaps) > 0 {
			label := headerView
			if label == "" {
				label = v
			}
			emitContentGaps(label, coverage.UnionContentGaps(capGaps))
		}
	}

	if failures > 0 {
		return fmt.Errorf("%d file(s) failed coverage check", failures)
	}
	return nil
}

// matchCapture loads a saved capture's .tree.json, matches it against the corpus
// using the snap's own route header (viewset.RouteOf), and returns the parsed tree,
// the match map, and that route. ok is false (with a stderr note) when the capture
// can't be loaded, parsed, or matched. Shared by coverage/report/multi-coverage so
// every reader matches a capture the same route-aware way instead of
// each re-implementing the load/parse/match boilerplate.
func matchCapture(snapPath string, corpus *sightmap.Corpus) (root *comps.ComponentNode, matches map[*comps.ComponentNode]*match.SightmapMatch, route string, ok bool) {
	treeFile := snapPath + ".tree.json"
	data, err := os.ReadFile(treeFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: tree file not found (%s), skipping\n",
			filepath.Base(snapPath), treeFile)
		return nil, nil, "", false
	}
	root = &comps.ComponentNode{}
	if err := json.Unmarshal(data, root); err != nil {
		fmt.Fprintf(os.Stderr, "%s: parse tree JSON: %v\n", filepath.Base(snapPath), err)
		return nil, nil, "", false
	}
	route = viewset.RouteOf(snapPath)
	matches = corpus.MatchTree(root, route)
	return root, matches, route, true
}

// coverCapture computes and prints the T1/T2/T3 coverage line for a single
// capture (snapPath is a .snap path; its .tree.json companion holds the tree).
// It returns that capture's per-component matched-node counts, a leaf-only
// match map for dead components (used to distinguish broken selectors from
// genuinely-absent components), the view name from the snap header, the T3
// count, the annotation-completeness gaps found in the capture,
// and ok=false if the capture could not be loaded/parsed/matched.
func coverCapture(snapPath string, corpus *sightmap.Corpus, visible, trace bool) (counts map[string]int, leafCounts map[string]int, viewName string, t3 int, gaps []coverage.ContentGap, ok bool) {
	root, matches, route, ok := matchCapture(snapPath, corpus)
	if !ok {
		return nil, nil, "", 0, nil, false
	}

	// Find view for memory note lookup (T2 trace) and leaf-probe component list.
	var view *sightmap.View
	if corpus != nil {
		for i := range corpus.Views {
			if corpus.Views[i].Route == route {
				view = &corpus.Views[i]
				break
			}
		}
	}

	cov := coverage.Score(root, matches, coverage.Options{VisibleOnly: visible})
	t1, t2, total := cov.T1, cov.T2, cov.Total
	t3nodes, t2clusters, t2children := cov.Orphans, cov.Scopes, cov.ScopeChildren
	parentMap := cov.ParentMap
	t3 = cov.T3
	check := "✓"
	if t3 > 0 {
		check = "✗"
	}
	suffix := ""
	if visible {
		suffix = " (visible only)"
	}

	fmt.Printf("%s%s:\n  %d interactive · %d direct T1 (%d%%) · %d scoped T2 (%d%%) · %d orphaned T3 %s\n",
		filepath.Base(snapPath), suffix,
		total,
		t1, coverage.Pct(t1, total),
		t2, coverage.Pct(t2, total),
		t3, check,
	)
	if trace {
		if len(t3nodes) > 0 {
			fmt.Fprintln(os.Stdout)
			observe.WriteTrace(os.Stdout, t3nodes, parentMap)
		}
		if len(t2clusters) > 0 {
			fmt.Fprintln(os.Stdout)
			observe.WriteT2Trace(os.Stdout, t2clusters, t2children, matches, view)
		}
		fmt.Println()
	}

	// Annotation-completeness gaps for this capture: named content
	// nodes with no component context, surfaced in the union advisory section.
	gaps = coverage.Gaps(root, matches, parentMap, visible)

	counts = make(map[string]int)
	for _, m := range matches {
		if m != nil {
			counts[m.Name]++
		}
	}

	// Leaf-probe dead components: for each view component with 0 full matches,
	// count how many tree nodes match only the selector's leaf (last) part.
	// A non-zero leaf count means the DOM has compatible nodes but the scoped
	// selector missed them — a broken selector, not a missing scenario state.
	if view != nil {
		for _, comp := range view.Components {
			if counts[comp.Name] > 0 || len(comp.Selectors) == 0 {
				continue // matched or no selector — nothing to probe
			}
			if n := countLeafMatches(root, comp.Selectors[0]); n > 0 {
				if leafCounts == nil {
					leafCounts = make(map[string]int)
				}
				leafCounts[comp.Name] = n
			}
		}
		// Page-check: if every top-level view-specific component matched 0 nodes,
		// the captured DOM is almost certainly the wrong page. Build globalNames
		// so that shared components (Header, Nav, etc.) don't suppress the check.
		globalNames := make(map[string]bool, len(corpus.GlobalComponents))
		for _, gc := range corpus.GlobalComponents {
			globalNames[gc.Name] = true
		}
		observe.CheckRootComponents(os.Stdout, view.Components, globalNames, counts, view.Name)
	}

	return counts, leafCounts, viewset.ViewNameOf(snapPath), t3, gaps, true
}

// countLeafMatches returns the number of tree nodes that satisfy the leaf
// (last) SelectorPart of selStr, ignoring all ancestor scoping. Used to
// distinguish a broken scoped selector (leaf matches > 0) from a legitimately-
// absent component (leaf matches == 0).
func countLeafMatches(root *comps.ComponentNode, selStr string) int {
	ps, err := sel.ParseSightmapSelector(selStr)
	if err != nil || len(ps.Parts) == 0 {
		return 0
	}
	leafPart := ps.Parts[len(ps.Parts)-1]
	count := 0
	comps.Walk(root, func(node *comps.ComponentNode, _ int) bool {
		if sel.MatchesNode(node, leafPart) {
			count++
		}
		return true
	})
	return count
}

// viewComponentNames returns the declared, selector-bearing component names for
// the named view (the inventory checked for dead/partial presence).
func viewComponentNames(corpus *sightmap.Corpus, viewName string) []string {
	var names []string
	for _, v := range corpus.Views {
		if v.Name != viewName {
			continue
		}
		for _, comp := range v.Components {
			if comp.Name == "" || len(comp.Selectors) == 0 {
				continue
			}
			names = append(names, comp.Name)
		}
		break
	}
	return names
}

// emitUnionWarnings prints the union-level [Warnings] (dead components whose
// selector is likely broken), [Absent] (dead components whose selector is
// structurally fine but the element isn't rendered in this scenario), and, for
// multi-capture sets, a [Presence] section for partial-match components.
//
// The broken/absent split uses LeafMatchAny: when the leaf selector part matches
// nodes in any capture but the full scoped selector matched none, the selector
// is broken (misconfigured ancestor scoping). When even the leaf matches nothing,
// the component is genuinely absent from this scenario \u2014 typically expected for
// step-gated UI (e.g. credit card fields on a delivery-step snapshot).
func emitUnionWarnings(viewName string, pres []viewset.ComponentPresence) {
	n := 0
	if len(pres) > 0 {
		n = pres[0].Captures
	}
	var broken, absent, partial []viewset.ComponentPresence
	for _, p := range pres {
		switch {
		case p.MatchedIn == 0 && p.LeafMatchAny:
			broken = append(broken, p)
		case p.MatchedIn == 0:
			absent = append(absent, p)
		case p.MatchedIn < n:
			partial = append(partial, p)
		}
	}

	if len(broken) > 0 {
		fmt.Println("[Warnings]")
		for _, p := range broken {
			if n > 1 {
				fmt.Printf("  %s: %s \u2014 0 matches (0 of %d snaps) \u2014 selector likely broken (leaf part matches nodes but full scoped selector does not)\n", viewName, p.Name, n)
			} else {
				fmt.Printf("  %s: %s \u2014 0 matches \u2014 selector likely broken (leaf part matches nodes but full scoped selector does not)\n", viewName, p.Name)
			}
		}
		fmt.Println()
	}

	if len(absent) > 0 {
		fmt.Println("[Absent] (not rendered in this scenario \u2014 selector is structurally valid)")
		for _, p := range absent {
			if n > 1 {
				fmt.Printf("  %s: %s \u2014 0 of %d snaps\n", viewName, p.Name, n)
			} else {
				fmt.Printf("  %s: %s\n", viewName, p.Name)
			}
		}
		fmt.Println()
	}

	if n > 1 && len(partial) > 0 {
		fmt.Println("[Presence] (matched in a subset of the view's snapshots)")
		for _, p := range partial {
			if rec := viewset.FormatStamp(p.LastMatchedStamp); rec != "" {
				fmt.Printf("  %s: %s \u2014 matched in %d of %d snaps (last %s)\n", viewName, p.Name, p.MatchedIn, n, rec)
			} else {
				fmt.Printf("  %s: %s \u2014 matched in %d of %d snaps\n", viewName, p.Name, p.MatchedIn, n)
			}
		}
		fmt.Println()
	}
}
