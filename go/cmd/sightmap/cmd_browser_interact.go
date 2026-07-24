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
	"github.com/sightmap/sightmap/go/comps"
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
func resolveNode(ctx context.Context, conn *browser.CDPConn, id string) (*comps.ComponentNode, error) {
	return browser.ResolveBySightmapID(ctx, conn, id)
}

// dialAddrTab connects to Chrome for a page-affecting command. With an explicit
// tabID it connects to that tab. Without one it auto-selects the single open
// CONTENT tab, but errors (listing the tabs) when there are zero or several — so
// concurrent agents can never silently drive each other's tab, and a command
// never lands on the extension side panel.
func dialAddrTab(addr, tabID string) (*browser.CDPConn, error) {
	ctx := context.Background()
	if tabID != "" {
		conn, err := browser.ConnectTab(ctx, addr, tabID)
		if err != nil {
			return nil, fmt.Errorf("cannot connect to tab %s at %s\n"+
				"Run 'sightmap browser tabs list' to see open tabs.", tabID, addr)
		}
		return conn, nil
	}

	tabs, err := browser.ListTabs(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to Chrome at %s\n"+
			"Start a session first: sightmap browser start", addr)
	}
	switch len(tabs) {
	case 0:
		return nil, fmt.Errorf("no content tab open at %s\n"+
			"Start a session first: sightmap browser start", addr)
	case 1:
		conn, err := browser.ConnectTab(ctx, addr, tabs[0].TargetID)
		if err != nil {
			return nil, fmt.Errorf("cannot connect to tab %s at %s", tabs[0].TargetID, addr)
		}
		return conn, nil
	default:
		return nil, ambiguousTabError(tabs)
	}
}

// ambiguousTabError builds the "pass --tab" guidance shown when several content
// tabs are open (the concurrent multi-agent case).
func ambiguousTabError(tabs []browser.TabInfo) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%d content tabs are open — pass --tab <ID> to choose one\n", len(tabs))
	b.WriteString("(no default when a session has multiple tabs, to avoid cross-agent crosstalk):\n")
	for _, t := range tabs {
		url := t.URL
		if len(url) > 70 {
			url = url[:67] + "..."
		}
		fmt.Fprintf(&b, "  --tab %s  %s\n", t.TargetID, url)
	}
	b.WriteString("Your session's tab ID was printed by 'sightmap browser start'.")
	return errors.New(b.String())
}

// dialAddr connects to the default page (backwards-compatible single-agent path).
func dialAddr(addr string) (*browser.CDPConn, error) {
	return dialAddrTab(addr, "")
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
	addrFlag := fs.String("addr", browser.DefaultAddr(), "CDP address")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (for component queries)")
	xFlag := fs.Int("x", -1, "Absolute x coordinate (skips node lookup)")
	yFlag := fs.Int("y", -1, "Absolute y coordinate (skips node lookup)")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}

	ctx := context.Background()
	conn, err := dialAddrTab(*addrFlag, *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

	if *xFlag >= 0 && *yFlag >= 0 {
		return browser.ClickAt(ctx, conn, *xFlag, *yFlag)
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: browser click (COMPONENT-ID | 'ComponentQuery' | --x N --y N)")
	}
	node, err := resolveTarget(ctx, conn, *sightmapDirFlag, fs.Arg(0))
	if err != nil {
		return err
	}
	return browser.Click(ctx, conn, node)
}

// ── fill ──────────────────────────────────────────────────────────────────────

func runFill(args []string) error {
	fs := flag.NewFlagSet("fill", flag.ContinueOnError)
	addrFlag := fs.String("addr", browser.DefaultAddr(), "CDP address")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (for component queries)")
	clearFlag := fs.Bool("clear", false, "Clear the field via JS before typing (use for React-controlled inputs)")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("usage: browser fill [--clear] (COMPONENT-ID | 'ComponentQuery') VALUE")
	}

	ctx := context.Background()
	conn, err := dialAddrTab(*addrFlag, *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

	node, err := resolveTarget(ctx, conn, *sightmapDirFlag, fs.Arg(0))
	if err != nil {
		return err
	}
	if *clearFlag {
		return browser.ClearAndFill(ctx, conn, node, fs.Arg(1))
	}
	return browser.Fill(ctx, conn, node, fs.Arg(1))
}

// ── hover ─────────────────────────────────────────────────────────────────────

func runHover(args []string) error {
	fs := flag.NewFlagSet("hover", flag.ContinueOnError)
	addrFlag := fs.String("addr", browser.DefaultAddr(), "CDP address")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (for component queries)")
	xFlag := fs.Int("x", -1, "Absolute x coordinate (skips node lookup)")
	yFlag := fs.Int("y", -1, "Absolute y coordinate (skips node lookup)")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}

	ctx := context.Background()
	conn, err := dialAddrTab(*addrFlag, *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

	if *xFlag >= 0 && *yFlag >= 0 {
		return browser.HoverAt(ctx, conn, *xFlag, *yFlag)
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: browser hover (COMPONENT-ID | 'ComponentQuery' | --x N --y N)")
	}
	node, err := resolveTarget(ctx, conn, *sightmapDirFlag, fs.Arg(0))
	if err != nil {
		return err
	}
	return browser.Hover(ctx, conn, node)
}

// ── keypress ──────────────────────────────────────────────────────────────────

func runKeyPress(args []string) error {
	fs := flag.NewFlagSet("keypress", flag.ContinueOnError)
	addrFlag := fs.String("addr", browser.DefaultAddr(), "CDP address")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: browser keypress KEY\n" +
			"  KEY examples: Enter Tab Escape Backspace Delete ArrowUp ArrowDown Space")
	}

	ctx := context.Background()
	conn, err := dialAddrTab(*addrFlag, *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

	return browser.KeyPress(ctx, conn, fs.Arg(0))
}

// ── scroll ────────────────────────────────────────────────────────────────────

func runScroll(args []string) error {
	fs := flag.NewFlagSet("scroll", flag.ContinueOnError)
	addrFlag := fs.String("addr", browser.DefaultAddr(), "CDP address")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (for component queries)")
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
	conn, err := dialAddrTab(*addrFlag, *tabFlag)
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
	}
	if *deltaX != 0 || *deltaY != 0 {
		return browser.ScrollBy(ctx, conn, *deltaX, *deltaY)
	}
	return nil
}

// ── drag ──────────────────────────────────────────────────────────────────────

func runDrag(args []string) error {
	fs := flag.NewFlagSet("drag", flag.ContinueOnError)
	addrFlag := fs.String("addr", browser.DefaultAddr(), "CDP address")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	deltaX := fs.Int("delta-x", 0, "Horizontal drag delta in pixels")
	deltaY := fs.Int("delta-y", 0, "Vertical drag delta in pixels")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: browser drag COMPONENT-ID --delta-x N --delta-y N")
	}

	ctx := context.Background()
	conn, err := dialAddrTab(*addrFlag, *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

	node, err := resolveNode(ctx, conn, fs.Arg(0))
	if err != nil {
		return err
	}
	return browser.Drag(ctx, conn, node, *deltaX, *deltaY)
}

// ── wait-for ──────────────────────────────────────────────────────────────────

func runWaitFor(args []string) error {
	fs := flag.NewFlagSet("wait-for", flag.ContinueOnError)
	addrFlag := fs.String("addr", browser.DefaultAddr(), "CDP address")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	urlPattern := fs.String("url", "", "Wait until the page URL contains this pattern")
	selector := fs.String("selector", "", "Wait until a DOM element matching this selector appears")
	loadFlag := fs.Bool("load", false, "Wait for the page load event")
	timeoutMs := fs.Int("timeout-ms", 10000, "Timeout in milliseconds (default 10 000)")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}

	n := 0
	if *urlPattern != "" {
		n++
	}
	if *selector != "" {
		n++
	}
	if *loadFlag {
		n++
	}
	if n != 1 {
		return fmt.Errorf("usage: browser wait-for (--url PATTERN | --selector SEL | --load)")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(*timeoutMs)*time.Millisecond,
	)
	defer cancel()

	conn, err := dialAddrTab(*addrFlag, *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

	switch {
	case *urlPattern != "":
		return browser.WaitForURL(ctx, conn, *urlPattern)
	case *selector != "":
		return browser.WaitForSelector(ctx, conn, *selector)
	default:
		return browser.WaitForLoad(ctx, conn)
	}
}

// ── dialog ────────────────────────────────────────────────────────────────────

func runDialog(args []string) error {
	fs := flag.NewFlagSet("dialog", flag.ContinueOnError)
	addrFlag := fs.String("addr", browser.DefaultAddr(), "CDP address")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
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
	conn, err := dialAddrTab(*addrFlag, *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

	return browser.HandleDialog(ctx, conn, action, *text)
}

// ── screenshot ──────────────────────────────────────────────────────────────────

func runScreenshot(args []string) error {
	fs := flag.NewFlagSet("screenshot", flag.ContinueOnError)
	addrFlag := fs.String("addr", browser.DefaultAddr(), "CDP address")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	outFlag := fs.String("out", "screenshot.png", "Output file path")
	stdoutFlag := fs.Bool("stdout", false, "Write image bytes to stdout (for piping)")
	timeoutMsFlag := fs.Int("timeout-ms", 15000, "Timeout in milliseconds for each attempt")
	jpegFlag := fs.Bool("jpeg", false, "Force JPEG output (skips PNG attempt; implies optimizeForSpeed)")
	qualityFlag := fs.Int("quality", 80, "JPEG quality 0-100 (default: 80, ignored for PNG)")
	noRetryFlag := fs.Bool("no-retry", false, "Disable automatic JPEG fallback on timeout")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}

	conn, err := dialAddrTab(*addrFlag, *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

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
	addrFlag := fs.String("addr", browser.DefaultAddr(), "CDP address")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	originFlag := fs.String("origin", "", "Origin to clear (default: current page origin)")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := dialAddrTab(*addrFlag, *tabFlag)
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

func sessionAddr() string {
	info, err := browser.ReadSessionInfo()
	if err != nil || info.Port <= 0 {
		return fmt.Sprintf("localhost:%d", browser.DefaultCDPPort)
	}
	return fmt.Sprintf("localhost:%d", info.Port)
}

func runTabsList(_ []string) error {
	ctx := context.Background()
	tabs, err := browser.ListTabs(ctx, sessionAddr())
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
	tabURL := ""
	if len(args) > 0 {
		tabURL = args[0]
	}

	ctx := context.Background()
	addr := sessionAddr()

	targetID, conn, err := browser.CreateTab(ctx, addr, tabURL)
	if err != nil {
		return err
	}
	conn.Close()
	fmt.Println(targetID)
	return nil
}

func runTabsClose(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: browser tabs close <target-id>")
	}
	ctx := context.Background()
	if err := browser.CloseTab(ctx, sessionAddr(), args[0]); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "closed tab %s\n", args[0])
	return nil
}

func runTabsResize(args []string) error {
	fs := flag.NewFlagSet("tabs-resize", flag.ContinueOnError)
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
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
	conn, err := dialAddrTab(sessionAddr(), *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

	return browser.ResizeViewport(ctx, conn, width, height)
}
