// capture-prune — drop captures that no longer add anything to their view's set,
// the retrospective counterpart to the capture-time novelty gate.
//
// Because novelty is corpus-relative and re-computed against the CURRENT corpus,
// a capture kept while the corpus was incomplete (it had unique orphans) becomes
// redundant once components are authored to cover those orphans. This pass
// re-matches every capture in a view against the current corpus and removes the
// ones that are SUBSUMED — i.e. removing them does not shrink the union (their
// component types and orphan slots are all present in some retained capture).
// Conservative: never chases a minimal cover, never drops the last capture.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sightmap/sightmap/go/sightmap"
	"github.com/sightmap/sightmap/go/viewset"
)

func runCapturePrune(args []string) error {
	fs := flag.NewFlagSet("capture-prune", flag.ContinueOnError)
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir")
	dryRunFlag := fs.Bool("dry-run", false, "Report what would be pruned without deleting")
	allFlag := fs.Bool("all", false, "Prune every view with captures")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if (*allFlag) == (len(rest) == 1) {
		return fmt.Errorf("usage: sightmap capture-prune [--dry-run] (<view> | --all)")
	}

	corpus, err := sightmap.Load(*sightmapDirFlag)
	if err != nil {
		return fmt.Errorf("capture-prune: load corpus: %v", err)
	}
	all, _ := viewset.Find(*sightmapDirFlag, nil)
	sets := viewset.GroupByView(all)

	var views []string
	if *allFlag {
		for v := range sets {
			views = append(views, v)
		}
	} else {
		views = []string{rest[0]}
	}

	totalPruned := 0
	for _, view := range views {
		totalPruned += pruneView(view, sets[view], corpus, *dryRunFlag)
	}
	if *dryRunFlag && totalPruned > 0 {
		fmt.Printf("\n(dry run \u2014 nothing deleted; re-run without --dry-run to prune)\n")
	}
	return nil
}

// pruneView re-matches a view's captures against the current corpus and removes
// the subsumed ones. Returns how many were pruned (or would be, under
// --dry-run).
func pruneView(view string, entries []viewset.Entry, corpus *sightmap.Corpus, dryRun bool) int {
	paths := make([]string, 0, len(entries))
	caps := make([]viewset.Slots, 0, len(entries))
	for _, e := range entries {
		cs, ok := viewset.SlotsForCapture(e.Path, corpus, true)
		if !ok {
			continue
		}
		cs.Stamp = e.Stamp
		paths = append(paths, e.Path)
		caps = append(caps, cs)
	}
	if len(caps) <= 1 {
		return 0
	}

	prune := viewset.PlanPrune(caps)
	if len(prune) == 0 {
		fmt.Printf("%s: %d capture(s), nothing redundant \u2014 kept all\n", view, len(caps))
		return 0
	}

	verb := "pruned"
	if dryRun {
		verb = "would prune"
	}
	fmt.Printf("%s: %d capture(s) \u2192 keep %d, %s %d\n", view, len(caps), len(caps)-len(prune), verb, len(prune))
	for _, idx := range prune {
		stamp := viewset.FormatStamp(caps[idx].Stamp)
		if stamp == "" {
			stamp = "(untimestamped)"
		}
		fmt.Printf("  %s %s\n", verb, stamp)
		if !dryRun {
			if err := os.Remove(paths[idx]); err != nil && !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "  warning: %v\n", err)
			}
			tree := viewset.TreePath(paths[idx])
			if err := os.Remove(tree); err != nil && !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "  warning: %v\n", err)
			}
		}
	}
	return len(prune)
}
