// inspect connects to Chrome (or launches it), extracts the raw DOM component
// tree, optionally annotates it from the .sightmap/ corpus, and renders the
// full structural tree for CSS selector authoring.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/render"
)

func runInspect(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	lf := addLiveFlags(fs, "inspect")
	outFlag := fs.String("out", "", "Write output to file instead of stdout")
	classesFlag := fs.Bool("classes", false, "Show CSS class names in selector display")
	selectorsFlag := fs.Bool("selectors", false, "Show all attributes, not just key identifying ones")
	depthFlag := fs.Int("depth", 0, "Max tree depth to print (0 = unlimited)")
	interactiveFlag := fs.Bool("interactive", false, "Show only interactive nodes and their ancestors")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := context.Background()

	conn, cleanup, err := lf.connect(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	// A minimum 0.5s settle applies when --url is used, so freshly-navigated
	// content is present before extraction.
	if err := lf.navigate(ctx, conn, navOpts{minWait: 0.5}); err != nil {
		return err
	}

	// ── Extract ──────────────────────────────────────────────────────────
	pageURL, err := browser.GetURL(ctx, conn)
	if err != nil {
		return fmt.Errorf("get URL: %v", err)
	}

	page, err := conn.DefaultPage()
	if err != nil {
		return fmt.Errorf("default page: %v", err)
	}
	root, err := browser.ExtractComponents(ctx, page)
	if err != nil {
		return fmt.Errorf("extract components: %v", err)
	}

	// ── Load sightmap (optional enrichment; warn but don't fail on a bad corpus) ──
	var inspectMatches map[*comps.ComponentNode]*match.ComponentMatch
	if corpus, cErr := lf.loadCorpus(); cErr != nil {
		fmt.Fprintf(os.Stderr, "inspect: load corpus: %v (continuing without component names)\n", cErr)
	} else if corpus != nil {
		inspectMatches = corpus.MatchTree(root, pageURL)
	}

	// ── Render ────────────────────────────────────────────────────────────────
	inRoot := render.Inspect(root, inspectMatches)
	opts := render.InspectOpts{
		Classes:     *classesFlag,
		Selectors:   *selectorsFlag,
		MaxDepth:    *depthFlag,
		Interactive: *interactiveFlag,
	}

	// ── Write output ──────────────────────────────────────────────────────
	return writeOut(*outFlag, func(w io.Writer) error {
		render.FormatInspect(w, inRoot, opts)
		return nil
	})
}
