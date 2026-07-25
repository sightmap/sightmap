// Browser `bounds` subcommand: emit viewport-relative bounding boxes for
// components (or raw CSS selectors) at the live browser's current viewport and
// scroll position. The percentage values map directly onto a screenshot taken
// at the same browser state — see goggles-bb09.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/sightmap"
)

// boundsResult is one emitted bounding box, in both viewport-% and raw px.
type boundsResult struct {
	Comp       string   `json:"comp"`
	Label      string   `json:"label"`
	Id         string   `json:"id,omitempty"`
	Top        float64  `json:"top"`
	Left       float64  `json:"left"`
	Width      float64  `json:"width"`
	Height     float64  `json:"height"`
	Px         boundsPx `json:"px"`
	InViewport bool     `json:"inViewport"`
}

type boundsPx struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func runBounds(args []string) error {
	fs := flag.NewFlagSet("bounds", flag.ContinueOnError)
	addrFlag := fs.String("addr", browser.DefaultAddr(), "CDP address (host:port)")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (for component-name queries)")
	selectorFlag := fs.String("selector", "", "Query raw DOM elements by CSS selector instead of component name")
	substringFlag := fs.Bool("substring", false, "Match component names by case-insensitive substring (default: exact, case-insensitive)")
	offscreenFlag := fs.Bool("include-offscreen", false, "Include components whose bounds fall entirely outside the viewport")
	allFlag := fs.Bool("all", false, "Emit bounds for every matched component (ignores positional queries)")

	// Allow flags and positional queries in any order (Go's flag package stops
	// at the first positional, so `bounds Foo --addr X` would otherwise swallow
	// --addr as a query). Parse flags, take a positional, repeat.
	var queries []string
	rest := args
	for len(rest) > 0 {
		if err := fs.Parse(rest); err != nil {
			return err
		}
		rest = fs.Args()
		if len(rest) == 0 {
			break
		}
		queries = append(queries, rest[0])
		rest = rest[1:]
	}

	if *selectorFlag == "" && !*allFlag && len(queries) == 0 {
		return fmt.Errorf("usage: browser bounds [QUERY...] | --selector SEL | --all\n" +
			"  QUERY     one or more sightmap component names\n" +
			"  --selector CSS selector (raw DOM, no sightmap needed)\n" +
			"  --all     every matched component on the page")
	}

	ctx := context.Background()
	conn, err := browser.Connect(*addrFlag, *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Live viewport dimensions — bounds are viewport-relative, so this is the
	// denominator for the percentage conversion.
	vw, vh, err := viewportSize(ctx, conn)
	if err != nil {
		return err
	}
	if vw <= 0 || vh <= 0 {
		return fmt.Errorf("bounds: invalid viewport size %dx%d", vw, vh)
	}
	fmt.Fprintf(os.Stderr, "viewport: %dx%d\n", vw, vh)

	var results []boundsResult

	if *selectorFlag != "" {
		results, err = boundsBySelector(ctx, conn, *selectorFlag, vw, vh)
		if err != nil {
			return err
		}
	} else {
		results, err = boundsByComponent(ctx, conn, *sightmapDirFlag, queries,
			*allFlag, *substringFlag, vw, vh)
		if err != nil {
			return err
		}
	}

	// Filter offscreen unless asked to keep them.
	if !*offscreenFlag {
		kept := results[:0]
		dropped := 0
		for _, r := range results {
			if r.InViewport {
				kept = append(kept, r)
			} else {
				dropped++
			}
		}
		results = kept
		if dropped > 0 {
			fmt.Fprintf(os.Stderr, "skipped %d offscreen match(es) (use --include-offscreen to keep)\n", dropped)
		}
	}

	if results == nil {
		results = []boundsResult{}
	}
	out, _ := json.MarshalIndent(results, "", "  ")
	fmt.Println(string(out))
	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "bounds: no matching components in viewport")
	}
	return nil
}

// boundsByComponent extracts the component tree, applies the sightmap corpus,
// and returns bounds for nodes whose matched component name satisfies the query.
func boundsByComponent(
	ctx context.Context,
	conn *browser.CDPConn,
	sightmapDir string,
	queries []string,
	all, substring bool,
	vw, vh int,
) ([]boundsResult, error) {
	if _, statErr := os.Stat(sightmapDir); statErr != nil {
		return nil, fmt.Errorf("bounds: sightmap dir %q not found (use --selector for raw DOM queries): %v", sightmapDir, statErr)
	}

	page, err := conn.DefaultPage()
	if err != nil {
		return nil, fmt.Errorf("bounds: default page: %w", err)
	}
	// useScrollOffset=false → bounds are viewport-relative (matches screenshot).
	root, err := browser.ExtractComponents(ctx, page)
	if err != nil {
		return nil, fmt.Errorf("bounds: extract components: %w", err)
	}

	pageURL, err := browser.GetURL(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("bounds: get URL: %w", err)
	}

	corpus, err := sightmap.Load(sightmapDir)
	if err != nil {
		return nil, fmt.Errorf("bounds: load corpus: %w", err)
	}
	matches := corpus.MatchTree(root, pageURL)
	if len(matches) == 0 {
		return nil, fmt.Errorf("bounds: no sightmap components matched the current page (%s)", pageURL)
	}

	// Normalize the query set for matching.
	wanted := make([]string, len(queries))
	for i, q := range queries {
		wanted[i] = strings.ToLower(q)
	}
	nameMatches := func(comp string) bool {
		if all {
			return true
		}
		lc := strings.ToLower(comp)
		for _, q := range wanted {
			if substring {
				if strings.Contains(lc, q) {
					return true
				}
			} else if lc == q {
				return true
			}
		}
		return false
	}

	var results []boundsResult
	seenComp := map[string]bool{}
	comps.Walk(root, func(n *comps.ComponentNode, _ int) bool {
		m := matches[n]
		if m == nil || !nameMatches(m.Name) {
			return true
		}
		if n.Bounds == nil {
			return true
		}
		seenComp[m.Name] = true
		results = append(results, makeBoundsResult(m.Name, n.Name, n.Id, n.Bounds, vw, vh))
		return true
	})

	// Warn about queries that matched no component, with a substring hint.
	if !all {
		matchedNames := map[string]bool{}
		for name := range seenComp {
			matchedNames[strings.ToLower(name)] = true
		}
		for i, q := range wanted {
			hit := false
			for name := range matchedNames {
				if (substring && strings.Contains(name, q)) || (!substring && name == q) {
					hit = true
					break
				}
			}
			if !hit {
				fmt.Fprintf(os.Stderr, "bounds: query %q matched no component", queries[i])
				if !substring {
					fmt.Fprintf(os.Stderr, " (try --substring, or check the name with `sightmap snapshot`)")
				}
				fmt.Fprintln(os.Stderr)
			}
		}
	}

	sortResults(results)
	return results, nil
}

// boundsBySelector queries raw DOM elements and returns their viewport-% bounds,
// bypassing the sightmap corpus entirely.
func boundsBySelector(
	ctx context.Context,
	conn *browser.CDPConn,
	selector string,
	vw, vh int,
) ([]boundsResult, error) {
	// Return the rects directly (not stringified) so EvalJSON gives us the
	// object value rather than a JSON-encoded string.
	script := fmt.Sprintf(`(function(){
  var els = document.querySelectorAll(%s);
  var out = [];
  for (var i = 0; i < els.length; i++) {
    var r = els[i].getBoundingClientRect();
    var label = (els[i].getAttribute('aria-label') || els[i].textContent || '').trim().replace(/\s+/g,' ');
    if (label.length > 80) label = label.slice(0, 80);
    out.push({x: r.left, y: r.top, width: r.width, height: r.height, label: label});
  }
  return out;
})()`, jsString(selector))

	raw, err := browser.EvalJSON(ctx, conn, script)
	if err != nil {
		return nil, fmt.Errorf("bounds: selector eval: %w", err)
	}
	var rects []struct {
		X      float64 `json:"x"`
		Y      float64 `json:"y"`
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
		Label  string  `json:"label"`
	}
	if err := json.Unmarshal(raw, &rects); err != nil {
		return nil, fmt.Errorf("bounds: parse selector result: %w", err)
	}

	results := make([]boundsResult, 0, len(rects))
	for _, r := range rects {
		b := &comps.Bounds{
			X:      int(math.Round(r.X)),
			Y:      int(math.Round(r.Y)),
			Width:  int(math.Round(r.Width)),
			Height: int(math.Round(r.Height)),
		}
		results = append(results, makeBoundsResult(selector, r.Label, "", b, vw, vh))
	}
	sortResults(results)
	return results, nil
}

// makeBoundsResult converts viewport-relative px bounds into viewport-% and
// determines whether the box intersects the viewport at all.
func makeBoundsResult(comp, label, id string, b *comps.Bounds, vw, vh int) boundsResult {
	return boundsResult{
		Comp:       comp,
		Label:      label,
		Id:         id,
		Top:        round1(float64(b.Y) / float64(vh) * 100),
		Left:       round1(float64(b.X) / float64(vw) * 100),
		Width:      round1(float64(b.Width) / float64(vw) * 100),
		Height:     round1(float64(b.Height) / float64(vh) * 100),
		Px:         boundsPx{X: b.X, Y: b.Y, Width: b.Width, Height: b.Height},
		InViewport: intersectsViewport(b, vw, vh),
	}
}

// intersectsViewport reports whether the box overlaps the [0,vw]x[0,vh] region.
func intersectsViewport(b *comps.Bounds, vw, vh int) bool {
	if b.Width <= 0 || b.Height <= 0 {
		return false
	}
	return b.X < vw && b.X+b.Width > 0 && b.Y < vh && b.Y+b.Height > 0
}

// viewportSize reads window.innerWidth/innerHeight from the live page.
func viewportSize(ctx context.Context, conn *browser.CDPConn) (int, int, error) {
	raw, err := browser.EvalJSON(ctx, conn,
		`({w: window.innerWidth, h: window.innerHeight})`)
	if err != nil {
		return 0, 0, fmt.Errorf("bounds: read viewport: %w", err)
	}
	var vp struct {
		W int `json:"w"`
		H int `json:"h"`
	}
	if err := json.Unmarshal(raw, &vp); err != nil {
		return 0, 0, fmt.Errorf("bounds: parse viewport: %w", err)
	}
	return vp.W, vp.H, nil
}

// sortResults orders results top-to-bottom, then left-to-right, for stable,
// readable output.
func sortResults(rs []boundsResult) {
	sort.SliceStable(rs, func(i, j int) bool {
		if rs[i].Px.Y != rs[j].Px.Y {
			return rs[i].Px.Y < rs[j].Px.Y
		}
		return rs[i].Px.X < rs[j].Px.X
	})
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

// jsString returns a JSON-quoted string literal safe to embed in a JS expression.
func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
