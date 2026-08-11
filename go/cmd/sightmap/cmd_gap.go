package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/sightmap/sightmap/go/sightmap"
	"sort"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/coverage"
	"github.com/sightmap/sightmap/go/match"
)

func runGap(args []string) error {
	fs := flag.NewFlagSet("gap", flag.ContinueOnError)
	lf := addLiveFlags(fs, "gap")
	includeHiddenFlag := fs.Bool("include-hidden", false, "Include hidden/off-screen nodes in gap analysis")
	scopeFlag := fs.String("scope", "", "Filter gaps to components within named component's subtree")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := context.Background()

	conn, cleanup, err := lf.connect(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := lf.navigate(ctx, conn, navOpts{}); err != nil {
		return err
	}

	// ── Extract component tree ────────────────────────────────────────
	pageURL, err := browser.GetURL(ctx, conn)
	if err != nil {
		return fmt.Errorf("gap: get URL: %v", err)
	}

	page, err := conn.DefaultPage()
	if err != nil {
		return fmt.Errorf("gap: default page: %v", err)
	}

	root, err := browser.ExtractComponents(ctx, page)
	if err != nil {
		return fmt.Errorf("gap: extract components: %v", err)
	}

	// ── Require .sightmap dir ────────────────────────────────────────
	corpus, loadErr := lf.requireCorpus()
	if loadErr != nil {
		return loadErr
	}
	m := match.NewMatcher(corpus)
	matches := m.Match(root, pageURL)

	// ── Build parent map ─────────────────────────────────────────────────────
	parentMap := coverage.BuildParentMap(root)

	// ── Handle --scope flag ──────────────────────────────────────────────────
	var scopeNode *sightmap.ComponentNode
	if *scopeFlag != "" {
		// Find the named component in matches
		for node, matchInfo := range matches {
			if matchInfo != nil && matchInfo.Name == *scopeFlag {
				scopeNode = node
				break
			}
		}

		if scopeNode == nil {
			// Component not found or not matched on page
			// Check if it exists in the corpus
			components := m.Components(pageURL)
			found := false
			for _, comp := range components {
				if comp.Name == *scopeFlag {
					found = true
					break
				}
			}

			if found {
				return fmt.Errorf("gap: component [%s] not found on current page\n(Component is defined but its selector doesn't match anything)", *scopeFlag)
			}

			// Component doesn't exist in corpus
			var availableNames []string
			for _, comp := range components {
				availableNames = append(availableNames, comp.Name)
			}
			sort.Strings(availableNames)
			return fmt.Errorf(
				"gap: component %q not found in sightmap\nAvailable components: %v",
				*scopeFlag,
				availableNames,
			)
		}
	}

	// ── Collect nodes based on scope ─────────────────────────────────────
	var gapNodes []*sightmap.ComponentNode

	// Helper to check if a node is within the scope subtree
	isInScope := func(n *sightmap.ComponentNode) bool {
		if scopeNode == nil {
			return true // no scope filter
		}
		// Check if n is a descendant of scopeNode (but not scopeNode itself)
		for curr := parentMap[n]; curr != nil; curr = parentMap[curr] {
			if curr == scopeNode {
				return true
			}
		}
		return false
	}

	// When --scope is provided, collect T2 nodes (interactive nodes inside the
	// scope component that are not matched).
	// When --scope is absent, collect T3 nodes (interactive nodes with no matched
	// ancestor anywhere).
	sightmap.Walk(root, func(n *sightmap.ComponentNode, _ int) bool {
		if !n.IsInteractive {
			return true
		}
		// Default to visible-only; --include-hidden overrides.
		if !*includeHiddenFlag && (!n.IsVisible || n.IsIgnored) {
			return true
		}
		if matches[n] != nil {
			return true // node itself is matched
		}

		if scopeNode != nil {
			// Scoped mode: collect T2 nodes (unmatched but inside scope)
			if isInScope(n) {
				gapNodes = append(gapNodes, n)
			}
		} else {
			// Unscoped mode: collect T3 nodes (no matched ancestor)
			hasMatchedAncestor := false
			for p := parentMap[n]; p != nil; p = parentMap[p] {
				if matches[p] != nil {
					hasMatchedAncestor = true
					break
				}
			}
			if !hasMatchedAncestor {
				gapNodes = append(gapNodes, n)
			}
		}
		return true
	})

	if len(gapNodes) == 0 {
		if scopeNode != nil {
			fmt.Printf("✓ No T2 gaps in [%s]\n", *scopeFlag)
		} else {
			fmt.Println("✓ no orphaned interactive nodes")
		}
		return nil
	}

	// ── Group by nearest stable-anchor ancestor ────────────────────────────
	type t3Group struct {
		ancestorDesc string
		nodes        []*sightmap.ComponentNode
	}

	type groupKey struct {
		role   string
		name   string
		inside string
	}

	var order []groupKey
	clusters := make(map[groupKey]*t3Group)

	for _, n := range gapNodes {
		anc := coverage.NearestDataAttrAncestor(n, parentMap)
		inside := "(no stable ancestor)"
		if scopeNode != nil {
			// In scoped mode, use the scope component as the inside context
			inside = fmt.Sprintf("[%s]", *scopeFlag)
		} else if anc != nil {
			inside = coverage.DataAttrSelector(anc)
		}

		nameStr := n.Name
		if nameStr == "" {
			nameStr = "(no text)"
		} else if len([]rune(nameStr)) > 40 {
			runes := []rune(nameStr)
			nameStr = string(runes[:40]) + "…"
		}

		role := n.Role
		if role == "" {
			role = "?"
		}

		key := groupKey{role: role, name: nameStr, inside: inside}
		if g, ok := clusters[key]; ok {
			g.nodes = append(g.nodes, n)
		} else {
			clusters[key] = &t3Group{
				ancestorDesc: inside,
				nodes:        []*sightmap.ComponentNode{n},
			}
			order = append(order, key)
		}
	}

	// Sort by count descending, then role, then name.
	sort.Slice(order, func(i, j int) bool {
		ci, cj := clusters[order[i]], clusters[order[j]]
		if len(ci.nodes) != len(cj.nodes) {
			return len(ci.nodes) > len(cj.nodes)
		}
		if order[i].role != order[j].role {
			return order[i].role < order[j].role
		}
		return order[i].name < order[j].name
	})

	// ── Print results ──────────────────────────────────────────────────────
	if scopeNode != nil {
		fmt.Printf("T2 gaps in [%s] — %d unmatched interactive element", *scopeFlag, len(gapNodes))
	} else {
		fmt.Printf("Gap analysis — %d orphaned interactive node", len(gapNodes))
	}
	if len(gapNodes) != 1 {
		fmt.Print("s")
	}
	fmt.Println(":")
	fmt.Println()

	for _, key := range order {
		g := clusters[key]
		rep := g.nodes[0]

		nameDisp := key.name
		if key.name != "(no text)" {
			nameDisp = `"` + key.name + `"`
		}

		fmt.Printf("  %d\u00d7 %s %s\n", len(g.nodes), key.role, nameDisp)
		fmt.Printf("       inside: %s\n", g.ancestorDesc)

		// Build arrow hint from the representative node.
		if hint := coverage.ArrowHint(rep, parentMap); hint != "" {
			fmt.Printf("       \u2192 %s\n", hint)
		}
		fmt.Println()
	}

	return nil
}
