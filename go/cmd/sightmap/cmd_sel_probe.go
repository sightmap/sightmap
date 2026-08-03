// sel-probe queries the live browser for all DOM elements matching a CSS
// selector and prints per-match details: role, text, key attributes, and
// parent chain — with optional sightmap component annotation.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sel"
	"github.com/sightmap/sightmap/go/sightmap"
)

// queryScript is the inline JS injected via EvalJSON.
// Placeholders: %s = JSON-encoded selector, %d = max, %v = full (bool).
const queryScript = `(function(sel, max, full) {
  try {
    const els = [...document.querySelectorAll(sel)].slice(0, max);
    const interestingAttrs = new Set([
      'id','class','role','href','type','name','placeholder','aria-label',
      'aria-expanded','aria-haspopup','aria-controls','aria-selected',
      'data-testid','data-component','tabindex'
    ]);
    return els.map(el => {
      const attrs = {};
      for (const a of el.attributes) {
        if (interestingAttrs.has(a.name) || a.name.startsWith('aria-') || a.name.startsWith('data-')) {
          if (a.value !== '') attrs[a.name] = a.value;
        }
      }
      const parents = [];
      let p = el.parentElement;
      for (let i = 0; i < 5 && p && p.tagName !== 'HTML'; i++, p = p.parentElement) {
        parents.push({
          tag: p.tagName.toLowerCase(),
          id:  p.id || '',
          cls: [...p.classList].slice(0, 4),
          dt:  p.getAttribute('data-testid') || '',
          dc:  p.getAttribute('data-component') || '',
        });
      }
      return {
        tag:     el.tagName.toLowerCase(),
        id:      el.id || '',
        cls:     [...el.classList],
        role:    el.getAttribute('role') || '',
        text:    full ? el.textContent.trim() : el.textContent.trim().slice(0, 80),
        attrs:   attrs,
        parents: parents,
      };
    });
  } catch(e) {
    return {error: e.message};
  }
})(%s, %d, %v)`

// elementResult is the Go-side representation of one querySelectorAll hit.
type elementResult struct {
	Tag     string            `json:"tag"`
	Id      string            `json:"id"`
	Cls     []string          `json:"cls"`
	Role    string            `json:"role"`
	Text    string            `json:"text"`
	Attrs   map[string]string `json:"attrs"`
	Parents []parentInfo      `json:"parents"`
}

// parentInfo is one entry in the parent-chain walk (up to 5 levels).
type parentInfo struct {
	Tag string   `json:"tag"`
	Id  string   `json:"id"`
	Cls []string `json:"cls"`
	Dt  string   `json:"dt"` // data-testid
	Dc  string   `json:"dc"` // data-component
}

// compAnnotation pairs a component name with the first parsed SelectorPart of
// its first selector. Used for Go-side parent-chain annotation.
type compAnnotation struct {
	name string
	part *comps.SelectorPart
}

func runSelProbe(args []string) error {
	fs := flag.NewFlagSet("sel-probe", flag.ContinueOnError)
	addr := fs.String("addr", "", "CDP address (host:port; default: the session for --sightmap-dir)")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDir := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ directory for component annotation")
	maxN := fs.Int("max", 10, "Max matches to show")
	full := fs.Bool("full", false, "Show full text (no truncation to 80 chars)")
	allFlag := fs.Bool("all", false, "Check selector against every view URL in views/*.yaml")
	waitFlag := fs.Float64("wait", 0, "Seconds to wait after navigation when using --all (minimum 0.5)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: sel-probe [flags] -- 'css-selector'\n\n")
		fmt.Fprintf(os.Stderr, "The -- separator before the selector is recommended because selectors\n")
		fmt.Fprintf(os.Stderr, "can start with - and would otherwise be misinterpreted as flags.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}
	resolvedAddr := resolveCDPAddr(*addr, *sightmapDir)

	if *allFlag {
		return runSelProbeAll(fs.Args(), *sightmapDir, resolvedAddr, *tabFlag, *maxN, *full, *waitFlag)
	}

	remaining := fs.Args()
	if len(remaining) != 1 {
		fmt.Fprintln(os.Stderr, "sel-probe: expected exactly one selector argument")
		fmt.Fprintln(os.Stderr)
		fs.Usage()
		return fmt.Errorf("sel-probe: expected exactly one selector argument")
	}
	selector := remaining[0]

	ctx := context.Background()

	// Step 1: connect to Chrome.
	conn, err := browser.Connect(resolvedAddr, *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Step 2: run the query script.
	results, err := queryElements(ctx, conn, selector, *maxN, *full)
	if err != nil {
		return fmt.Errorf("sel-probe: query failed: %v", err)
	}

	fmt.Printf("selector: %s\n", selector)
	fmt.Printf("matches: %d\n", len(results))

	// Cross-check against the OFFLINE matcher. sel-probe queries the live DOM,
	// but snapshot/coverage/capture match the extracted component tree with the
	// Go matcher, which supports a subset of CSS and a reduced node model. When
	// the two disagree, a "verified" selector can be silently dead downstream, so
	// surface the divergence loudly. Best-effort: never fail sel-probe on it.
	printOfflineCheck(ctx, conn, selector, len(results))

	if len(results) == 0 {
		return nil
	}

	// Step 3: load sightmap annotations (best-effort; silently skipped if absent).
	annotations := loadCompAnnotations(*sightmapDir)

	// Print nearest component summary before per-match details.
	if nearest := summarizeNearestComponents(results, annotations); len(nearest) > 0 {
		if len(nearest) == 1 {
			fmt.Printf("nearest component: ★ %s  (%s)\n", nearest[0].name, nearest[0].desc)
		} else {
			parts := make([]string, 0, len(nearest))
			for _, nc := range nearest {
				parts = append(parts, fmt.Sprintf("★ %s (%d×)", nc.name, nc.count))
			}
			fmt.Printf("nearest component: %s\n", strings.Join(parts, "  "))
		}
	}

	// Step 4: format and print each match.
	for i, res := range results {
		fmt.Println()
		fmt.Printf("[%d] %s\n", i+1, elementDesc(res.Tag, res.Id, res.Cls))

		if res.Role != "" {
			fmt.Printf("    role:  %s\n", res.Role)
		}

		if res.Text != "" {
			text := res.Text
			if !*full && len(text) > 80 {
				text = text[:80]
			}
			fmt.Printf("    text:  %q\n", text)
		}

		// Attrs: exclude id and class (already shown in the element descriptor).
		// Sort keys for stable output.
		var attrParts []string
		for k, v := range res.Attrs {
			if k == "id" || k == "class" {
				continue
			}
			attrParts = append(attrParts, fmt.Sprintf("%s=%q", k, v))
		}
		if len(attrParts) > 0 {
			sort.Strings(attrParts)
			fmt.Printf("    attrs: %s\n", strings.Join(attrParts, "  "))
		}

		// Parents (up to 5 levels, collected by the JS script).
		if len(res.Parents) > 0 {
			fmt.Printf("    parents:\n")
			for _, p := range res.Parents {
				line := "      " + elementDesc(p.Tag, p.Id, p.Cls)
				if name := findCompAnnotation(annotations, p); name != "" {
					line += "  ★ " + name
				}
				fmt.Println(line)
			}
		}
	}
	return nil
}

// printOfflineCheck compares the live querySelectorAll count against the offline
// matcher's count for the same selector and warns when they diverge. The offline
// matcher is what snapshot/coverage/capture use, so a divergence means the
// selector will behave differently there than sel-probe's live query suggests.
// liveShown is the (possibly --max-capped) number of results already printed; the
// true live total is re-counted here for an honest comparison.
func printOfflineCheck(ctx context.Context, conn *browser.CDPConn, selector string, liveShown int) {
	liveN, lErr := liveSelectorCount(ctx, conn, selector)
	if lErr != nil {
		liveN = liveShown // fall back to what we showed
	}
	offN, oErr := offlineSelectorCount(ctx, conn, selector)
	if oErr != nil {
		return // extraction/match unavailable — skip the cross-check silently
	}

	fmt.Printf("offline matcher: %d match%s (snapshot/coverage/capture use this)\n", offN, pluralS(offN))
	if offN == liveN {
		return
	}
	fmt.Printf("⚠ offline/live divergence: live DOM matches %d, but the offline matcher sees %d.\n"+
		"  snapshot/coverage/capture will see %d — don't trust a live-only match here.\n", liveN, offN, offN)
	for _, h := range offlineDivergenceHints(selector) {
		fmt.Printf("  - %s\n", h)
	}
}

// liveSelectorCount returns the true (uncapped) number of DOM elements matching
// selector in the live page.
func liveSelectorCount(ctx context.Context, conn *browser.CDPConn, selector string) (int, error) {
	selJSON, _ := json.Marshal(selector)
	raw, err := browser.EvalJSON(ctx, conn, fmt.Sprintf(
		`(function(s){try{return document.querySelectorAll(s).length}catch(e){return -1}})(%s)`, string(selJSON)))
	if err != nil {
		return 0, err
	}
	var n int
	if json.Unmarshal(raw, &n) != nil || n < 0 {
		return 0, fmt.Errorf("live count eval failed")
	}
	return n, nil
}

// offlineSelectorCount extracts the component tree from the live page and counts
// how many nodes the offline matcher assigns to selector — exactly what
// snapshot/coverage/capture would see.
func offlineSelectorCount(ctx context.Context, conn *browser.CDPConn, selector string) (int, error) {
	page, err := conn.DefaultPage()
	if err != nil {
		return 0, err
	}
	root, err := browser.ExtractComponents(ctx, page)
	if err != nil {
		return 0, err
	}
	defs := []match.ComponentDef{{Name: "__probe__", Selectors: []string{selector}}}
	return len(match.ApplySightmap(root, defs)), nil
}

// offlineDivergenceHints returns human-readable reasons a selector might match
// live but not offline, derived from the parsed selector. It recognizes the two
// best-known traps — attribute selectors on `id` and on `class` — and otherwise
// gives a general nudge about the reduced offline node model.
func offlineDivergenceHints(selector string) []string {
	ps, err := sel.ParseSightmapSelector(selector)
	if err != nil {
		return []string{"the offline selector parser could not parse this selector, so it matches nothing offline"}
	}
	var hints []string
	seen := map[string]bool{}
	add := func(h string) {
		if !seen[h] {
			seen[h] = true
			hints = append(hints, h)
		}
	}
	for _, part := range ps.Parts {
		if part == nil {
			continue
		}
		if _, ok := part.Attrs["id"]; ok {
			add("attribute selectors on `id` (e.g. [id^=\"…\"]) don't match offline — `id` is a dedicated node field; use `#id` instead")
		}
		if _, ok := part.Attrs["class"]; ok {
			add("attribute selectors on `class` (e.g. [class*=\"…\"]) never match offline — use a `.class` selector instead")
		}
	}
	if len(hints) == 0 {
		add("the offline node model may not capture the attribute(s) or structure this selector relies on (e.g. non-standard form attributes like `placeholder`, or `class` on SVG elements)")
	}
	return hints
}

// queryElements runs the inline JS and returns the parsed element list.
func queryElements(
	ctx context.Context,
	conn *browser.CDPConn,
	selector string,
	maxN int,
	full bool,
) ([]elementResult, error) {
	selectorJSON, err := json.Marshal(selector)
	if err != nil {
		return nil, fmt.Errorf("marshal selector: %w", err)
	}

	script := fmt.Sprintf(queryScript, string(selectorJSON), maxN, full)

	raw, err := browser.EvalJSON(ctx, conn, script)
	if err != nil {
		return nil, err
	}

	// The script returns {error: "..."} on JS exception.
	var errObj struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &errObj) == nil && errObj.Error != "" {
		return nil, fmt.Errorf("JS: %s", errObj.Error)
	}

	var results []elementResult
	if err := json.Unmarshal(raw, &results); err != nil {
		return nil, fmt.Errorf("unmarshal results: %w", err)
	}
	return results, nil
}

// loadCompAnnotations reads the .sightmap/ corpus and builds one
// (name, SelectorPart) annotation per component from its first selector's
// first part. Returns nil (silently) if dir doesn't exist or loading fails.
func loadCompAnnotations(dir string) []compAnnotation {
	if _, statErr := os.Stat(dir); statErr != nil {
		return nil
	}

	corpus, err := sightmap.Load(dir)
	if err != nil {
		return nil
	}
	components := corpus.Components("")

	annotations := make([]compAnnotation, 0, len(components))
	for _, comp := range components {
		if len(comp.Selectors) == 0 {
			continue
		}
		ps, parseErr := sel.ParseSightmapSelector(comp.Selectors[0])
		if parseErr != nil || len(ps.Parts) == 0 {
			continue
		}
		annotations = append(annotations, compAnnotation{
			name: comp.Name,
			part: ps.Parts[0],
		})
	}
	return annotations
}

// findCompAnnotation returns the name of the first component whose first
// SelectorPart matches the parent node, or "" if none match.
func findCompAnnotation(annotations []compAnnotation, p parentInfo) string {
	node := &comps.SelectorPart{
		Tag:     p.Tag,
		Id:      p.Id,
		Classes: p.Cls,
		Attrs:   make(map[string]string),
	}
	if p.Dt != "" {
		node.Attrs["data-testid"] = p.Dt
	}
	if p.Dc != "" {
		node.Attrs["data-component"] = p.Dc
	}

	for _, ann := range annotations {
		if sel.Matches(node, ann.part) {
			return ann.name
		}
	}
	return ""
}

// nearestComp holds a component name, a representative parent descriptor, and
// the count of matched results whose nearest ancestor is this component.
type nearestComp struct {
	name  string
	desc  string
	count int
}

// summarizeNearestComponents walks every result's parent chain, finds the
// nearest annotated component ancestor for each match, and returns a
// frequency-sorted slice of unique component names. Results whose parent chain
// contains no known component are skipped. Returns nil when no component is
// found across all matches.
func summarizeNearestComponents(results []elementResult, annotations []compAnnotation) []nearestComp {
	if len(annotations) == 0 {
		return nil
	}

	type entry struct {
		desc  string
		count int
	}
	byName := make(map[string]*entry)

	for _, res := range results {
		for _, p := range res.Parents {
			name := findCompAnnotation(annotations, p)
			if name == "" {
				continue
			}
			desc := parentDesc(p)
			if e, ok := byName[name]; ok {
				e.count++
			} else {
				byName[name] = &entry{desc: desc, count: 1}
			}
			break // nearest ancestor only; don't double-count for this result
		}
	}

	if len(byName) == 0 {
		return nil
	}

	out := make([]nearestComp, 0, len(byName))
	for name, e := range byName {
		out = append(out, nearestComp{name: name, desc: e.desc, count: e.count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].count != out[j].count {
			return out[i].count > out[j].count
		}
		return out[i].name < out[j].name
	})
	return out
}

// parentDesc formats a parent node for the nearest-component summary line.
// Prefers data-testid when present: tag[data-testid="val"].
func parentDesc(p parentInfo) string {
	if p.Dt != "" {
		return fmt.Sprintf(`%s[data-testid="%s"]`, p.Tag, p.Dt)
	}
	return elementDesc(p.Tag, p.Id, p.Cls)
}

// elementDesc formats an element identity as tag[#id][.class1.class2...].
func elementDesc(tag, id string, classes []string) string {
	s := tag
	if id != "" {
		s += "#" + id
	}
	for _, c := range classes {
		s += "." + c
	}
	return s
}

// runSelProbeAll navigates to each probe URL and reports how many DOM elements
// match the given selector on each page.
func runSelProbeAll(args []string, sightmapDir, addr, tabID string, max int, full bool, wait float64) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: sel-probe --all [flags] 'selector'")
	}
	selector := args[0]

	corpus, err := sightmap.Load(sightmapDir)
	if err != nil {
		return fmt.Errorf("sel-probe --all: load corpus: %w", err)
	}
	targets, err := corpus.ProbeTargets()
	if err != nil {
		return err
	}

	ctx := context.Background()
	conn, err := browser.Connect(addr, tabID)
	if err != nil {
		return fmt.Errorf("sel-probe --all: %w", err)
	}
	defer conn.Close()

	selectorJSON, _ := json.Marshal(selector)

	for _, v := range targets {
		label := v.ViewName
		if v.SnapName != "base" {
			label = v.ViewName + "/" + v.SnapName
		}
		if err := browser.NavigateAndWait(ctx, conn, v.URL); err != nil {
			fmt.Printf("%-24s  ERROR: %v\n", label, err)
			continue
		}
		w := wait
		if w < 0.5 {
			w = 0.5
		}
		time.Sleep(time.Duration(w * float64(time.Second)))

		script := fmt.Sprintf(`(function(sel,max){
            try {
                var els = Array.from(document.querySelectorAll(sel)).slice(0,max);
                return els.length;
            } catch(e) { return -1; }
        })(%s, %d)`, string(selectorJSON), max)

		raw, evalErr := browser.EvalJSON(ctx, conn, script)
		if evalErr != nil {
			fmt.Printf("%-24s  ERROR: %v\n", label, evalErr)
			continue
		}
		var count int
		if err := json.Unmarshal(raw, &count); err != nil || count < 0 {
			fmt.Printf("%-24s  ERROR: eval failed\n", label)
			continue
		}
		fmt.Printf("%-24s  sel=%s  %d match%s\n", label, selector, count, pluralS(count))
	}
	return nil
}

// pluralS returns "es" for n != 1, "" for n == 1, matching "match/matches".
func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "es"
}
