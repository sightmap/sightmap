package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sightmap/sightmap/go/authoring"
	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

func runSuggest(args []string) error {
	fs := flag.NewFlagSet("suggest", flag.ContinueOnError)
	lf := addLiveFlags(fs, "suggest")
	maxFlag := fs.Int("max", 20, "Max candidates to show")
	excludeKnownFlag := fs.Bool("exclude-known", false, "Exclude elements already matched by a known sightmap component")
	minCountFlag := fs.Int("min-count", 1, "Only show candidates matching >= N elements")
	groupFlag := fs.Bool("group", false, "Group candidates by nearest known ancestor component")
	snapFlag := fs.String("snap", "", "Path to .snap.tree.json for grouping (auto-detected if omitted)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := context.Background()

	conn, cleanup, err := lf.connect(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	// ── Scan the live DOM for selector candidates ───────────────────────
	candidates, err := authoring.ScanCandidates(ctx, conn, *maxFlag)
	if err != nil {
		return fmt.Errorf("suggest: %v", err)
	}

	// ── Optionally filter out known sightmap components ────────────────────
	var knownSelectors []string
	if *excludeKnownFlag {
		if corpus, cErr := lf.loadCorpus(); cErr != nil {
			fmt.Fprintf(os.Stderr, "suggest: load corpus: %v (continuing without --exclude-known filtering)\n", cErr)
		} else if corpus != nil {
			for _, c := range match.NewMatcher(corpus).Components("") {
				knownSelectors = append(knownSelectors, c.Selectors...)
			}
		}
	}

	// ── Apply filters ────────────────────────────────────────────────────────
	var filtered []authoring.Candidate
	for _, c := range candidates {
		if c.Count < *minCountFlag {
			continue
		}
		if len(knownSelectors) > 0 {
			covered := false
			for _, known := range knownSelectors {
				if strings.Contains(c.Sel, known) || strings.Contains(known, c.Sel) {
					covered = true
					break
				}
			}
			if covered {
				continue
			}
		}
		filtered = append(filtered, c)
	}

	// ── Print results ────────────────────────────────────────────────────────
	if len(filtered) == 0 {
		fmt.Println("No candidate selectors found.")
		return nil
	}

	// ── Grouped output ───────────────────────────────────────────────────────
	if *groupFlag {
		// Build the sightmapId → matchName map.
		sightmapMatchMap, mapErr := buildSightmapMatchMapForSuggest(ctx, conn, *snapFlag, *lf.sightmapDir)
		if mapErr != nil {
			return fmt.Errorf("suggest: --group: %v", mapErr)
		}

		// Group candidates by match name.
		groups := make(map[string][]authoring.Candidate)
		for _, c := range filtered {
			matchName := sightmapMatchMap[c.AncestorId]
			groups[matchName] = append(groups[matchName], c)
		}

		// Sort each group's candidates by count descending.
		for name := range groups {
			sort.Slice(groups[name], func(i, j int) bool {
				return groups[name][i].Count > groups[name][j].Count
			})
		}

		// Build sorted list of named groups (exclude ungrouped for now).
		type namedGroup struct {
			name  string
			cands []authoring.Candidate
		}
		var sortedGroups []namedGroup
		for name, cands := range groups {
			if name == "" {
				continue
			}
			sortedGroups = append(sortedGroups, namedGroup{name: name, cands: cands})
		}
		sort.Slice(sortedGroups, func(i, j int) bool {
			if len(sortedGroups[i].cands) != len(sortedGroups[j].cands) {
				return len(sortedGroups[i].cands) > len(sortedGroups[j].cands)
			}
			return sortedGroups[i].name < sortedGroups[j].name
		})

		// Print named groups.
		for _, g := range sortedGroups {
			fmt.Printf("[%s] \u2014 %d uncovered candidate", g.name, len(g.cands))
			if len(g.cands) != 1 {
				fmt.Print("s")
			}
			fmt.Println(":")
			printSuggestCandidates(g.cands)
			fmt.Println()
		}

		// Print ungrouped section if non-empty.
		if ungrouped := groups[""]; len(ungrouped) > 0 {
			fmt.Println("Ungrouped (no known ancestor):")
			printSuggestCandidates(ungrouped)
			fmt.Println()
		}

		return nil
	}

	// ── Flat output (no --group) ─────────────────────────────────────────────
	// Compute column width for alignment.
	maxSelLen := 0
	for _, c := range filtered {
		if len(c.Sel) > maxSelLen {
			maxSelLen = len(c.Sel)
		}
	}

	fmt.Println("Candidate selectors (not yet in sightmap):")
	fmt.Println()
	for _, c := range filtered {
		// Build the role/sample annotation.
		annotation := c.Role
		if c.Sample != "" {
			sample := c.Sample
			if len(sample) > 40 {
				sample = sample[:40] + "…"
			}
			annotation = fmt.Sprintf(`%s "%s"`, c.Role, sample)
		}
		fmt.Printf("  %-*s  %d element", maxSelLen, c.Sel, c.Count)
		if c.Count != 1 {
			fmt.Print("s")
		}
		fmt.Printf("  (%s)\n", annotation)
	}
	return nil
}

// printSuggestCandidates prints a formatted list of candidates, aligned by
// selector width. Used by the grouped output path.
func printSuggestCandidates(cands []authoring.Candidate) {
	maxSelLen := 0
	for _, c := range cands {
		if len(c.Sel) > maxSelLen {
			maxSelLen = len(c.Sel)
		}
	}
	for _, c := range cands {
		annotation := c.Role
		if c.Sample != "" {
			sample := c.Sample
			if len(sample) > 40 {
				sample = sample[:40] + "…"
			}
			annotation = fmt.Sprintf(`%s "%s"`, c.Role, sample)
		}
		fmt.Printf("  %-*s  %d element", maxSelLen, c.Sel, c.Count)
		if c.Count != 1 {
			fmt.Print("s")
		}
		if annotation != "" {
			fmt.Printf("  (%s)", annotation)
		}
		fmt.Println()
	}
}

// buildSightmapMatchMapForSuggest builds a map from sightmap node ID to sightmap
// component name. It tries the snap-file path first (lightweight), then falls
// back to a live ExtractComponents+MatchTree pass (heavy).
func buildSightmapMatchMapForSuggest(
	ctx context.Context,
	conn *browser.CDPConn,
	snapFile string,
	smDir string,
) (map[string]string, error) {
	// Get page URL for MatchTree view selection.
	pageURL, _ := browser.GetURL(ctx, conn)

	// Resolve snap file path.
	if snapFile == "" {
		snapFile = findLatestSnapTree(".")
	}

	if snapFile != "" {
		m, err := buildSightmapMatchMap(snapFile, smDir, pageURL)
		if err != nil {
			return nil, fmt.Errorf("load snap tree %s: %v", snapFile, err)
		}
		return m, nil
	}

	// Heavy fallback: extract live component tree and run MatchTree.
	page, err := conn.DefaultPage()
	if err != nil {
		return nil, fmt.Errorf("no snap tree found and cannot get page for fallback: %v\n"+
			"Tip: run 'sightmap snapshot --tree-out page.snap.tree.json' first, then use --snap", err)
	}
	root, err := browser.ExtractComponents(ctx, page)
	if err != nil {
		return nil, fmt.Errorf("extract components (fallback): %v", err)
	}
	corpus, err := sightmap.Load(smDir)
	if err != nil {
		return nil, fmt.Errorf("load corpus (fallback): %v", err)
	}
	matches := match.NewMatcher(corpus).MatchTree(root, pageURL)
	m := make(map[string]string)
	sightmap.Walk(root, func(n *sightmap.ComponentNode, _ int) bool {
		if sm := matches[n]; sm != nil && n.Id != "" {
			m[n.Id] = sm.Name
		}
		return true
	})
	return m, nil
}

// buildSightmapMatchMap loads a .snap.tree.json file, runs MatchTree on it, and
// returns a map from sightmap node ID to matched sightmap component name.
func buildSightmapMatchMap(treeFile string, smDir string, pageURL string) (map[string]string, error) {
	data, err := os.ReadFile(treeFile)
	if err != nil {
		return nil, err
	}
	var root sightmap.ComponentNode
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse tree JSON: %v", err)
	}
	corpus, err := sightmap.Load(smDir)
	if err != nil {
		return nil, fmt.Errorf("load corpus: %v", err)
	}
	matches := match.NewMatcher(corpus).MatchTree(&root, pageURL)
	m := make(map[string]string)
	sightmap.Walk(&root, func(n *sightmap.ComponentNode, _ int) bool {
		if sm := matches[n]; sm != nil && n.Id != "" {
			m[n.Id] = sm.Name
		}
		return true
	})
	return m, nil
}

// findLatestSnapTree returns the path of the most recently modified
// *.snap.tree.json file in dir, or "" if none are found.
func findLatestSnapTree(dir string) string {
	matches, _ := filepath.Glob(filepath.Join(dir, "*.snap.tree.json"))
	var best string
	var bestTime time.Time
	for _, f := range matches {
		info, err := os.Stat(f)
		if err == nil && info.ModTime().After(bestTime) {
			best = f
			bestTime = info.ModTime()
		}
	}
	return best
}
