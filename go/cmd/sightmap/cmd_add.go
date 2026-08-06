// add installs a published sightmap corpus from the community atlas
// (github.com/sightmap/atlas) into the current project, so a visitor can run
// one command from a gallery page and get a working .sightmap/ directory. The
// index schema, the URL layout, the validation rules, and the install itself
// live in the atlas package; this file is flags in, formatted output out.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sightmap/sightmap/go/atlas"
)

// runAdd is the entry point registered in main.go.
func runAdd(args []string) error {
	return runAddOut(args, os.Stdout)
}

// runAddOut is the testable core: it writes all output to out.
func runAddOut(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	indexURL := fs.String("index", atlas.DefaultIndexURL, "Atlas index.json URL (for mirrors and tests)")
	target := fs.String("target", ".sightmap", "Directory to install the corpus into")
	force := fs.Bool("force", false, "Install into a non-empty target directory, replacing its contents (refused unless it holds only corpus files)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: sightmap add SLUG [--index URL] [--target DIR] [--force]\n\n")
		fmt.Fprintf(os.Stderr, "Installs a published sightmap corpus from the community atlas\n")
		fmt.Fprintf(os.Stderr, "(github.com/sightmap/atlas) into TARGET (default ./.sightmap).\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fs.Usage()
		return fmt.Errorf("expected a SLUG argument")
	}
	slug := rest[0]
	// Re-parse what follows the slug so `sightmap add SLUG --force` works —
	// the flag package stops at the first positional argument.
	if err := fs.Parse(rest[1:]); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q after SLUG", atlas.SafeText(fs.Arg(0)))
	}

	res, err := atlas.Install(context.Background(), slug, atlas.Options{
		IndexURL: *indexURL,
		Target:   *target,
		Replace:  *force,
	})
	if err != nil {
		// The only error the CLI reshapes: the library refuses a non-empty
		// target, the CLI knows the flag that overrides it.
		if errors.Is(err, atlas.ErrTargetNotEmpty) {
			return fmt.Errorf("%w — pass --force to replace it", err)
		}
		return err
	}

	for _, rel := range res.Files {
		fmt.Fprintf(out, "  wrote  %s\n", filepath.Join(res.Target, filepath.FromSlash(rel)))
	}
	if res.Replaced {
		fmt.Fprintf(out, "  replaced the previous contents of %s\n", res.Target)
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}

	files := fmt.Sprintf("%d files", len(res.Files))
	if len(res.Files) == 1 {
		files = "1 file"
	}
	fmt.Fprintf(out, "\nInstalled %s: %s → %s. Next:\n  sightmap validate\n", res.Label(), files, res.Target)
	return nil
}
