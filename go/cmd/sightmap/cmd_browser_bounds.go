// Browser `bounds` subcommand: emit viewport-relative bounding boxes for
// components (or raw CSS selectors) at the live browser's current viewport and
// scroll position. The percentage values map directly onto a screenshot taken
// at the same browser state — see goggles-bb09.
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/compquery"
	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/observe"
	"github.com/sightmap/sightmap/go/sightmap"
)

//go:embed cmd_browser_bounds.js
var boundsJSSource string

// boundsJS is boundsJSSource with its __smDeepQueryAll dependency
// (deepquery.js) composed in once here, so the call site below just does
// boundsJS+<call> -- there's no "did I remember to prepend DeepQueryJS" to
// get wrong.
var boundsJS = browser.DeepQueryJS + "\n" + boundsJSSource

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
	addrFlag := fs.String("addr", "", "CDP address (host:port; default: the session for --sightmap-dir)")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (for component queries)")
	selectorFlag := fs.String("selector", "", "Query raw DOM elements by CSS selector instead of a component query")
	substringFlag := fs.Bool("substring", false, "Match a case-insensitive substring of the component NAME (name-only; skips the query grammar)")
	offscreenFlag := fs.Bool("include-offscreen", false, "Include components whose bounds fall entirely outside the viewport")
	allFlag := fs.Bool("all", false, "Emit bounds for every matched component (ignores positional queries)")

	queries, err := parseFlagsAroundArgs(fs, args)
	if err != nil {
		return err
	}

	if *selectorFlag == "" && !*allFlag && len(queries) == 0 {
		return fmt.Errorf("usage: browser bounds [QUERY...] | --selector SEL | --all\n" +
			"  QUERY     one or more component queries (name; [prop] predicates; descendant chains)\n" +
			"  --selector CSS selector (raw DOM, no sightmap needed)\n" +
			"  --all     every matched component on the page")
	}

	ctx := context.Background()
	conn, err := browser.Connect(resolveCDPAddr(*addrFlag, *sightmapDirFlag), *tabFlag)
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
// and returns bounds for the nodes selected by the positional queries. By
// default a query is a full component query (name + [prop] predicates +
// descendant chains) resolved with the same compquery engine as
// click/fill/hover/wait-for; --all emits every matched component and --substring
// switches to a name-only fuzzy match.
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
	matcher := match.NewMatcher(corpus)
	matches := matcher.Match(root, pageURL)
	if len(matches) == 0 {
		return nil, fmt.Errorf("bounds: no sightmap components matched the current page (%s)", pageURL)
	}

	var results []boundsResult
	seen := map[string]bool{}
	add := func(n *sightmap.ComponentNode, m *sightmap.ComponentMatch) {
		if m == nil || n.Bounds == nil || seen[n.Id] {
			return
		}
		seen[n.Id] = true
		results = append(results, makeBoundsResult(m.Name, n.Name, n.Id, n.Bounds, vw, vh))
	}

	switch {
	case all:
		sightmap.Walk(root, func(n *sightmap.ComponentNode, _ int) bool {
			add(n, matches[n])
			return true
		})

	case substring:
		// Fuzzy, name-only matching (case-insensitive substring). This mode does
		// not parse the query grammar — it exists to find a component when you
		// only remember part of its name.
		wanted := make([]string, len(queries))
		for i, q := range queries {
			wanted[i] = strings.ToLower(q)
		}
		matched := make([]bool, len(queries))
		sightmap.Walk(root, func(n *sightmap.ComponentNode, _ int) bool {
			m := matches[n]
			if m == nil {
				return true
			}
			lc := strings.ToLower(m.Name)
			for i, q := range wanted {
				if strings.Contains(lc, q) {
					matched[i] = true
					add(n, m)
				}
			}
			return true
		})
		warnUnmatchedQueries(queries, matched, true)

	default:
		// Full component-query grammar — names + property predicates + descendant
		// chains — via the same compquery engine as click/fill/hover/wait-for.
		// FindCandidates returns EVERY match, so bounds keeps its multi-instance
		// semantics instead of resolving to a single node.
		parsed := make([]*compquery.Query, len(queries))
		queryNames := map[string]bool{}
		for i, qs := range queries {
			q, perr := compquery.ParseQuery(qs)
			if perr != nil {
				return nil, fmt.Errorf("bounds: %w", perr)
			}
			parsed[i] = q
			for _, part := range q.Parts {
				queryNames[part.Name] = true
			}
		}
		// Extract properties for the queried component names so [prop] predicates
		// resolve (mirrors resolveComponentQuery; keeps the extraction small).
		relevant := make(map[*sightmap.ComponentNode]*sightmap.ComponentMatch)
		for n, m := range matches {
			if queryNames[m.Name] {
				relevant[n] = m
			}
		}
		components := matcher.Components(pageURL)
		compByName := make(map[string]sightmap.ComponentDef, len(components))
		for _, c := range components {
			compByName[c.Name] = c
		}
		observe.ExtractProperties(ctx, conn, relevant, compByName)
		props := propsByNodeID(matches)

		matched := make([]bool, len(queries))
		for i, q := range parsed {
			for _, n := range compquery.FindCandidates(root, matches, props, q) {
				matched[i] = true
				add(n, matches[n])
			}
		}
		warnUnmatchedQueries(queries, matched, false)
	}

	sortResults(results)
	return results, nil
}

// warnUnmatchedQueries prints a stderr note for each query that matched no
// component, hinting at --substring when we were in the exact/query mode.
func warnUnmatchedQueries(queries []string, matched []bool, substring bool) {
	for i := range queries {
		if matched[i] {
			continue
		}
		fmt.Fprintf(os.Stderr, "bounds: query %q matched no component", queries[i])
		if !substring {
			fmt.Fprintf(os.Stderr, " (try --substring for a partial name, or check the name with `sightmap snapshot`)")
		}
		fmt.Fprintln(os.Stderr)
	}
}

// boundsBySelector queries raw DOM elements and returns their viewport-% bounds,
// bypassing the sightmap corpus entirely.
func boundsBySelector(
	ctx context.Context,
	conn *browser.CDPConn,
	selector string,
	vw, vh int,
) ([]boundsResult, error) {
	script := boundsJS + fmt.Sprintf("\n__smBoundsBySelector(%s)", jsString(selector))

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
		b := &sightmap.Bounds{
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

// resolveScreenshotClip resolves --component or --selector to a single clip box:
// the union of the in-viewport bounding boxes of the matches, grown outward on
// all sides by expandPct percent of its own size. Returns an error when nothing
// in-viewport matches. The box is viewport-relative px, matching
// browser.ScreenshotClip's coordinate space.
func resolveScreenshotClip(
	ctx context.Context,
	conn *browser.CDPConn,
	sightmapDir, component, selector string,
	expandPct float64,
) (*browser.ScreenshotClip, error) {
	vw, vh, err := viewportSize(ctx, conn)
	if err != nil {
		return nil, err
	}
	var results []boundsResult
	if selector != "" {
		results, err = boundsBySelector(ctx, conn, selector, vw, vh)
	} else {
		results, err = boundsByComponent(ctx, conn, sightmapDir, []string{component}, false, false, vw, vh)
	}
	if err != nil {
		return nil, err
	}

	// Union the in-viewport match boxes.
	var x0, y0, x1, y1 float64
	have := false
	for _, r := range results {
		if !r.InViewport {
			continue
		}
		rx0, ry0 := float64(r.Px.X), float64(r.Px.Y)
		rx1, ry1 := rx0+float64(r.Px.Width), ry0+float64(r.Px.Height)
		if !have {
			x0, y0, x1, y1 = rx0, ry0, rx1, ry1
			have = true
			continue
		}
		x0, y0 = math.Min(x0, rx0), math.Min(y0, ry0)
		x1, y1 = math.Max(x1, rx1), math.Max(y1, ry1)
	}
	if !have {
		target := component
		if selector != "" {
			target = selector
		}
		return nil, fmt.Errorf("screenshot: no in-viewport match to clip to for %q", target)
	}

	// Grow outward on all sides by expandPct% of the box size.
	if expandPct > 0 {
		w, h := x1-x0, y1-y0
		dx, dy := w*expandPct/100, h*expandPct/100
		x0, y0, x1, y1 = x0-dx, y0-dy, x1+dx, y1+dy
	}
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	return &browser.ScreenshotClip{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0}, nil
}

// makeBoundsResult converts viewport-relative px bounds into viewport-% and
// determines whether the box intersects the viewport at all.
func makeBoundsResult(comp, label, id string, b *sightmap.Bounds, vw, vh int) boundsResult {
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
func intersectsViewport(b *sightmap.Bounds, vw, vh int) bool {
	if b.Width <= 0 || b.Height <= 0 {
		return false
	}
	return b.X < vw && b.X+b.Width > 0 && b.Y < vh && b.Y+b.Height > 0
}

// viewportSize reads window.innerWidth/innerHeight from the live page.
func viewportSize(ctx context.Context, conn *browser.CDPConn) (int, int, error) {
	raw, err := browser.EvalJSON(ctx, conn, boundsJS+"\n__smViewportSize()")
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
