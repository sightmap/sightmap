// stats prints corpus-wide totals (views, components, requests, properties,
// memory entries) plus a per-view component/request table. Counting follows
// the corpus model, not a fresh YAML walk: the loader has already expanded
// $refs and flattened hierarchies, and AllComponents dedupes by first-seen
// name — so the component total counts distinct components corpus-wide, while
// each per-view row counts what is reachable in that view after expansion.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sightmap/sightmap/go/sightmap"
)

// corpusStats is stats' summary of a loaded corpus. The JSON field names are a
// published contract (CI in other repos consumes `stats --json`) — never
// rename them.
type corpusStats struct {
	Views      int         `json:"views"`
	Components int         `json:"components"` // distinct names corpus-wide (AllComponents)
	Requests   int         `json:"requests"`   // global + view-scoped
	Properties int         `json:"properties"` // summed over the distinct components
	Memory     int         `json:"memory"`     // file-, view-, component-, and request-level entries
	PerView    []viewStats `json:"per_view"`
}

// viewStats is one per-view row: the components and requests reachable in that
// view after $ref expansion. A global component reused by several views
// appears in each view's row but only once in the corpus-wide totals.
type viewStats struct {
	Name       string `json:"name"`
	Route      string `json:"route"`
	Components int    `json:"components"`
	Requests   int    `json:"requests"`
}

func runStats(args []string) error {
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	sightmapDir := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ directory")
	jsonFlag := fs.Bool("json", false, "Emit machine-readable JSON instead of the table")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: sightmap stats [--sightmap-dir DIR] [--json]\n\n")
		fmt.Fprintf(os.Stderr, "Prints corpus totals — views, components, requests, properties, memory\n")
		fmt.Fprintf(os.Stderr, "entries — plus a per-view component/request table.\n\n")
		fmt.Fprintf(os.Stderr, "Counting follows the corpus model: $refs are expanded and components are\n")
		fmt.Fprintf(os.Stderr, "deduped by first-seen name, so the component total counts distinct components\n")
		fmt.Fprintf(os.Stderr, "corpus-wide. Per-view rows count the components reachable in that view after\n")
		fmt.Fprintf(os.Stderr, "expansion, so a global component reused by several views appears in each\n")
		fmt.Fprintf(os.Stderr, "view's row but only once in the total, and global (view-less) components and\n")
		fmt.Fprintf(os.Stderr, "requests are counted in the totals only — the per-view columns therefore need\n")
		fmt.Fprintf(os.Stderr, "not sum to the totals.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	corpus, err := sightmap.Load(*sightmapDir)
	if err != nil {
		return fmt.Errorf("load corpus: %w", err)
	}

	s := computeStats(corpus)

	// An existing-but-empty corpus has nothing to count; teach the schema the
	// same way report does instead of printing an all-zero table.
	if s.Views == 0 && s.Components == 0 && s.Requests == 0 {
		return fmt.Errorf("empty corpus in %s — no views, components, or requests defined.\n"+
			"Define views under a top-level \"views:\" list:\n%s", *sightmapDir, viewSchemaExample())
	}

	if *jsonFlag {
		out, _ := json.MarshalIndent(s, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	printStats(s)
	return nil
}

// computeStats folds a loaded corpus into the stats summary. Totals come from
// the corpus model itself — AllComponents' first-seen-name dedupe over the
// $ref-expanded, flattened lists — so a global reused across views counts once
// corpus-wide, and its memory entries count once too.
func computeStats(c *sightmap.Corpus) corpusStats {
	s := corpusStats{
		Views:   len(c.Views),
		Memory:  len(c.Memory),
		PerView: make([]viewStats, 0, len(c.Views)),
	}

	all := c.AllComponents()
	s.Components = len(all)
	for _, comp := range all {
		s.Properties += len(comp.Properties)
		s.Memory += len(comp.Memory)
	}

	countRequests := func(defs []sightmap.RequestDef) {
		s.Requests += len(defs)
		for _, rd := range defs {
			s.Memory += len(rd.Memory)
		}
	}
	countRequests(c.Requests)

	for i := range c.Views {
		v := &c.Views[i]
		s.Memory += len(v.Memory)
		countRequests(v.Requests)
		s.PerView = append(s.PerView, viewStats{
			Name:       v.Name,
			Route:      v.Route,
			Components: len(v.Components),
			Requests:   len(v.Requests),
		})
	}
	return s
}

// printStats renders the human summary: a totals block, then a per-view table
// following report's formatting conventions (header line, ─ separators,
// width-capped name/route columns, trailing summary line).
func printStats(s corpusStats) {
	// ── Header ────────────────────────────────────────────────────────────────
	wd, _ := os.Getwd()
	site := lastPathComponent(wd)
	fmt.Printf("sightmap stats · %s · %s\n", site, time.Now().Format("2006-01-02"))

	// ── Compute column widths ─────────────────────────────────────────────────
	nameW := len("View")
	routeW := len("Route")
	for _, r := range s.PerView {
		if w := len(r.Name); w > nameW {
			nameW = w
		}
		if w := len(r.Route); w > routeW {
			routeW = w
		}
	}
	if nameW > 30 {
		nameW = 30
	}
	if routeW > 35 {
		routeW = 35
	}
	sep := strings.Repeat("─", nameW+routeW+25)

	// ── Totals ────────────────────────────────────────────────────────────────
	fmt.Println(sep)
	fmt.Printf(" %-10s  %d\n", "Views", s.Views)
	fmt.Printf(" %-10s  %d\n", "Components", s.Components)
	fmt.Printf(" %-10s  %d\n", "Requests", s.Requests)
	fmt.Printf(" %-10s  %d\n", "Properties", s.Properties)
	fmt.Printf(" %-10s  %d\n", "Memory", s.Memory)
	fmt.Println(sep)

	if len(s.PerView) == 0 {
		fmt.Println(" no views defined — totals cover global definitions only")
		return
	}

	// ── Per-view table ────────────────────────────────────────────────────────
	fmt.Printf(" %-*s  %-*s  %10s  %8s\n",
		nameW, "View", routeW, "Route", "Components", "Requests")
	fmt.Println(sep)

	sumComponents := 0
	for _, r := range s.PerView {
		sumComponents += r.Components
		fmt.Printf(" %-*s  %-*s  %10d  %8d\n",
			nameW, truncate(r.Name, nameW),
			routeW, truncate(r.Route, routeW),
			r.Components, r.Requests)
	}

	// ── Summary line ──────────────────────────────────────────────────────────
	fmt.Println(sep)
	fmt.Printf(" %d views  ·  %d distinct components (per-view rows sum to %d)\n",
		s.Views, s.Components, sumComponents)
}
