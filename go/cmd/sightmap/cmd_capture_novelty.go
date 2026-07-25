// capture-novelty — does a candidate capture add anything NEW relative to the
// existing union of its view's set?
//
// "New" is purely STRUCTURAL and corpus-relative: a new component TYPE matched,
// or a new uncovered-interactive SLOT (orphanSlotKey) — never instance/value
// churn (different products, prices, copy), which is deliberately ignored so
// dynamic pages can saturate. Every capture is re-matched against the CURRENT
// corpus, so a capture that looks novel today (because the corpus is incomplete
// and it has unique orphans) stops being novel once components are authored to
// cover those orphans — which is what makes after-the-fact dedup/merge possible.
//
// This is the primitive; capture-time gating and the subsumption
// prune/merge pass build on it.
package main

import (
	"flag"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sightmap/sightmap/go/sightmap"
	"github.com/sightmap/sightmap/go/viewset"
)

func runCaptureNovelty(args []string) error {
	fs := flag.NewFlagSet("capture-novelty", flag.ContinueOnError)
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("usage: sightmap capture-novelty [--sightmap-dir DIR] <candidate.snap>")
	}

	candidate := strings.TrimSuffix(rest[0], ".tree.json")
	view, _, ok := viewset.ParsePath(candidate)
	if !ok {
		return fmt.Errorf("capture-novelty: not a capture path: %s", rest[0])
	}

	corpus, err := sightmap.Load(*sightmapDirFlag)
	if err != nil {
		return fmt.Errorf("capture-novelty: load corpus: %v", err)
	}

	candSlots, ok := viewset.SlotsForCapture(candidate, corpus)
	if !ok {
		return fmt.Errorf("capture-novelty: cannot read candidate tree (%s.tree.json)", candidate)
	}

	// The rest of the candidate's view set, re-matched against the current corpus
	// (the candidate itself excluded).
	others := viewset.ViewSlots(*sightmapDirFlag, view, corpus, candidate)

	res := viewset.ComputeNovelty(candSlots, others)
	printNovelty(view, filepath.Base(candidate), res)
	return nil
}

func printNovelty(view, candName string, res viewset.Novelty) {
	fmt.Printf("%s · candidate %s vs %d existing capture(s)\n\n",
		view, candName, res.ComparedTo)

	if res.ComparedTo == 0 {
		fmt.Printf("first capture of this view — keep (nothing to compare against; %d component(s), %d orphan slot(s))\n",
			len(res.NovelComponents), len(res.NovelOrphans))
		return
	}
	if !res.IsNovel() {
		fmt.Printf("nothing new vs %d capture(s) — redundant.\n", res.ComparedTo)
		return
	}

	if len(res.NovelComponents) > 0 {
		fmt.Println("Novel components (matched here, in no existing capture):")
		for _, c := range res.NovelComponents {
			fmt.Printf("  %s\n", c)
		}
		fmt.Println()
	}
	if len(res.NovelOrphans) > 0 {
		fmt.Println("Novel orphan slots (uncovered interactive nodes, new vs the set):")
		keys := make([]string, 0, len(res.NovelOrphans))
		for k := range res.NovelOrphans {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if res.NovelOrphans[keys[i]] != res.NovelOrphans[keys[j]] {
				return res.NovelOrphans[keys[i]] > res.NovelOrphans[keys[j]]
			}
			return keys[i] < keys[j]
		})
		for _, k := range keys {
			fmt.Printf("  %d× %s\n", res.NovelOrphans[k], k)
		}
		fmt.Println()
	}

	noun := func(n int, s string) string {
		if n == 1 {
			return s
		}
		return s + "s"
	}
	fmt.Printf("→ NOVEL: adds %d %s, %d orphan %s — keep.\n",
		len(res.NovelComponents), noun(len(res.NovelComponents), "component"),
		len(res.NovelOrphans), noun(len(res.NovelOrphans), "slot"))
}
