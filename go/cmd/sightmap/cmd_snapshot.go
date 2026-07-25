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

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/observe"
)

// snapshot is a PURE OBSERVE command: it connects, navigates, extracts, matches
// the corpus, and renders the annotated tree + [Coverage] to stdout (or --out
// FILE). It never writes into the corpus's capture set and never novelty-gates —
// persisting a capture is the separate `capture` command's job.
func runSnapshot(args []string) error {
	fs := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	lf := addLiveFlags(fs, "snapshot")
	outFlag := fs.String("out", "", "Write the annotated output to this file instead of stdout")
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
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Apply .sightmap/config.yaml defaults for flags not explicitly set.
	{
		explicit := map[string]bool{}
		fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
		cfg := loadSiteConfig(*lf.sightmapDir)
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

	visible := *visibleFlag
	if *includeHiddenFlag {
		visible = false
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

	// ── Observe (extract + match + coverage + properties) ──────────────────────
	corpus, cErr := lf.loadCorpus()
	if cErr != nil {
		fmt.Fprintf(os.Stderr, "snapshot: load corpus: %v\n", cErr)
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
	return writeOut(*outFlag, func(w io.Writer) error {
		observe.Format(w, res, fmtOpts)
		return nil
	})
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
