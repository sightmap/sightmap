// sel-check evaluates a CSS selector against a saved .snap.tree.json file
// without needing a live browser session.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/sel"
)

// errSelCheckNoMatches is returned (exit code 1) when the selector matches
// zero nodes in the tree.
var errSelCheckNoMatches = errors.New("no matches found")

// runSelCheck is the entry point registered in main.go.
func runSelCheck(args []string) error {
	return runSelCheckOut(args, os.Stdout)
}

// runSelCheckOut is the testable core: it writes all output to out.
func runSelCheckOut(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("sel-check", flag.ContinueOnError)
	limitFlag := fs.Int("limit", 20, "Max matches to print (all matches are still counted)")
	// --include-hidden is accepted for UX parity with other commands; all nodes
	// are shown by default since this is a diagnostic tool, so the flag is a no-op.
	_ = fs.Bool("include-hidden", false, "Include hidden nodes (default: all nodes are shown)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: sel-check 'SELECTOR' [FILE.snap | FILE.snap.tree.json]\n\n")
		fmt.Fprintf(os.Stderr, "Evaluates a CSS selector against a saved .snap.tree.json file offline.\n")
		fmt.Fprintf(os.Stderr, "Exit code 1 when the selector matches zero nodes.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) < 1 {
		fs.Usage()
		return fmt.Errorf("expected a SELECTOR argument")
	}
	if len(remaining) < 2 {
		return fmt.Errorf("no snapshot file given — provide a .snap or .snap.tree.json file path\n" +
			"Example: sightmap sel-check '[data-component*=\":MyComp:\"]' .sightmap/snapshots/pdp/base.snap")
	}

	selectorStr := remaining[0]
	inputFile := remaining[1]
	limit := *limitFlag

	// Resolve the tree file path and a human-readable display name.
	treeFile, displayName := selCheckResolvePaths(inputFile)

	// Parse the user's selector.
	ps, err := sel.ParseSightmapSelector(selectorStr)
	if err != nil {
		return err
	}
	if len(ps.Parts) == 0 {
		return fmt.Errorf("selector parsed to zero parts: %q", selectorStr)
	}
	// For per-node matching use the last part of the selector (the target element).
	// Descendant/child relationships are not enforced — this tool checks whether
	// any individual node's selector satisfies the last simple selector.
	rulePart := ps.Parts[len(ps.Parts)-1]

	// Load the tree JSON.
	data, err := os.ReadFile(treeFile)
	if err != nil {
		return fmt.Errorf("cannot read tree file %s: %w", treeFile, err)
	}

	var root comps.ComponentNode
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("parse tree JSON %s: %w", treeFile, err)
	}

	// Walk the full tree and collect every node whose selector matches.
	var matches []*comps.ComponentNode
	comps.Walk(&root, func(node *comps.ComponentNode, _ int) bool {
		// MatchesNode handles :has() (against node's subtree) and nil selectors.
		if sel.MatchesNode(node, rulePart) {
			matches = append(matches, node)
		}
		return true // always descend
	})

	total := len(matches)

	if total == 0 {
		fmt.Fprintf(out, "0 matches in %s\n", displayName)
		return errSelCheckNoMatches
	}

	fmt.Fprintf(out, "%d match%s in %s:\n", total, pluralS(total), displayName)

	// Print up to limit matches.
	shown := matches
	if limit > 0 && len(shown) > limit {
		shown = shown[:limit]
	}

	// Attribute keys surfaced per match (only when present on the node selector).
	interestingAttrs := []string{"data-component", "data-testid", "aria-label", "id"}

	for _, node := range shown {
		name := node.Name
		if len([]rune(name)) > 50 {
			name = string([]rune(name)[:50])
		}
		fmt.Fprintf(out, "\n  node %s  %-8s%q\n", node.Id, node.Role, name)
		if node.Element != nil && len(node.Element.Attrs) > 0 {
			for _, key := range interestingAttrs {
				if val, ok := node.Element.Attrs[key]; ok {
					fmt.Fprintf(out, "    %s: %s\n", key, val)
				}
			}
		}
	}

	if limit > 0 && total > limit {
		fmt.Fprintf(out, "\n  … %d more (use --limit to show more)\n", total-limit)
	}

	return nil
}

// selCheckResolvePaths returns the .tree.json path to read plus a display name
// for output. Accepts either a .snap file or a .snap.tree.json file.
func selCheckResolvePaths(inputFile string) (treeFile, displayName string) {
	if strings.HasSuffix(inputFile, ".snap.tree.json") {
		snapFile := strings.TrimSuffix(inputFile, ".tree.json")
		return inputFile, snapFile
	}
	// Assume .snap (or any other path) — derive the tree file by appending suffix.
	return inputFile + ".tree.json", inputFile
}
