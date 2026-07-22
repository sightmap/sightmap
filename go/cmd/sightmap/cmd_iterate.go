// iterate connects to Chrome, navigates, snaps, and prints coverage+clusters
// without printing the full tree.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

func runIterate(args []string) error {
	fs := flag.NewFlagSet("iterate", flag.ContinueOnError)
	addrFlag := fs.String("addr", browser.DefaultAddr(), "CDP address (host:port)")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDir := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir")
	waitFlag := fs.Float64("wait", 0, "Seconds to wait after navigation")
	includeHiddenFlag := fs.Bool("include-hidden", false, "Include hidden/off-screen nodes in analysis")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: iterate URL")
	}
	url := fs.Arg(0)

	// Apply .sightmap/config.yaml defaults for flags not explicitly set.
	{
		explicit := map[string]bool{}
		fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
		cfg := loadSiteConfig(*sightmapDir)
		if !explicit["wait"] && cfg.Snapshot.Wait > 0 {
			*waitFlag = cfg.Snapshot.Wait
		}
		if !explicit["include-hidden"] && cfg.Snapshot.IncludeHidden {
			*includeHiddenFlag = true
		}
	}

	visible := !*includeHiddenFlag

	// Auto-generate snap paths in /tmp.
	slug := urlSlug(url)
	snapOut := filepath.Join(os.TempDir(), "sightmap-iter-"+slug+".snap")
	treeOut := filepath.Join(os.TempDir(), "sightmap-iter-"+slug+".snap.tree.json")

	ctx := context.Background()

	// ── Connect to Chrome ────────────────────────────────────────────────────
	conn, err := dialAddrTab(*addrFlag, *tabFlag)
	if err != nil {
		return fmt.Errorf("iterate: %w", err)
	}
	defer conn.Close()

	// ── Navigate ─────────────────────────────────────────────────────────────
	if err := browser.NavigateAndWaitIdle(ctx, conn, url, 8*time.Second); err != nil {
		return fmt.Errorf("navigate: %v", err)
	}
	if *waitFlag > 0 {
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

	// ── Load sightmap (optional) ──────────────────────────────────────────────
	var snapshotMatches map[*comps.ComponentNode]*match.SightmapMatch
	var view *sightmap.View
	var viewComponents []match.SightmapComponent
	if _, statErr := os.Stat(*sightmapDir); statErr == nil {
		sess := sightmap.NewSession(sightmap.DirLoader(*sightmapDir))
		if m, mErr := sess.MatchTree(root, pageURL); mErr == nil {
			snapshotMatches = m
		} else {
			fmt.Fprintf(os.Stderr, "iterate: sightmap match: %v\n", mErr)
		}
		if v, vErr := sess.ViewForURL(pageURL); vErr == nil {
			view = v
		} else {
			fmt.Fprintf(os.Stderr, "iterate: view lookup: %v\n", vErr)
		}
		if components, cErr := sess.Components(pageURL); cErr == nil {
			viewComponents = components
		}
	}

	opts := printOpts{
		trace:          true,
		visibleOnly:    visible,
		viewComponents: viewComponents,
	}

	// ── Write snap file (full tree, silent to stdout) ────────────────────────
	{
		tmpPath := snapOut + ".tmp"
		f, ferr := os.Create(tmpPath)
		if ferr != nil {
			return fmt.Errorf("create snap file: %v", ferr)
		}
		writeOutput(f, root, view, snapshotMatches, opts)
		if cerr := f.Close(); cerr != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("close snap file: %v", cerr)
		}
		if rerr := os.Rename(tmpPath, snapOut); rerr != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("rename snap file: %v", rerr)
		}
	}

	// ── Write tree JSON ───────────────────────────────────────────────────────
	if err := writeTreeJSON(root, treeOut); err != nil {
		fmt.Fprintf(os.Stderr, "iterate: tree-out: %v\n", err)
	}

	// ── Build parent map for coverage ─────────────────────────────────────────
	var parentMap map[*comps.ComponentNode]*comps.ComponentNode
	if root != nil {
		parentMap = buildParentMap(root)
	}

	// ── Print condensed output ────────────────────────────────────────────────
	if view != nil {
		fmt.Fprintf(os.Stdout, "[View: %s %q]\n", view.Name, pageURL)
	} else {
		fmt.Fprintf(os.Stdout, "[View: %q]\n", pageURL)
	}

	if snapshotMatches != nil {
		t1, t2, t3, total, t3nodes, t2clusters, t2children := computeCoverage(root, snapshotMatches, parentMap, visible)
		writeCoverage(os.Stdout, t1, t2, t3, total, visible)
		if t3 > 0 {
			fmt.Fprintln(os.Stdout)
			writeTrace(os.Stdout, t3nodes, parentMap)
		}
		if len(t2clusters) > 0 {
			fmt.Fprintln(os.Stdout)
			writeT2Trace(os.Stdout, t2clusters, t2children, snapshotMatches, view)
		}
	}

	fmt.Fprintf(os.Stdout, "\nSnap saved: %s\n", snapOut)
	return nil
}

// urlSlug turns a URL into a short filesystem-safe slug.
//
//	"https://www.vividseats.com/concerts"     → "concerts"
//	"https://www.vividseats.com/"             → "home"
//	"https://www.vividseats.com/foo/bar/baz"  → "foo-bar-baz"
func urlSlug(rawURL string) string {
	s := rawURL
	// Strip scheme.
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	// Strip host (up to first /). If there's no slash, the entire remainder is
	// the host with no path → treat as home.
	if i := strings.Index(s, "/"); i >= 0 {
		s = s[i+1:]
	} else {
		s = ""
	}
	// Drop query string and fragment.
	if i := strings.Index(s, "?"); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, "#"); i >= 0 {
		s = s[:i]
	}
	s = strings.Trim(s, "/")
	if s == "" {
		return "home"
	}
	// Replace non-alphanumeric runs with -.
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(strings.ToLower(s), "-")
	s = strings.Trim(s, "-")
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}
