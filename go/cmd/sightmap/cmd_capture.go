// capture connects to Chrome, extracts the component tree, and PERSISTS it into
// the corpus as a timestamped capture in the matched view's set
// (snapshots/<view>/<stamp>.snap). It is the write-side counterpart to the
// read-only `snapshot` command.
//
// A view is a SET of captures (real pages are non-deterministic), so capture
// appends rather than overwrites. A novelty gate drops a capture that adds no
// new component type or orphan slot vs the view's existing set — the first
// capture of a view always writes, and --force bypasses the gate. Writing to an
// arbitrary file is snapshot's job (`snapshot --out FILE`); capture only ever
// targets the corpus set.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/coverage"
	"github.com/sightmap/sightmap/go/observe"
	"github.com/sightmap/sightmap/go/sightmap"
	"github.com/sightmap/sightmap/go/viewset"
)

func runCapture(args []string) error {
	fs := flag.NewFlagSet("capture", flag.ContinueOnError)
	lf := addLiveFlags(fs, "capture")
	allFlag := fs.Bool("all", false, "Capture every view URL declared in views/*.yaml")
	forceFlag := fs.Bool("force", false, "Write the capture even if it adds no new component/slot vs the view set (skip the novelty gate)")
	traceFlag := fs.Bool("trace", false, "Include selector hints for unlabeled interactive clusters in the saved capture")
	selectorsFlag := fs.Bool("selectors", false, "Show tag #id [data-testid] selector hints in the saved capture")
	includeHiddenFlag := fs.Bool("include-hidden", false, "Include hidden/off-screen nodes in analysis")
	jsonOutFlag := fs.Bool("json", false, "Also write an annotated JSON sibling next to each capture")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Apply .sightmap/config.yaml defaults for flags not explicitly set.
	{
		explicit := map[string]bool{}
		fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
		cfg := sightmap.LoadConfig(*lf.sightmapDir)
		if !explicit["wait"] && cfg.Snapshot.Wait > 0 {
			*lf.wait = cfg.Snapshot.Wait
		}
		if !explicit["include-hidden"] && cfg.Snapshot.IncludeHidden {
			*includeHiddenFlag = true
		}
		if !explicit["trace"] && cfg.Snapshot.Trace {
			*traceFlag = true
		}
	}

	visible := !*includeHiddenFlag

	if *allFlag {
		return runCaptureAll(*lf.sightmapDir, *lf.addr, *lf.tab, *lf.wait,
			visible, *selectorsFlag, *jsonOutFlag, *forceFlag)
	}

	ctx := context.Background()

	conn, cleanup, err := lf.connect(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := lf.navigate(ctx, conn, navOpts{idle: true}); err != nil {
		return err
	}

	// ── Observe (require a matching view) ───────────────────────────────
	corpus, cErr := lf.requireCorpus()
	if cErr != nil {
		return cErr
	}
	res, err := observe.Page(ctx, conn, corpus, observe.Options{VisibleOnly: visible})
	if err != nil {
		return fmt.Errorf("capture: observe: %v", err)
	}
	if res.View == nil {
		return fmt.Errorf("capture: no view matches %q — capture appends to a view's set; "+
			"use `snapshot` to observe an unmapped page, or add a view with a matching route", res.URL)
	}
	fmtOpts := observe.FormatOpts{Trace: *traceFlag, Selectors: *selectorsFlag}

	// ── Novelty gate ──────────────────────────────────────────────────────────
	viewBasename := res.View.SnapBasename()
	cov := res.Coverage
	if !*forceFlag {
		cand := viewset.SlotsFromMatch(res.Matches, cov.Orphans, cov.ParentMap)
		if gres, write := viewset.Gate(corpus, *lf.sightmapDir, viewBasename, cand, false); !write {
			fmt.Fprintf(os.Stderr, "capture: nothing new vs %d capture(s) in %s — not saved (use --force to keep)\n",
				gres.ComparedTo, viewBasename)
			return nil
		}
	}

	// ── Write the capture into the view's set ─────────────────────────────────
	snapPath := viewset.CapturePath(*lf.sightmapDir, viewBasename, time.Now())
	if err := writeCapture(snapPath, res, fmtOpts); err != nil {
		return err
	}
	if *jsonOutFlag {
		jsonPath := strings.TrimSuffix(snapPath, ".snap") + ".snap.annotated.json"
		if err := writeAnnotatedJSON(res.Root, jsonPath, res.View, res.Matches, res.Props); err != nil {
			fmt.Fprintf(os.Stderr, "capture: json: %v\n", err)
		}
	}

	fmt.Fprintf(os.Stderr, "capture saved: %s\n  %d interactive · T1 %d%% · T2 %d%% · T3 %d\n",
		snapPath, cov.Total, coverage.Pct(cov.T1, cov.Total), coverage.Pct(cov.T2, cov.Total), cov.T3)
	return nil
}

// runCaptureAll navigates to each declared view URL and appends a novelty-gated
// capture per view, then prints a summary table.
func runCaptureAll(
	sightmapDir, addr, tabID string,
	wait float64,
	visible bool,
	selectors bool,
	jsonOut bool,
	force bool,
) error {
	corpus, err := sightmap.Load(sightmapDir)
	if err != nil {
		return fmt.Errorf("capture --all: load corpus: %v", err)
	}
	targets, err := corpus.ProbeTargets()
	if err != nil {
		return err
	}

	ctx := context.Background()
	conn, err := browser.Connect(addr, tabID)
	if err != nil {
		return fmt.Errorf("capture --all: %w", err)
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
		fmt.Fprintf(os.Stderr, "capturing %s → %s\n", label, v.URL)

		if err := browser.NavigateAndWaitIdle(ctx, conn, v.URL, 8*time.Second); err != nil {
			results = append(results, result{name: label, err: err})
			continue
		}
		if wait > 0 {
			time.Sleep(time.Duration(wait * float64(time.Second)))
		}

		res, err := observe.Page(ctx, conn, corpus, observe.Options{VisibleOnly: visible})
		if err != nil {
			results = append(results, result{name: label, err: err})
			continue
		}
		fmtOpts := observe.FormatOpts{Selectors: selectors}
		cov := res.Coverage

		// Novelty gate: don't append a redundant capture to the view set.
		if !force {
			cand := viewset.SlotsFromMatch(res.Matches, cov.Orphans, cov.ParentMap)
			if gres, write := viewset.Gate(corpus, sightmapDir, v.ViewDir, cand, false); !write {
				results = append(results, result{name: label, skipped: true, skippedVs: gres.ComparedTo})
				continue
			}
		}

		snapPath := viewset.CapturePath(sightmapDir, v.ViewDir, time.Now())
		if err := writeCapture(snapPath, res, fmtOpts); err != nil {
			results = append(results, result{name: label, err: err})
			continue
		}
		if jsonOut {
			jsonPath := strings.TrimSuffix(snapPath, ".snap") + ".snap.annotated.json"
			writeAnnotatedJSON(res.Root, jsonPath, res.View, res.Matches, nil) // no propValues in --all mode
		}

		results = append(results, result{name: label, t1: cov.T1, t2: cov.T2, t3: cov.T3, total: cov.Total})
	}

	fmt.Fprintln(os.Stderr)
	for _, r := range results {
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "%-20s  ERROR: %v\n", r.name, r.err)
			continue
		}
		if r.skipped {
			fmt.Fprintf(os.Stderr, "%-20s  = nothing new vs %d capture(s) — not saved\n", r.name, r.skippedVs)
			continue
		}
		check := "✓"
		if r.t3 > 0 {
			check = "✗"
		}
		fmt.Fprintf(os.Stderr, "%-20s  %d interactive · T1 %d%% · T2 %d%% · T3 %d %s\n",
			r.name, r.total,
			coverage.Pct(r.t1, r.total),
			coverage.Pct(r.t2, r.total),
			r.t3, check,
		)
	}
	return nil
}

// writeCapture renders a capture to snapPath (atomically) plus its .tree.json
// sibling, creating parent directories as needed. This is the persistence
// primitive shared by the single-view and --all capture paths.
func writeCapture(snapPath string, res *observe.Result, opts observe.FormatOpts) error {
	if err := writeOut(snapPath, func(w io.Writer) error {
		observe.Format(w, res, opts)
		return nil
	}); err != nil {
		return err
	}
	if err := writeTreeJSON(res.Root, viewset.TreePath(snapPath)); err != nil {
		fmt.Fprintf(os.Stderr, "capture: tree-out: %v\n", err)
	}
	return nil
}
