// explain answers "what is this node, and how is it covered?" for one or a few
// nodes picked out of the component tree — by CSS selector, by tree node id, or
// by an accessible-name/role substring. For each it prints the node's raw facts
// (tag/id/classes/attrs), ranked stable selector candidates, its coverage tier
// and owning component, and its ancestor chain with each ancestor's best hook.
//
// It fills the node-first gap left by sel-probe (which is selector-first): when
// you've spotted a node in a captured *.snap.tree.json but don't yet have a
// selector, `explain --snap tree.json --grep Refresh` (or --id) dumps exactly the
// facts you'd otherwise hand-walk the JSON for. Works live (default) or offline
// against a captured tree via --snap.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/coverage"
	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
	"github.com/sightmap/sightmap/go/viewset"
)

func runExplain(args []string) error {
	fs := flag.NewFlagSet("explain", flag.ContinueOnError)
	lf := addLiveFlags(fs, "explain")
	snapFlag := fs.String("snap", "", "Inspect a captured tree offline (path to a .snap or .snap.tree.json) instead of the live page")
	idFlag := fs.String("id", "", "Select the node with this tree id")
	grepFlag := fs.String("grep", "", "Select nodes whose role or accessible name contains this substring (case-insensitive)")
	maxN := fs.Int("max", 10, "Max nodes to explain")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: explain [flags] [selector]\n\n")
		fmt.Fprintf(os.Stderr, "Pick nodes by a CSS selector (positional), --id, or --grep, then dump each\n")
		fmt.Fprintf(os.Stderr, "node's facts, selector candidates, coverage tier + owning component, and\n")
		fmt.Fprintf(os.Stderr, "ancestor hooks. Live by default; --snap FILE inspects a captured tree.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	var selector string
	if rest := fs.Args(); len(rest) == 1 {
		selector = rest[0]
	} else if len(rest) > 1 {
		return fmt.Errorf("explain: expected at most one selector argument, got %d", len(rest))
	}
	if selector == "" && *idFlag == "" && *grepFlag == "" {
		fs.Usage()
		return fmt.Errorf("explain: choose nodes with a selector, --id, or --grep")
	}

	// ── Acquire the tree + matches (offline via --snap, else live) ────────────
	var (
		root    *sightmap.ComponentNode
		matches map[*sightmap.ComponentNode]*sightmap.ComponentMatch
	)
	corpus, cErr := lf.loadCorpus()
	if cErr != nil {
		fmt.Fprintf(os.Stderr, "explain: load corpus: %v (continuing without tier/owner)\n", cErr)
	}

	if *snapFlag != "" {
		r, m, err := loadCapturedTree(*snapFlag, corpus)
		if err != nil {
			return err
		}
		root, matches = r, m
	} else {
		ctx := context.Background()
		conn, cleanup, err := lf.connect(ctx)
		if err != nil {
			return err
		}
		defer cleanup()
		if err := lf.navigate(ctx, conn, navOpts{minWait: 0.5}); err != nil {
			return err
		}
		pageURL, _ := browser.GetURL(ctx, conn)
		page, err := conn.DefaultPage()
		if err != nil {
			return fmt.Errorf("explain: default page: %v", err)
		}
		root, err = browser.ExtractComponents(ctx, page)
		if err != nil {
			return fmt.Errorf("explain: extract components: %v", err)
		}
		if corpus != nil {
			matches = match.NewMatcher(corpus).Match(root, pageURL)
		}
	}

	parentMap := coverage.BuildParentMap(root)
	selected := selectExplainNodes(root, selector, *idFlag, *grepFlag, *maxN)
	if len(selected) == 0 {
		fmt.Println("no matching nodes")
		return nil
	}

	for i, n := range selected {
		if i > 0 {
			fmt.Println()
		}
		printExplainNode(n, matches, parentMap)
	}
	return nil
}

// loadCapturedTree reads a captured tree (accepting either the .snap or the
// .snap.tree.json path) and matches it against the corpus using the capture's own
// route header when available (globals-only otherwise).
func loadCapturedTree(arg string, corpus *sightmap.Corpus) (*sightmap.ComponentNode, map[*sightmap.ComponentNode]*sightmap.ComponentMatch, error) {
	treeFile := arg
	if !strings.HasSuffix(treeFile, ".tree.json") {
		treeFile = arg + ".tree.json"
	}
	data, err := os.ReadFile(treeFile)
	if err != nil {
		return nil, nil, fmt.Errorf("explain: read %s: %v", treeFile, err)
	}
	root := &sightmap.ComponentNode{}
	if err := json.Unmarshal(data, root); err != nil {
		return nil, nil, fmt.Errorf("explain: parse %s: %v", treeFile, err)
	}
	var matches map[*sightmap.ComponentNode]*sightmap.ComponentMatch
	if corpus != nil {
		route := ""
		if snapPath := strings.TrimSuffix(treeFile, ".tree.json"); snapPath != treeFile {
			if _, statErr := os.Stat(snapPath); statErr == nil {
				route = viewset.RouteOf(snapPath)
			}
		}
		matches = match.NewMatcher(corpus).Match(root, route)
	}
	return root, matches, nil
}

// selectExplainNodes resolves the node selection (selector | --id | --grep) into
// tree-ordered nodes, capped at max.
func selectExplainNodes(root *sightmap.ComponentNode, selector, id, grep string, max int) []*sightmap.ComponentNode {
	var matchSet map[*sightmap.ComponentNode]bool
	if selector != "" {
		defs := []sightmap.ComponentDef{{Name: "__explain__", Selectors: []string{selector}}}
		m := match.NewMatcher(&sightmap.Corpus{GlobalComponents: defs}).Match(root, "")
		matchSet = make(map[*sightmap.ComponentNode]bool, len(m))
		for n := range m {
			matchSet[n] = true
		}
	}
	grepLC := strings.ToLower(grep)

	var out []*sightmap.ComponentNode
	sightmap.Walk(root, func(n *sightmap.ComponentNode, _ int) bool {
		if len(out) >= max {
			return false
		}
		switch {
		case id != "":
			if n.Id == id {
				out = append(out, n)
			}
		case selector != "":
			if matchSet[n] {
				out = append(out, n)
			}
		case grep != "":
			if strings.Contains(strings.ToLower(n.Role), grepLC) || strings.Contains(strings.ToLower(n.Name), grepLC) {
				out = append(out, n)
			}
		}
		return true
	})
	return out
}

// printExplainNode dumps one node's facts, candidates, tier/owner, and ancestors.
func printExplainNode(
	n *sightmap.ComponentNode,
	matches map[*sightmap.ComponentNode]*sightmap.ComponentMatch,
	parentMap map[*sightmap.ComponentNode]*sightmap.ComponentNode,
) {
	fmt.Printf("%s  #%s\n", elementDescOf(n.Element), n.Id)
	if n.Role != "" || n.Name != "" {
		fmt.Printf("    role/name: %s %q\n", nonEmpty(n.Role, "?"), n.Name)
	}
	flags := []string{}
	if n.IsInteractive {
		flags = append(flags, "interactive")
	}
	if !n.IsVisible {
		flags = append(flags, "hidden")
	}
	if n.IsIgnored {
		flags = append(flags, "ignored")
	}
	if len(flags) > 0 {
		fmt.Printf("    flags: %s\n", strings.Join(flags, ", "))
	}
	if n.Element != nil && len(n.Element.Attrs) > 0 {
		var parts []string
		for k, v := range n.Element.Attrs {
			if k == "id" || k == "class" {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s=%q", k, v))
		}
		if len(parts) > 0 {
			sort.Strings(parts)
			fmt.Printf("    attrs: %s\n", strings.Join(parts, "  "))
		}
	}

	// Coverage tier + owning component.
	tier, owner := tierAndOwner(n, matches, parentMap)
	if matches != nil {
		line := "    coverage: " + tier
		if owner != "" {
			line += "  ← [" + owner + "]"
		}
		fmt.Println(line)
	}

	// Ranked selector candidates for THIS node.
	if cands := coverage.SelectorCandidates(n.Element); len(cands) > 0 {
		fmt.Printf("    candidates: %s\n", strings.Join(cands, "  ·  "))
	}

	// Ancestor chain with each ancestor's best hook + owning component.
	var anc []string
	depth := 0
	for p := parentMap[n]; p != nil && depth < 6; p = parentMap[p] {
		desc := elementDescOf(p.Element)
		if cands := coverage.SelectorCandidates(p.Element); len(cands) > 0 {
			desc += "  {" + cands[0] + "}"
		}
		if m := matches[p]; m != nil {
			desc += "  ← [" + m.Name + "]"
		}
		anc = append(anc, desc)
		depth++
	}
	if len(anc) > 0 {
		fmt.Printf("    ancestors:\n")
		for _, a := range anc {
			fmt.Printf("      %s\n", a)
		}
	}
}

// tierAndOwner classifies a node as T1/T2/T3 and names its owning component (the
// node's own match for T1, the nearest matched ancestor for T2).
func tierAndOwner(
	n *sightmap.ComponentNode,
	matches map[*sightmap.ComponentNode]*sightmap.ComponentMatch,
	parentMap map[*sightmap.ComponentNode]*sightmap.ComponentNode,
) (tier, owner string) {
	if m := matches[n]; m != nil {
		return "T1 (direct)", m.Name
	}
	for p := parentMap[n]; p != nil; p = parentMap[p] {
		if m := matches[p]; m != nil {
			return "T2 (scoped)", m.Name
		}
	}
	return "T3 (orphan)", ""
}

// elementDescOf formats an element's identity as tag#id.class1.class2, or its
// role when there is no underlying element.
func elementDescOf(el *sightmap.Element) string {
	if el == nil {
		return "(no element)"
	}
	return elementDesc(el.Tag, el.Id, el.Classes)
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
