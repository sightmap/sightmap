package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sightmap/sightmap/go/sightmap"
)

func runLint(args []string) error {
	fs := flag.NewFlagSet("lint", flag.ContinueOnError)
	sightmapDir := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ directory")
	warnOnly := fs.Bool("warn-only", false, "Always exit 0 even if warnings exist")
	snapshotFlag := fs.String("snapshot", "", "Path to a .snap (or .snap.tree.json) file; augments lint with match counts")
	allSnapshotsFlag := fs.Bool("all-snapshots", false, "Walk .sightmap/snapshots/**/*.snap.tree.json and augment lint with match counts")
	if err := fs.Parse(args); err != nil {
		return err
	}

	corpus, err := sightmap.Load(*sightmapDir)
	if err != nil {
		return fmt.Errorf("load corpus: %w", err)
	}

	// Collect tree JSON paths from flags.
	var treeFiles []string
	if *snapshotFlag != "" {
		tf := *snapshotFlag
		if !strings.HasSuffix(tf, ".tree.json") {
			tf = tf + ".tree.json"
		}
		treeFiles = append(treeFiles, tf)
	}
	if *allSnapshotsFlag {
		found, findErr := findLintSnapshotTreeFiles(*sightmapDir)
		if findErr != nil {
			return fmt.Errorf("find snapshots: %w", findErr)
		}
		treeFiles = append(treeFiles, found...)
	}

	var warnings []sightmap.LintWarning
	if len(treeFiles) > 0 {
		counts, cErr := computeSnapshotCounts(corpus, treeFiles)
		if cErr != nil {
			return fmt.Errorf("snapshot counts: %w", cErr)
		}
		warnings = sightmap.LintWithCounts(corpus, counts)
	} else {
		warnings = sightmap.Lint(corpus)
	}

	if len(warnings) == 0 {
		fmt.Fprintf(os.Stderr, "✓ no lint warnings\n")
		return nil
	}
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "warn [%s]: %s\n", w.Rule, w.String())
	}
	if *warnOnly {
		return nil
	}
	return fmt.Errorf("%d lint warning(s)", len(warnings))
}

// findLintSnapshotTreeFiles walks sightmapDir/snapshots/ and returns all
// .snap.tree.json files found.
func findLintSnapshotTreeFiles(sightmapDir string) ([]string, error) {
	snapshotsDir := filepath.Join(sightmapDir, "snapshots")
	var treeFiles []string
	err := filepath.Walk(snapshotsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".snap.tree.json") {
			treeFiles = append(treeFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", snapshotsDir, err)
	}
	return treeFiles, nil
}

// computeSnapshotCounts walks each tree JSON file and counts, for every
// component in the corpus, how many tree nodes match the component's selector
// (last part only). It returns a map of component name → max count across all
// provided tree files and all selectors for that component.
//
// Components absent from any of the tree files will have a count of 0 in the
// returned map.
func computeSnapshotCounts(corpus *sightmap.Corpus, treeFiles []string) (map[string]int, error) {
	// Pre-parse selectors: component index → slice of last SelectorParts.
	type parsedEntry struct {
		name      string
		lastParts []*sightmap.SelectorPart
	}
	all := corpus.AllComponents()
	parsed := make([]parsedEntry, 0, len(all))
	for _, comp := range all {
		var lasts []*sightmap.SelectorPart
		for _, selStr := range comp.Selectors {
			ps, err := sightmap.ParseSightmapSelector(selStr)
			if err != nil || len(ps.Parts) == 0 {
				continue
			}
			lasts = append(lasts, ps.Parts[len(ps.Parts)-1])
		}
		parsed = append(parsed, parsedEntry{name: comp.Name, lastParts: lasts})
	}

	// counts starts at 0 for every component (explicit presence distinguishes
	// "0 matches in snapshot" from "component not checked").
	counts := make(map[string]int, len(parsed))
	for _, pe := range parsed {
		counts[pe.name] = 0
	}

	for _, tf := range treeFiles {
		data, err := os.ReadFile(tf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lint: %s: tree file not found, skipping\n", tf)
			continue
		}
		var root sightmap.ComponentNode
		if err := json.Unmarshal(data, &root); err != nil {
			fmt.Fprintf(os.Stderr, "lint: %s: parse tree JSON: %v, skipping\n", tf, err)
			continue
		}

		// For each component, count matching nodes using its last selector parts.
		for _, pe := range parsed {
			if len(pe.lastParts) == 0 {
				continue
			}
			maxForTree := 0
			for _, lastPart := range pe.lastParts {
				count := 0
				sightmap.Walk(&root, func(node *sightmap.ComponentNode, _ int) bool {
					if sightmap.MatchesNode(node, lastPart) {
						count++
					}
					return true
				})
				if count > maxForTree {
					maxForTree = count
				}
			}
			if maxForTree > counts[pe.name] {
				counts[pe.name] = maxForTree
			}
		}
	}

	return counts, nil
}
