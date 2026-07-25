// snapshot connects to Chrome (or launches it), runs the full
// component-extraction pipeline, applies the .sightmap/ corpus, and emits an
// annotated ARIA tree together with coverage statistics.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/observe"
	"github.com/sightmap/sightmap/go/sightmap"
)

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

	conn, dialErr := browser.Connect(*addrFlag, *tabFlag)
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

	// ── Observe (extract + match + coverage + properties) ──────────────────────
	var corpus *sightmap.Corpus
	if _, statErr := os.Stat(*sightmapDirFlag); statErr == nil {
		if c, cErr := sightmap.Load(*sightmapDirFlag); cErr != nil {
			fmt.Fprintf(os.Stderr, "snapshot: load corpus: %v\n", cErr)
		} else {
			corpus = c
		}
	}
	res, err := observe.Page(ctx, conn, corpus, observe.Options{VisibleOnly: visible, ExtractProps: true})
	if err != nil {
		return fmt.Errorf("snapshot: observe: %v", err)
	}

	fmtOpts := observe.FormatOpts{
		Interactive:  *interactiveFlag,
		MaxDepth:     *depthFlag,
		Trace:        *traceFlag,
		CoverageOnly: *coverageOnlyFlag,
		Selectors:    *selectorsFlag,
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

	// ── Side JSON artifacts (for offline coverage tools) ───────────────────────
	if *treeOutFlag != "" {
		if err := writeTreeJSON(res.Root, *treeOutFlag); err != nil {
			fmt.Fprintf(os.Stderr, "snapshot: tree-out: %v\n", err)
		}
	}
	if *jsonOutFlag != "" {
		if err := writeAnnotatedJSON(res.Root, *jsonOutFlag, res.View, res.Matches, res.Props); err != nil {
			fmt.Fprintf(os.Stderr, "snapshot: json-out: %v\n", err)
		}
	}

	// ── Write output ───────────────────────────────────────────────────────────
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
		observe.Format(f, res, fmtOpts)
		if cerr := f.Close(); cerr != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("close output file: %v", cerr)
		}
		if rerr := os.Rename(tmpPath, *outFlag); rerr != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("rename output file: %v", rerr)
		}
	} else {
		observe.Format(os.Stdout, res, fmtOpts)
	}
	return nil
}

// ── Output sections ───────────────────────────────────────────────────────────

// ── Property extraction ──────────────────────────────────────────────────────

// ── Formatting helpers ────────────────────────────────────────────────────────

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
