// inspect connects to Chrome (or launches it), extracts the raw DOM component
// tree, optionally annotates it from the .sightmap/ corpus, and renders the
// full structural tree for CSS selector authoring.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/render"
	"github.com/sightmap/sightmap/go/sightmap"
)

func runInspect(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	addrFlag := fs.String("addr", browser.DefaultAddr(), "CDP address (host:port)")
	launchFlag := fs.Bool("launch", false, "Auto-launch Chrome if unreachable")
	urlFlag := fs.String("url", "", "Navigate to this URL before inspecting")
	waitFlag := fs.Float64("wait", 0, "Seconds to wait after navigation (minimum 0.5 when --url is used)")
	outFlag := fs.String("out", "", "Write output to file instead of stdout")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir for ★ annotations")
	classesFlag := fs.Bool("classes", false, "Show CSS class names in selector display")
	selectorsFlag := fs.Bool("selectors", false, "Show all attributes, not just key identifying ones")
	depthFlag := fs.Int("depth", 0, "Max tree depth to print (0 = unlimited)")
	interactiveFlag := fs.Bool("interactive", false, "Show only interactive nodes and their ancestors")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := context.Background()

	// ── Connect to Chrome ────────────────────────────────────────────────────
	cleanup := func() {}
	defer func() { cleanup() }()

	conn, dialErr := browser.DialCDP(ctx, *addrFlag)
	if dialErr != nil {
		if !*launchFlag {
			return fmt.Errorf(
				"inspect: cannot connect to Chrome at %s\n"+
					"Start a session first:\n"+
					"  inspect --launch --url https://...    (auto-launch Chrome)\n"+
					"  /path/to/chrome --remote-debugging-port=7892 --user-data-dir=/tmp/cdp",
				*addrFlag,
			)
		}
		var launchCleanup func()
		var launchErr error
		conn, launchCleanup, launchErr = browser.Launch(ctx, browser.LaunchOptions{})
		if launchErr != nil {
			return fmt.Errorf("cannot launch Chrome: %v", launchErr)
		}
		if launchCleanup != nil {
			cleanup = launchCleanup
		}
	} else {
		cleanup = func() { conn.Close() }
	}

	// ── Navigate ─────────────────────────────────────────────────────────────
	if *urlFlag != "" {
		if err := browser.NavigateAndWait(ctx, conn, *urlFlag); err != nil {
			return fmt.Errorf("navigate: %v", err)
		}
		// Minimum 0.5s wait when --url is used; --wait overrides upward only.
		wait := *waitFlag
		if wait < 0.5 {
			wait = 0.5
		}
		time.Sleep(time.Duration(wait * float64(time.Second)))
	} else if *waitFlag > 0 {
		time.Sleep(time.Duration(*waitFlag * float64(time.Second)))
	}

	// ── Extract ───────────────────────────────────────────────────────────────
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

	// ── Load sightmap (optional, silent on error) ─────────────────────────────
	var inspectMatches map[*comps.ComponentNode]*match.SightmapMatch
	if _, statErr := os.Stat(*sightmapDirFlag); statErr == nil {
		if corpus, cErr := sightmap.Load(*sightmapDirFlag); cErr == nil {
			inspectMatches = corpus.MatchTree(root, pageURL)
		}
	}

	// ── Render ────────────────────────────────────────────────────────────────
	inRoot := render.Inspect(root, inspectMatches)
	opts := render.InspectOpts{
		Classes:     *classesFlag,
		Selectors:   *selectorsFlag,
		MaxDepth:    *depthFlag,
		Interactive: *interactiveFlag,
	}

	// ── Write output ─────────────────────────────────────────────────────────
	if *outFlag != "" {
		tmpPath := *outFlag + ".tmp"
		f, ferr := os.Create(tmpPath)
		if ferr != nil {
			return fmt.Errorf("create output file: %v", ferr)
		}
		render.FormatInspect(f, inRoot, opts)
		if cerr := f.Close(); cerr != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("close output file: %v", cerr)
		}
		if rerr := os.Rename(tmpPath, *outFlag); rerr != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("rename output file: %v", rerr)
		}
	} else {
		render.FormatInspect(os.Stdout, inRoot, opts)
	}
	return nil
}
