// report reads each view's snapshot SET, loads the saved captures, and prints a
// per-view coverage health table plus T2 quality analysis. A view is a set of
// timestamped captures, so report aggregates over the whole set
// : T1/T2 percentages are summed across the set and recomputed, and the
// T3 orphan gate is the MAX across captures — which makes report's ✓/✗ agree with
// `coverage`'s union verdict (both fail iff some capture has an orphan).
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/sightmap/sightmap/go/sightmap"
)

// captureCoverage is one capture's per-DOM coverage tiers plus its largest T2
// scope, the unit report aggregates over a view's set.
type captureCoverage struct {
	t1, t2, t3, total int
	worstT2Name       string
	worstT2Count      int
}

// viewCoverageAgg is report's per-view rollup over a view's capture set
// . T1/T2 are summed node counts (rendered as % over sumTotal), T3 is
// the MAX single-capture orphan count (the ✓/✗ gate), and snaps is the set size.
type viewCoverageAgg struct {
	snaps                  int
	sumT1, sumT2, sumTotal int
	maxT3                  int
	worstT2Name            string
	worstT2Count           int
}

func (a viewCoverageAgg) t1pct() int { return coveragePct(a.sumT1, a.sumTotal) }
func (a viewCoverageAgg) t2pct() int { return coveragePct(a.sumT2, a.sumTotal) }

// aggregateViewCoverage folds a view's per-capture coverage into the report
// rollup: sum T1/T2/total, take the max T3 (so the gate trips iff ANY capture had
// an orphan — matching coverage's union fail condition), and keep the single
// largest T2 scope seen across the set.
func aggregateViewCoverage(caps []captureCoverage) viewCoverageAgg {
	agg := viewCoverageAgg{snaps: len(caps)}
	for _, c := range caps {
		agg.sumT1 += c.t1
		agg.sumT2 += c.t2
		agg.sumTotal += c.total
		if c.t3 > agg.maxT3 {
			agg.maxT3 = c.t3
		}
		if c.worstT2Count > agg.worstT2Count {
			agg.worstT2Count = c.worstT2Count
			agg.worstT2Name = c.worstT2Name
		}
	}
	return agg
}

// t2clusterHit is one T2 cluster observed in some capture of some view, fed to the
// cross-view T2-quality section.
type t2clusterHit struct {
	view  string
	name  string
	count int
}

func runReport(args []string) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := loadSiteConfig(*sightmapDirFlag)

	// Load corpus to get views with URLs
	loader := sightmap.DirLoader(*sightmapDirFlag)
	corpus, err := loader.Load()
	if err != nil {
		return fmt.Errorf("load corpus: %v", err)
	}

	// Filter to views that have URLs
	var views []sightmap.View
	for _, v := range corpus.Views {
		if v.URL != "" {
			views = append(views, v)
		}
	}

	if len(views) == 0 {
		return fmt.Errorf("no views with URLs found\n" +
			"Add a url: field to your views/*.yaml files")
	}

	// visibleOnly: visible-by-default; overridden by include_hidden: true in config.
	visibleOnly := cfg.VisibleOnly()

	sess := sightmap.NewSession(sightmap.DirLoader(*sightmapDirFlag))

	type viewResult struct {
		name    string
		url     string
		route   string
		missing bool
		agg     viewCoverageAgg
	}

	var results []viewResult
	var t2hits []t2clusterHit // every T2 cluster across every capture (for the T2-quality block)

	for _, v := range views {
		// Resolve the whole capture set for the view and aggregate over it
		//; previously report used only the latest capture.
		set := viewSnapshotSet(*sightmapDirFlag, v.SnapBasename())
		if len(set) == 0 {
			results = append(results, viewResult{name: v.Name, url: v.URL, missing: true})
			continue
		}

		var caps []captureCoverage
		route := ""
		for _, e := range set {
			root, matches, capRoute, ok := matchCapture(e.Path, sess)
			if !ok {
				continue
			}
			if route == "" {
				route = capRoute
			}
			parentMap := buildParentMap(root)
			t1, t2, t3, total, _, t2c, _ := computeCoverage(root, matches, parentMap, visibleOnly)

			cc := captureCoverage{t1: t1, t2: t2, t3: t3, total: total}
			for node, count := range t2c {
				m := matches[node]
				if m == nil || m.Name == "" {
					continue
				}
				if count > cc.worstT2Count {
					cc.worstT2Count = count
					cc.worstT2Name = m.Name
				}
				t2hits = append(t2hits, t2clusterHit{view: v.Name, name: m.Name, count: count})
			}
			caps = append(caps, cc)
		}

		if len(caps) == 0 {
			results = append(results, viewResult{name: v.Name, url: v.URL, missing: true})
			continue
		}
		if route == "" {
			route = v.URL
		}
		results = append(results, viewResult{
			name:  v.Name,
			url:   v.URL,
			route: route,
			agg:   aggregateViewCoverage(caps),
		})
	}

	// ── Header ────────────────────────────────────────────────────────────────
	wd, _ := os.Getwd()
	site := lastPathComponent(wd)
	fmt.Printf("sightmap report · %s · %s\n", site, time.Now().Format("2006-01-02"))

	// ── Compute column widths ─────────────────────────────────────────────────
	const t2WarnThreshold = 15 // ⚠ when worst T2 scope ≥ this

	nameW := len("View")
	routeW := len("Route")
	for _, r := range results {
		if w := len(r.name); w > nameW {
			nameW = w
		}
		if w := len(routeDisplay(r.route, r.url)); w > routeW {
			routeW = w
		}
	}
	if nameW > 30 {
		nameW = 30
	}
	if routeW > 35 {
		routeW = 35
	}

	// ── Table header ─────────────────────────────────────────────────────────
	sep := strings.Repeat("─", nameW+routeW+50)
	fmt.Println(sep)
	fmt.Printf(" %-*s  %-*s  %5s  %4s  %4s  %3s  %s\n",
		nameW, "View", routeW, "Route", "Snaps", "T1", "T2", "T3", "Largest T2 scope")
	fmt.Println(sep)

	// ── Rows ──────────────────────────────────────────────────────────────────
	ok, fail, missing := 0, 0, 0
	sumT1, sumT2, sumTotal := 0, 0, 0

	for _, r := range results {
		name := truncate(r.name, nameW)
		route := truncate(routeDisplay(r.route, r.url), routeW)

		if r.missing {
			missing++
			fmt.Printf(" %-*s  %-*s  %s\n",
				nameW, name, routeW, route,
				"MISSING – run: sightmap capture --all")
			continue
		}

		check := "✓"
		if r.agg.maxT3 > 0 {
			check = "✗"
			fail++
		} else {
			ok++
		}

		sumT1 += r.agg.sumT1
		sumT2 += r.agg.sumT2
		sumTotal += r.agg.sumTotal

		t2scope := "─"
		if r.agg.worstT2Name != "" {
			t2scope = fmt.Sprintf("[%s] (%d)", r.agg.worstT2Name, r.agg.worstT2Count)
			if r.agg.worstT2Count >= t2WarnThreshold {
				t2scope += " ⚠"
			}
		}

		fmt.Printf(" %-*s  %-*s  %5d  %3d%%  %3d%%  %3d %s  %s\n",
			nameW, name,
			routeW, route,
			r.agg.snaps,
			r.agg.t1pct(),
			r.agg.t2pct(),
			r.agg.maxT3, check,
			t2scope,
		)
	}

	// ── Summary line ──────────────────────────────────────────────────────────
	fmt.Println(sep)
	total := ok + fail + missing
	statusLine := fmt.Sprintf("%d/%d views ✓", ok, total)
	if fail > 0 {
		statusLine += fmt.Sprintf("  ·  %d ✗", fail)
	}
	if missing > 0 {
		statusLine += fmt.Sprintf("  ·  %d missing", missing)
	}
	if sumTotal > 0 {
		statusLine += fmt.Sprintf("  ·  avg T1 %d%%  ·  avg T2 %d%%",
			coveragePct(sumT1, sumTotal),
			coveragePct(sumT2, sumTotal),
		)
	}
	fmt.Printf(" %s\n", statusLine)

	// ── T2 quality analysis ───────────────────────────────────────────────────
	// Aggregate T2 clusters across ALL captures of all views, keyed by component
	// name (was latest-capture-only, now the whole set).
	printT2Quality(t2hits, t2WarnThreshold)

	if fail > 0 || missing > 0 {
		return fmt.Errorf("%d view(s) with issues", fail+missing)
	}
	return nil
}

// printT2Quality prints the cross-view "add child components" T2-quality table,
// aggregating cluster hits by component name (top 15 by largest single scope).
func printT2Quality(hits []t2clusterHit, warnThreshold int) {
	type t2entry struct {
		name     string
		total    int
		views    []string
		maxCount int
	}
	byName := map[string]*t2entry{}
	for _, h := range hits {
		if h.count < 3 || h.name == "" {
			continue
		}
		e := byName[h.name]
		if e == nil {
			e = &t2entry{name: h.name}
			byName[h.name] = e
		}
		e.total += h.count
		e.views = append(e.views, h.view)
		if h.count > e.maxCount {
			e.maxCount = h.count
		}
	}
	if len(byName) == 0 {
		return
	}

	entries := make([]*t2entry, 0, len(byName))
	for _, e := range byName {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].maxCount != entries[j].maxCount {
			return entries[i].maxCount > entries[j].maxCount
		}
		return entries[i].name < entries[j].name
	})
	if len(entries) > 15 {
		entries = entries[:15]
	}

	fmt.Printf("\nT2 quality (add child components to improve coverage quality):\n")
	for _, e := range entries {
		warn := ""
		if e.maxCount >= warnThreshold {
			warn = " ⚠"
		}
		viewList := uniqueStrings(e.views)
		viewDisp := strings.Join(viewList, ", ")
		if len(viewList) > 2 {
			viewDisp = fmt.Sprintf("%d views", len(viewList))
		}
		fmt.Printf("  %3d  [%s]%s  (%s)\n", e.maxCount, e.name, warn, viewDisp)
	}
}

// routeDisplay returns the display string for the route column.
// Uses the snap route if available, otherwise falls back to the probe URL.
func routeDisplay(route, url string) string {
	if route != "" {
		return route
	}
	return url
}

// truncate shortens s to at most n runes, appending "…" if truncated.
// (Uses truncateStr from cmd_snapshot.go for consistency.)
func truncate(s string, n int) string {
	return truncateStr(s, n)
}

// lastPathComponent returns the last path segment of s.
func lastPathComponent(s string) string {
	s = strings.TrimRight(s, "/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// uniqueStrings returns a deduplicated, order-preserving copy of ss.
func uniqueStrings(ss []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
