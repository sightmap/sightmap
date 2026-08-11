// Browser interaction subcommands: click, fill, hover, keypress, scroll,
// drag, wait-for, dialog, and tab management.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/compquery"
	"github.com/sightmap/sightmap/go/sightmap"
)

// ── Component ID resolution ───────────────────────────────────────────────────

// resolveNode maps a probe id to a live element via its persisted data-sightmap-id
// attribute (set during the prior snapshot/bounds extraction) and returns a node
// carrying the element's current bounds. It deliberately does NOT re-extract the
// component tree: a fresh extraction re-probes the page and reassigns every
// data-sightmap-id, which would invalidate the id the caller just minted and is the
// root cause of the snapshot->click race. Resolving through the
// already-present attribute keeps the id stable across re-renders that don't
// remount the target.
func resolveNode(ctx context.Context, conn *browser.CDPConn, id string) (*sightmap.ComponentNode, error) {
	return browser.ResolveBySightmapID(ctx, conn, id)
}

// actionLabel builds a short human label for a confirmation line: the argument
// the caller passed (a probe id or component query) plus the element's click
// point when its bounds are known. Interaction commands echo one of these on
// success so an agent driving the browser sees what happened.
func actionLabel(arg string, node *sightmap.ComponentNode) string {
	if node != nil && node.Bounds != nil {
		x := node.Bounds.X + node.Bounds.Width/2
		y := node.Bounds.Y + node.Bounds.Height/2
		return fmt.Sprintf("%s @ (%d,%d)", arg, x, y)
	}
	return arg
}

// parseFlagsInterspersed parses fs allowing flags to appear AFTER positional
// arguments. Go's stdlib flag stops at the first positional, which silently
// drops flags like `click 'ComponentQuery' --tab X` — a multi-agent footgun where --tab is ignored. We reorder flags
// ahead of positionals (consulting fs to know which flags take a value), then
// delegate to fs.Parse.
func parseFlagsInterspersed(fs *flag.FlagSet, args []string) error {
	var flags, positionals []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" { // explicit end-of-flags terminator
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if len(a) > 1 && a[0] == '-' {
			flags = append(flags, a)
			if !strings.Contains(a, "=") {
				name := strings.TrimLeft(a, "-")
				if f := fs.Lookup(name); f != nil {
					bf, isBool := f.Value.(interface{ IsBoolFlag() bool })
					if !(isBool && bf.IsBoolFlag()) && i+1 < len(args) {
						// value-taking flag: pull its value along too
						flags = append(flags, args[i+1])
						i++
					}
				}
			}
			continue
		}
		positionals = append(positionals, a)
	}
	return fs.Parse(append(flags, positionals...))
}

// ── click ─────────────────────────────────────────────────────────────────────

func runClick(args []string) error {
	fs := flag.NewFlagSet("click", flag.ContinueOnError)
	addrFlag := fs.String("addr", "", "CDP address (default: the session for --sightmap-dir)")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (for component queries; keys session lookup)")
	xFlag := fs.Int("x", -1, "Absolute x coordinate (skips node lookup)")
	yFlag := fs.Int("y", -1, "Absolute y coordinate (skips node lookup)")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}

	ctx := context.Background()
	conn, err := browser.Connect(resolveCDPAddr(*addrFlag, *sightmapDirFlag), *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

	if *xFlag >= 0 && *yFlag >= 0 {
		if err := browser.ClickAt(ctx, conn, *xFlag, *yFlag); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "clicked @ (%d,%d)\n", *xFlag, *yFlag)
		return nil
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: browser click (COMPONENT-ID | 'ComponentQuery' | --x N --y N)")
	}
	node, err := resolveTarget(ctx, conn, *sightmapDirFlag, fs.Arg(0))
	if err != nil {
		return err
	}
	x, y, err := browser.Click(ctx, conn, node)
	if err != nil {
		return err
	}
	if x >= 0 {
		fmt.Fprintf(os.Stderr, "clicked %s @ (%d,%d)\n", fs.Arg(0), x, y)
	} else {
		fmt.Fprintf(os.Stderr, "clicked %s\n", fs.Arg(0))
	}
	// A click may trigger async (SPA) navigation that only settles after this
	// returns. We deliberately do NOT wait here — waiting is the caller's explicit
	// step: follow an action that should navigate with `wait-for --view/--component`
	// (or --url), matching how Playwright/Selenium separate the act from the wait.
	return nil
}

// ── fill ──────────────────────────────────────────────────────────────────────

func runFill(args []string) error {
	fs := flag.NewFlagSet("fill", flag.ContinueOnError)
	addrFlag := fs.String("addr", "", "CDP address (default: the session for --sightmap-dir)")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (for component queries; keys session lookup)")
	clearFlag := fs.Bool("clear", false, "Clear the field via JS before typing (use for React-controlled inputs)")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("usage: browser fill [--clear] (COMPONENT-ID | 'ComponentQuery') VALUE")
	}

	ctx := context.Background()
	conn, err := browser.Connect(resolveCDPAddr(*addrFlag, *sightmapDirFlag), *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

	node, err := resolveTarget(ctx, conn, *sightmapDirFlag, fs.Arg(0))
	if err != nil {
		return err
	}
	if *clearFlag {
		if err := browser.ClearAndFill(ctx, conn, node, fs.Arg(1)); err != nil {
			return err
		}
	} else if err := browser.Fill(ctx, conn, node, fs.Arg(1)); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "filled %s = %q\n", fs.Arg(0), fs.Arg(1))
	return nil
}

// ── hover ─────────────────────────────────────────────────────────────────────

func runHover(args []string) error {
	fs := flag.NewFlagSet("hover", flag.ContinueOnError)
	addrFlag := fs.String("addr", "", "CDP address (default: the session for --sightmap-dir)")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (for component queries; keys session lookup)")
	xFlag := fs.Int("x", -1, "Absolute x coordinate (skips node lookup)")
	yFlag := fs.Int("y", -1, "Absolute y coordinate (skips node lookup)")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}

	ctx := context.Background()
	conn, err := browser.Connect(resolveCDPAddr(*addrFlag, *sightmapDirFlag), *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

	if *xFlag >= 0 && *yFlag >= 0 {
		if err := browser.HoverAt(ctx, conn, *xFlag, *yFlag); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "hovered @ (%d,%d)\n", *xFlag, *yFlag)
		return nil
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: browser hover (COMPONENT-ID | 'ComponentQuery' | --x N --y N)")
	}
	node, err := resolveTarget(ctx, conn, *sightmapDirFlag, fs.Arg(0))
	if err != nil {
		return err
	}
	if err := browser.Hover(ctx, conn, node); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "hovered %s\n", actionLabel(fs.Arg(0), node))
	return nil
}

// ── keypress ──────────────────────────────────────────────────────────────────

func runKeyPress(args []string) error {
	fs := flag.NewFlagSet("keypress", flag.ContinueOnError)
	addrFlag := fs.String("addr", "", "CDP address (default: the session for --sightmap-dir)")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (keys session lookup)")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: browser keypress KEY\n" +
			"  KEY examples: Enter Tab Escape Backspace Delete ArrowUp ArrowDown Space")
	}

	ctx := context.Background()
	conn, err := browser.Connect(resolveCDPAddr(*addrFlag, *sightmapDirFlag), *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := browser.KeyPress(ctx, conn, fs.Arg(0)); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "pressed %s\n", fs.Arg(0))
	return nil
}

// ── scroll ────────────────────────────────────────────────────────────────────

func runScroll(args []string) error {
	fs := flag.NewFlagSet("scroll", flag.ContinueOnError)
	addrFlag := fs.String("addr", "", "CDP address (default: the session for --sightmap-dir)")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (for component queries; keys session lookup)")
	compID := fs.String("component-id", "", "Component ID or 'ComponentQuery' to scroll into view first")
	deltaX := fs.Int("delta-x", 0, "Horizontal scroll delta in pixels")
	deltaY := fs.Int("delta-y", 0, "Vertical scroll delta in pixels")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}
	if *compID == "" && *deltaX == 0 && *deltaY == 0 {
		return fmt.Errorf("usage: browser scroll [--component-id ID] [--delta-x N] [--delta-y N]")
	}

	ctx := context.Background()
	conn, err := browser.Connect(resolveCDPAddr(*addrFlag, *sightmapDirFlag), *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

	if *compID != "" {
		node, resolveErr := resolveTarget(ctx, conn, *sightmapDirFlag, *compID)
		if resolveErr != nil {
			return resolveErr
		}
		if err := browser.ScrollIntoView(ctx, conn, node); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "scrolled %s into view\n", *compID)
	}
	if *deltaX != 0 || *deltaY != 0 {
		if err := browser.ScrollBy(ctx, conn, *deltaX, *deltaY); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "scrolled by (%d,%d)\n", *deltaX, *deltaY)
	}
	return nil
}

// ── drag ──────────────────────────────────────────────────────────────────────

func runDrag(args []string) error {
	fs := flag.NewFlagSet("drag", flag.ContinueOnError)
	addrFlag := fs.String("addr", "", "CDP address (default: the session for --sightmap-dir)")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (keys session lookup)")
	deltaX := fs.Int("delta-x", 0, "Horizontal drag delta in pixels")
	deltaY := fs.Int("delta-y", 0, "Vertical drag delta in pixels")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: browser drag COMPONENT-ID --delta-x N --delta-y N")
	}

	ctx := context.Background()
	conn, err := browser.Connect(resolveCDPAddr(*addrFlag, *sightmapDirFlag), *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

	node, err := resolveNode(ctx, conn, fs.Arg(0))
	if err != nil {
		return err
	}
	if err := browser.Drag(ctx, conn, node, *deltaX, *deltaY); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "dragged %s by (%d,%d)\n", fs.Arg(0), *deltaX, *deltaY)
	return nil
}

// ── wait-for ──────────────────────────────────────────────────────────────────

func runWaitFor(args []string) error {
	fs := flag.NewFlagSet("wait-for", flag.ContinueOnError)
	addrFlag := fs.String("addr", "", "CDP address (default: the session for --sightmap-dir)")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (keys session lookup)")
	urlPattern := fs.String("url", "", "Wait until the page URL contains this substring (not a glob/regex; prefer --view)")
	selector := fs.String("selector", "", "Wait until a DOM element matching this CSS selector appears")
	viewName := fs.String("view", "", "Wait until the page URL resolves to this sightmap view")
	component := fs.String("component", "", "Wait until this component query matches a node (e.g. 'WorkItemDetail')")
	loadFlag := fs.Bool("load", false, "Wait for the page load event")
	timeoutMs := fs.Int("timeout-ms", 10000, "Timeout in milliseconds (default 10 000)")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}

	n := 0
	for _, set := range []bool{*urlPattern != "", *selector != "", *viewName != "", *component != "", *loadFlag} {
		if set {
			n++
		}
	}
	if n != 1 {
		return fmt.Errorf("usage: browser wait-for (--url PATTERN | --selector SEL | --view NAME | --component QUERY | --load)")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(*timeoutMs)*time.Millisecond,
	)
	defer cancel()

	conn, err := browser.Connect(resolveCDPAddr(*addrFlag, *sightmapDirFlag), *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

	var waitErr error
	var what, done string
	switch {
	case *urlPattern != "":
		what = fmt.Sprintf("url to contain %q", *urlPattern)
		done = fmt.Sprintf("url now contains %q", *urlPattern)
		waitErr = browser.WaitForURL(ctx, conn, *urlPattern)
	case *selector != "":
		what = fmt.Sprintf("selector %q", *selector)
		done = fmt.Sprintf("matched selector %q", *selector)
		waitErr = browser.WaitForSelector(ctx, conn, *selector)
	case *viewName != "":
		what = fmt.Sprintf("view %q", *viewName)
		done = fmt.Sprintf("view %q now matches the page URL", *viewName)
		waitErr = waitForView(ctx, conn, *sightmapDirFlag, *viewName)
	case *component != "":
		what = fmt.Sprintf("component query %q", *component)
		done = fmt.Sprintf("component query %q now matches", *component)
		waitErr = waitForComponent(ctx, conn, *sightmapDirFlag, *component)
	default:
		what = "page load"
		done = "page load complete"
		waitErr = browser.WaitForLoad(ctx, conn)
	}
	if waitErr != nil {
		if errors.Is(waitErr, context.DeadlineExceeded) {
			return fmt.Errorf("wait-for: timed out after %dms waiting for %s", *timeoutMs, what)
		}
		return waitErr
	}
	fmt.Fprintln(os.Stderr, done)
	return nil
}

// waitForView polls until the current page URL resolves to the view named
// viewName in the corpus at sightmapDir, or ctx expires. The corpus is loaded
// once up front (view routes don't change mid-wait); only the URL is re-read.
func waitForView(ctx context.Context, conn *browser.CDPConn, sightmapDir, viewName string) error {
	corpus, err := sightmap.Load(sightmapDir)
	if err != nil {
		return fmt.Errorf("wait-for --view: load corpus: %w", err)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		url, _ := browser.GetURL(ctx, conn)
		if v := corpus.ViewForURL(url); v != nil && v.Name == viewName {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// waitForComponent polls until the component query matches at least one node on
// the live page, or ctx expires. The query syntax is validated up front so a
// typo fails immediately instead of silently timing out. Each poll re-extracts
// the live tree (so property-filtered queries like `Row[state="Done"]` are
// honored), which is the same resolve path a `click 'Query'` uses.
func waitForComponent(ctx context.Context, conn *browser.CDPConn, sightmapDir, query string) error {
	if _, err := compquery.ParseQuery(query); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if node, err := resolveComponentQuery(ctx, conn, sightmapDir, query); err == nil && node != nil {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// ── dialog ────────────────────────────────────────────────────────────────────

func runDialog(args []string) error {
	fs := flag.NewFlagSet("dialog", flag.ContinueOnError)
	addrFlag := fs.String("addr", "", "CDP address (default: the session for --sightmap-dir)")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (keys session lookup)")
	text := fs.String("text", "", "Prompt input text (prompt dialogs only)")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: browser dialog (accept|dismiss) [--text INPUT]")
	}
	action := fs.Arg(0)
	if action != "accept" && action != "dismiss" {
		return fmt.Errorf("dialog: action must be 'accept' or 'dismiss', got %q", action)
	}

	ctx := context.Background()
	conn, err := browser.Connect(resolveCDPAddr(*addrFlag, *sightmapDirFlag), *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := browser.HandleDialog(ctx, conn, action, *text); err != nil {
		if strings.Contains(err.Error(), "No dialog is showing") {
			return fmt.Errorf("dialog: no dialog is currently open (nothing to %s)", action)
		}
		return err
	}
	fmt.Fprintf(os.Stderr, "%sed dialog\n", action)
	return nil
}

// ── screenshot ──────────────────────────────────────────────────────────────────

func runScreenshot(args []string) error {
	fs := flag.NewFlagSet("screenshot", flag.ContinueOnError)
	addrFlag := fs.String("addr", "", "CDP address (default: the session for --sightmap-dir)")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	outFlag := fs.String("out", "screenshot.png", "Output file path")
	stdoutFlag := fs.Bool("stdout", false, "Write image bytes to stdout (for piping)")
	timeoutMsFlag := fs.Int("timeout-ms", 15000, "Timeout in milliseconds for each attempt")
	jpegFlag := fs.Bool("jpeg", false, "Force JPEG output (skips PNG attempt; implies optimizeForSpeed)")
	qualityFlag := fs.Int("quality", 80, "JPEG quality 0-100 (default: 80, ignored for PNG)")
	noRetryFlag := fs.Bool("no-retry", false, "Disable automatic JPEG fallback on timeout")
	componentFlag := fs.String("component", "", "Clip to the bounding box of this sightmap component (by name)")
	selectorFlag := fs.String("selector", "", "Clip to the bounding box of elements matching this CSS selector")
	expandPctFlag := fs.Float64("expand-pct", 0, "Grow the clip outward on all sides by this percent of its size (only with --component/--selector)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (for --component)")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}

	if *componentFlag != "" && *selectorFlag != "" {
		return fmt.Errorf("screenshot: pass only one of --component or --selector")
	}
	if *expandPctFlag != 0 && *componentFlag == "" && *selectorFlag == "" {
		return fmt.Errorf("screenshot: --expand-pct requires --component or --selector")
	}

	conn, err := browser.Connect(resolveCDPAddr(*addrFlag, *sightmapDirFlag), *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Resolve an optional clip box (union of in-viewport matches) once, up front,
	// so every capture attempt below clips identically.
	var clip *browser.ScreenshotClip
	if *componentFlag != "" || *selectorFlag != "" {
		clip, err = resolveScreenshotClip(context.Background(), conn, *sightmapDirFlag, *componentFlag, *selectorFlag, *expandPctFlag)
		if err != nil {
			return err
		}
	}

	timeout := time.Duration(*timeoutMsFlag) * time.Millisecond
	outPath := *outFlag

	var imgData []byte

	if *jpegFlag {
		// Force JPEG — skip PNG attempt entirely.
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		imgData, err = browser.ScreenshotWithOptions(ctx, conn, browser.ScreenshotOptions{
			Format:           "jpeg",
			Quality:          *qualityFlag,
			OptimizeForSpeed: true,
			PauseAnimations:  true,
			StopLoading:      true,
			Clip:             clip,
		})
		if err != nil {
			return fmt.Errorf("screenshot: %w", err)
		}
		// Silently fix up the extension so callers that use the default --out
		// don't end up with a JPEG file named .png.
		if strings.HasSuffix(outPath, ".png") {
			outPath = outPath[:len(outPath)-4] + ".jpg"
		}
	} else {
		// Attempt 1: PNG with StopLoading+PauseAnimations.
		// StopLoading halts ad/iframe repaints that prevent the compositor settling.
		ctx1, cancel1 := context.WithTimeout(context.Background(), timeout)
		defer cancel1()
		imgData, err = browser.ScreenshotWithOptions(ctx1, conn, browser.ScreenshotOptions{
			Format:          "png",
			PauseAnimations: true,
			StopLoading:     true,
			Clip:            clip,
		})
		if err != nil {
			if !*noRetryFlag && errors.Is(err, context.DeadlineExceeded) {
				// Attempt 2: JPEG fast path with half the original timeout (max 8s).
				fmt.Fprintln(os.Stderr, "screenshot: PNG timed out, retrying as JPEG...")
				fallbackTimeout := timeout / 2
				if fallbackTimeout > 8*time.Second {
					fallbackTimeout = 8 * time.Second
				}
				ctx2, cancel2 := context.WithTimeout(context.Background(), fallbackTimeout)
				defer cancel2()
				imgData, err = browser.ScreenshotWithOptions(ctx2, conn, browser.ScreenshotOptions{
					Format:           "jpeg",
					Quality:          80,
					OptimizeForSpeed: true,
					PauseAnimations:  true,
					StopLoading:      true,
					Clip:             clip,
				})
				if err != nil {
					return fmt.Errorf("screenshot (JPEG fallback): %w", err)
				}
				if strings.HasSuffix(outPath, ".png") {
					outPath = outPath[:len(outPath)-4] + ".jpg"
				}
			} else {
				return fmt.Errorf("screenshot: %w", err)
			}
		}
	}

	if *stdoutFlag {
		_, err = os.Stdout.Write(imgData)
		return err
	}
	if err := os.WriteFile(outPath, imgData, 0o644); err != nil {
		return fmt.Errorf("screenshot: write %s: %w", outPath, err)
	}
	fmt.Fprintf(os.Stderr, "screenshot saved: %s\n", outPath)
	return nil
}

// ── clear-storage ─────────────────────────────────────────────────────────────
// clear-storage nukes all cookies (including httpOnly) and origin storage for
// the given origin via CDP Network + Storage domains. This is the programmatic
// equivalent of Chrome DevTools Application → Clear storage — a reliable way to
// reset session state (including httpOnly cookies that JS cannot touch).
func runClearStorage(args []string) error {
	fs := flag.NewFlagSet("clear-storage", flag.ContinueOnError)
	addrFlag := fs.String("addr", "", "CDP address (default: the session for --sightmap-dir)")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (keys session lookup)")
	originFlag := fs.String("origin", "", "Origin to clear (default: current page origin)")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := browser.Connect(resolveCDPAddr(*addrFlag, *sightmapDirFlag), *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

	// 1. Clear ALL cookies (including httpOnly — JS document.cookie cannot touch these)
	if err := browser.ClearBrowserCookies(ctx, conn); err != nil {
		return fmt.Errorf("clear-storage: %w", err)
	}
	fmt.Fprintln(os.Stderr, "clear-storage: ✓ httpOnly cookies cleared")

	// 2. Clear origin storage (localStorage, IndexedDB, service workers, cache)
	origin := *originFlag
	if origin == "" {
		// Default to the origin of the current page
		raw, evalErr := browser.EvalJSON(ctx, conn, `JSON.stringify(location.origin)`)
		if evalErr == nil {
			_ = json.Unmarshal(raw, &origin)
		}
	}
	if origin != "" && origin != "null" {
		if err := browser.ClearStorageForOrigin(ctx, conn, origin); err != nil {
			fmt.Fprintf(os.Stderr, "clear-storage: warning: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "clear-storage: ✓ origin storage cleared (%s)\n", origin)
		}
	}

	fmt.Fprintln(os.Stderr, "clear-storage: done — navigate to homepage and wait before accessing protected pages")
	return nil
}

// ── tabs ──────────────────────────────────────────────────────────────────────

func runTabs(args []string) error {
	if len(args) == 0 {
		tabsUsage()
		return nil
	}
	switch args[0] {
	case "list":
		return runTabsList(args[1:])
	case "new":
		return runTabsNew(args[1:])
	case "close":
		return runTabsClose(args[1:])
	case "resize":
		return runTabsResize(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "sightmap browser tabs: unknown subcommand %q\n", args[0])
		tabsUsage()
		return nil
	}
}

func tabsUsage() {
	fmt.Fprint(os.Stderr, `sightmap browser tabs — tab management

Subcommands:
  list                     list open tabs (TargetID  URL  Title)
  new [URL]                open a new tab; prints its TargetID
  close  <target-id>       close a tab
  resize <width> <height>  resize the viewport of the current tab

`)
}

func sessionAddr(sightmapDir string) string {
	info, err := browser.ReadSessionInfo(sightmapDir)
	if err != nil || info.Port <= 0 {
		return fmt.Sprintf("localhost:%d", browser.DefaultCDPPort)
	}
	return fmt.Sprintf("localhost:%d", info.Port)
}

func runTabsList(args []string) error {
	sightmapDir, _ := resolveSightmapDir(args)
	ctx := context.Background()
	tabs, err := browser.ListTabs(ctx, sessionAddr(sightmapDir))
	if err != nil {
		return err
	}
	if len(tabs) == 0 {
		fmt.Fprintln(os.Stderr, "(no open tabs)")
		return nil
	}
	for _, t := range tabs {
		fmt.Printf("%s\t%s\t%s\n", t.TargetID, t.URL, t.Title)
	}
	return nil
}

func runTabsNew(args []string) error {
	sightmapDir, args := resolveSightmapDir(args)
	tabURL := ""
	if len(args) > 0 {
		tabURL = args[0]
	}

	ctx := context.Background()
	addr := sessionAddr(sightmapDir)

	targetID, conn, err := browser.CreateTab(ctx, addr, tabURL)
	if err != nil {
		return err
	}
	conn.Close()
	fmt.Println(targetID)
	return nil
}

func runTabsClose(args []string) error {
	sightmapDir, args := resolveSightmapDir(args)
	if len(args) == 0 {
		return fmt.Errorf("usage: browser tabs close <target-id>")
	}
	ctx := context.Background()
	if err := browser.CloseTab(ctx, sessionAddr(sightmapDir), args[0]); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "closed tab %s\n", args[0])
	return nil
}

func runTabsResize(args []string) error {
	fs := flag.NewFlagSet("tabs-resize", flag.ContinueOnError)
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (keys session lookup)")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("usage: browser tabs resize [--tab TAB_ID] <width> <height>")
	}
	width, err := strconv.Atoi(fs.Arg(0))
	if err != nil || width <= 0 {
		return fmt.Errorf("tabs resize: invalid width %q", fs.Arg(0))
	}
	height, err2 := strconv.Atoi(fs.Arg(1))
	if err2 != nil || height <= 0 {
		return fmt.Errorf("tabs resize: invalid height %q", fs.Arg(1))
	}

	ctx := context.Background()
	conn, err := browser.Connect(sessionAddr(*sightmapDirFlag), *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := browser.ResizeViewport(ctx, conn, width, height); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "resized viewport to %dx%d\n", width, height)
	return nil
}
