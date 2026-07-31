package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sightmap/sightmap/go/sightmap"
)

func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	sightmapDir := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	corpus, err := sightmap.Load(*sightmapDir)
	if err != nil {
		return fmt.Errorf("load corpus: %w", err)
	}

	findings := sightmap.Validate(corpus)

	// A corpus with global components but no views can never match a view, so
	// capture, report, per-view coverage, and route scoping are all unavailable.
	// This is almost always an authoring mistake (the views: list was never
	// created); nudge, but don't fail.
	if len(corpus.Views) == 0 && len(corpus.GlobalComponents) > 0 {
		findings = append(findings, sightmap.ValidationError{
			Code:     "no-views",
			Severity: sightmap.SeverityWarning,
			Message:  fmt.Sprintf("corpus defines %d global component(s) but no views — add a top-level \"views:\" list so pages can be matched (capture, report, and per-view coverage need it)", len(corpus.GlobalComponents)),
		})
	}

	// Print errors first, then warnings. Warnings are advisory (corpus conflicts
	// resolved by a fallback rule) and do NOT fail validation; only errors do.
	var errCount, warnCount int
	for _, f := range findings {
		if f.IsError() {
			errCount++
			fmt.Fprintf(os.Stderr, "error: %s\n", f.Error())
		}
	}
	for _, f := range findings {
		if !f.IsError() {
			warnCount++
			fmt.Fprintf(os.Stderr, "warning: %s\n", f.Error())
		}
	}

	if errCount > 0 {
		return fmt.Errorf("%d validation error(s), %d warning(s)", errCount, warnCount)
	}
	if warnCount > 0 {
		fmt.Fprintf(os.Stderr, "✓ no errors (%d warning(s))\n", warnCount)
		return nil
	}
	fmt.Fprintf(os.Stderr, "✓ no validation errors\n")
	return nil
}
