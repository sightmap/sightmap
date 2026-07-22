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

	loader := sightmap.DirLoader(*sightmapDir)
	corpus, err := loader.Load()
	if err != nil {
		return fmt.Errorf("load corpus: %w", err)
	}

	errs := sightmap.Validate(corpus)
	if len(errs) == 0 {
		fmt.Fprintf(os.Stderr, "✓ no validation errors\n")
		return nil
	}
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "error: %s\n", e.Error())
	}
	return fmt.Errorf("%d validation error(s)", len(errs))
}
