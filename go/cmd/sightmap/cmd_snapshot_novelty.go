// snapshot-novelty — does a candidate capture add anything NEW relative to the
// existing union of its view's set? (go-snap.3)
//
// "New" is purely STRUCTURAL and corpus-relative: a new component TYPE matched,
// or a new uncovered-interactive SLOT (orphanSlotKey) — never instance/value
// churn (different products, prices, copy), which is deliberately ignored so
// dynamic pages can saturate. Every capture is re-matched against the CURRENT
// corpus, so a capture that looks novel today (because the corpus is incomplete
// and it has unique orphans) stops being novel once components are authored to
// cover those orphans — which is what makes after-the-fact dedup/merge possible.
//
// This is the primitive; capture-time gating (go-snap.7) and the subsumption
// prune/merge pass build on it.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

func runSnapshotNovelty(args []string) error {
	fs := flag.NewFlagSet("snapshot-novelty", flag.ContinueOnError)
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("usage: sightmap snapshot-novelty [--sightmap-dir DIR] <candidate.snap>")
	}

	candidate := strings.TrimSuffix(rest[0], ".tree.json")
	view, _, ok := parseSnapshotPath(candidate)
	if !ok {
		return fmt.Errorf("snapshot-novelty: not a snapshot path: %s", rest[0])
	}

	sess := sightmap.NewSession(sightmap.DirLoader(*sightmapDirFlag))

	candSlots, ok := extractCaptureSlots(candidate, sess)
	if !ok {
		return fmt.Errorf("snapshot-novelty: cannot read candidate tree (%s.tree.json)", candidate)
	}

	// The rest of the candidate's view set, re-matched against the current corpus
	// (the candidate itself excluded).
	others := loadViewSlots(*sightmapDirFlag, view, sess, candidate)

	res := computeNovelty(candSlots, others)
	printNovelty(view, filepath.Base(candidate), res)
	return nil
}

// slotsFromMatch builds a capture's structural fingerprint from an already-matched
// tree: the component TYPES matched plus the orphan SLOTS left uncovered. Shared
// by the offline path (extractCaptureSlots) and the live capture path
// (snapshot / captureSnapshot) so both gate on the same notion of "new".
func slotsFromMatch(
	matches map[*comps.ComponentNode]*match.SightmapMatch,
	t3nodes []*comps.ComponentNode,
	parentMap map[*comps.ComponentNode]*comps.ComponentNode,
) captureSlots {
	cs := captureSlots{Matched: map[string]bool{}, Orphans: map[string]int{}}
	for _, m := range matches {
		if m != nil && m.Name != "" {
			cs.Matched[m.Name] = true
		}
	}
	for _, n := range t3nodes {
		cs.Orphans[orphanSlotKey(n, parentMap)]++
	}
	return cs
}

// extractCaptureSlots re-matches a saved capture against the current corpus and
// returns its component-type / orphan-slot fingerprint. Visible nodes only,
// matching the coverage default.
func extractCaptureSlots(snapPath string, sess *sightmap.Session) (captureSlots, bool) {
	data, err := os.ReadFile(snapPath + ".tree.json")
	if err != nil {
		return captureSlots{}, false
	}
	var root comps.ComponentNode
	if json.Unmarshal(data, &root) != nil {
		return captureSlots{}, false
	}
	matches, err := sess.MatchTree(&root, parseSnapRoute(snapPath))
	if err != nil {
		return captureSlots{}, false
	}
	parentMap := buildParentMap(&root)
	_, _, _, _, t3nodes, _, _ := computeCoverage(&root, matches, parentMap, true)
	return slotsFromMatch(matches, t3nodes, parentMap), true
}

// loadViewSlots re-matches every capture currently in the view's set against the
// current corpus, optionally excluding one path.
func loadViewSlots(sightmapDir, viewBasename string, sess *sightmap.Session, excludePath string) []captureSlots {
	all, _ := findSnapshots(sightmapDir, nil)
	entries := groupSnapshotsByView(all)[viewBasename]
	excl := filepath.ToSlash(excludePath)
	var out []captureSlots
	for _, e := range entries {
		if excl != "" && filepath.ToSlash(e.Path) == excl {
			continue
		}
		if cs, ok := extractCaptureSlots(e.Path, sess); ok {
			cs.Stamp = e.Stamp
			out = append(out, cs)
		}
	}
	return out
}

// noveltyGate decides whether a freshly extracted capture should be written to
// its view's set (go-snap.7). The first capture of a view always writes; force
// bypasses the gate. Returns the novelty result and the write decision.
func noveltyGate(sightmapDir, viewBasename string, cand captureSlots, force bool) (noveltyResult, bool) {
	sess := sightmap.NewSession(sightmap.DirLoader(sightmapDir))
	others := loadViewSlots(sightmapDir, viewBasename, sess, "")
	res := computeNovelty(cand, others)
	return res, force || len(others) == 0 || res.IsNovel()
}

func printNovelty(view, candName string, res noveltyResult) {
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
