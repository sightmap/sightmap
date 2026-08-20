// stats prints corpus-wide totals (views, components, requests, properties,
// memory entries) plus a per-view component/request table. The counting lives
// in the library — sightmap.Corpus.Stats, whose doc comment is the one
// authoritative home for what each total means — so external consumers (the
// atlas index generator, Subtext) get the same numbers without shelling out.
// This file is the adapter: parse flags, refuse a corpus whose counts would
// under-report, format.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sightmap/sightmap/go/sightmap"
)

// statsFailure is the machine-readable failure envelope emitted on stdout when
// `stats --json` cannot produce trustworthy counts. `--json` consumers parse
// stdout unconditionally, so a bare human error on stderr would leave them
// parsing nothing; instead every --json run prints one JSON object, and a
// present "error" key (never present on success) means the run failed. The
// exit code is nonzero either way.
type statsFailure struct {
	Error       string            `json:"error"`
	Diagnostics []statsDiagnostic `json:"diagnostics"`
}

// statsDiagnostic is one validation finding in the failure envelope, carrying
// the same fields sightmap.ValidationError prints in `sightmap validate`.
type statsDiagnostic struct {
	Code      string `json:"code,omitempty"`
	File      string `json:"file,omitempty"`
	Component string `json:"component,omitempty"`
	Selector  string `json:"selector,omitempty"`
	Message   string `json:"message"`
}

// runStats is the entry point registered in main.go.
func runStats(args []string) error {
	return runStatsOut(args, os.Stdout)
}

// runStatsOut is the testable core: it writes all output to out.
func runStatsOut(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	sightmapDir := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ directory")
	jsonFlag := fs.Bool("json", false, "Emit machine-readable JSON instead of the table")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: sightmap stats [--sightmap-dir DIR] [--json]\n\n")
		fmt.Fprintf(os.Stderr, "Prints corpus totals — views, components, requests, properties, memory\n"+
			"entries — plus a per-view component/request table. Counts follow the loaded\n"+
			"corpus model, with $refs expanded and hierarchies flattened. A corpus with\n"+
			"validation errors is refused: its dropped definitions would under-report.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q — stats takes no positional arguments", fs.Arg(0))
	}

	corpus, err := sightmap.Load(*sightmapDir)
	if err != nil {
		return fmt.Errorf("load corpus: %w", err)
	}

	// Load is lenient: an unresolved $ref, or a component missing its name or
	// selector, DROPS the definition and records an error-severity diagnostic
	// rather than failing. Counting that corpus would report smaller numbers
	// with a zero exit — silently wrong data for the CI consumers of --json —
	// so refuse anything `sightmap validate` would reject. Warnings are
	// advisory (a conflict a fallback rule resolved) and do not block.
	if bad := errorFindings(sightmap.Validate(corpus)); len(bad) > 0 {
		return reportStatsUncountable(out, *jsonFlag, *sightmapDir, bad)
	}

	s := corpus.Stats()

	// --json is answered before the empty-corpus gate: a consumer asking for
	// machine-readable output gets the documented shape with zero counts, not
	// a human error it cannot parse.
	if *jsonFlag {
		return writeJSONLine(out, s)
	}

	// A corpus with nothing in it has nothing to tabulate; teach the schema the
	// same way report does instead of printing an all-zero table. Memory counts
	// as content — a memory-only corpus is legal and is not empty.
	if s.IsEmpty() {
		return fmt.Errorf("empty corpus in %s — no views, components, requests, or memory defined.\n"+
			"Define views under a top-level \"views:\" list:\n%s", *sightmapDir, viewSchemaExample())
	}

	printStats(out, s)
	return nil
}

// errorFindings keeps only the error-severity validation findings.
func errorFindings(findings []sightmap.ValidationError) []sightmap.ValidationError {
	var out []sightmap.ValidationError
	for _, f := range findings {
		if f.IsError() {
			out = append(out, f)
		}
	}
	return out
}

// reportStatsUncountable renders the "this corpus cannot be counted" failure in
// whichever form the caller asked for, and returns the error that sets the exit
// code. In --json mode the machine-readable envelope goes to stdout; otherwise
// the findings are listed on stderr the way `sightmap validate` lists them.
func reportStatsUncountable(out io.Writer, asJSON bool, dir string, bad []sightmap.ValidationError) error {
	summary := fmt.Sprintf("%d validation error(s) in %s — counts would under-report the dropped definitions; run `sightmap validate` and fix them first", len(bad), dir)

	if asJSON {
		envelope := statsFailure{Error: summary, Diagnostics: make([]statsDiagnostic, 0, len(bad))}
		for _, f := range bad {
			envelope.Diagnostics = append(envelope.Diagnostics, statsDiagnostic{
				Code:      f.Code,
				File:      f.File,
				Component: f.Component,
				Selector:  f.Selector,
				Message:   f.Message,
			})
		}
		if err := writeJSONLine(out, envelope); err != nil {
			return err
		}
		return fmt.Errorf("%s", summary)
	}

	for _, f := range bad {
		fmt.Fprintf(os.Stderr, "error: %s\n", f.Error())
	}
	return fmt.Errorf("%s", summary)
}

// writeJSONLine marshals v as indented JSON followed by a newline.
func writeJSONLine(out io.Writer, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s\n", b)
	return err
}

// printStats renders the human summary: a totals block, then a per-view table
// in the shared offline-table style (banner, ─ separators, width-capped
// name/route columns, trailing summary line).
func printStats(out io.Writer, s sightmap.Stats) {
	printTableBanner(out, "stats")

	names := make([]string, len(s.PerView))
	routes := make([]string, len(s.PerView))
	for i, r := range s.PerView {
		names[i] = r.Name
		routes[i] = r.Route
	}
	nameW, routeW := viewColWidths(names, routes)
	sep := strings.Repeat("─", nameW+routeW+25)

	// ── Totals ────────────────────────────────────────────────────────────────
	fmt.Fprintln(out, sep)
	fmt.Fprintf(out, " %-10s  %d\n", "Views", s.Views)
	fmt.Fprintf(out, " %-10s  %d\n", "Components", s.Components)
	fmt.Fprintf(out, " %-10s  %d\n", "Requests", s.Requests)
	fmt.Fprintf(out, " %-10s  %d\n", "Messages", s.Messages)
	fmt.Fprintf(out, " %-10s  %d\n", "Properties", s.Properties)
	fmt.Fprintf(out, " %-10s  %d\n", "Memory", s.Memory)
	fmt.Fprintln(out, sep)

	if len(s.PerView) == 0 {
		fmt.Fprintln(out, " no views defined — totals cover corpus-level definitions only")
		return
	}

	// ── Per-view table ────────────────────────────────────────────────────────
	fmt.Fprintf(out, " %-*s  %-*s  %10s  %8s\n",
		nameW, "View", routeW, "Route", "Components", "Requests")
	fmt.Fprintln(out, sep)

	sumComponents := 0
	for _, r := range s.PerView {
		sumComponents += r.Components
		fmt.Fprintf(out, " %-*s  %-*s  %10d  %8d\n",
			nameW, truncate(r.Name, nameW),
			routeW, truncate(r.Route, routeW),
			r.Components, r.Requests)
	}

	// ── Summary line ──────────────────────────────────────────────────────────
	fmt.Fprintln(out, sep)
	fmt.Fprintf(out, " %d views  ·  %d distinct components (per-view rows sum to %d)\n",
		s.Views, s.Components, sumComponents)
}
