package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sightmap/sightmap/go/atlas"
)

// runAtlasAdd installs one published corpus.
func runAtlasAdd(args []string) error {
	return runAtlasAddOut(args, os.Stdout)
}

// runAtlasAddOut is the testable core: it writes all output to out.
func runAtlasAddOut(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("atlas add", flag.ContinueOnError)
	target := fs.String("target", ".sightmap", "Directory to install the corpus into")
	source := fs.String("source", atlas.DefaultArchiveURL, "Archive URL template, with {slug} substituted (for mirrors and private corpora)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: sightmap atlas add SLUG [--target DIR] [--source URL]\n\n")
		fmt.Fprintf(os.Stderr, "Installs a corpus published in the community atlas\n")
		fmt.Fprintf(os.Stderr, "(%s) into TARGET (default ./.sightmap).\n\n", atlas.AtlasURL)
		fmt.Fprintf(os.Stderr, "Have a URL rather than a slug? Run: sightmap atlas find <domain>\n\n")
		fmt.Fprintf(os.Stderr, "Install from a private corpus store instead:\n")
		fmt.Fprintf(os.Stderr, "  sightmap atlas add toast-pos --source https://internal.corp/{slug}.tar.gz\n\n")
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
	// Re-parse what follows the slug so `atlas add SLUG --target DIR` works —
	// the flag package stops at the first positional argument.
	if err := fs.Parse(rest[1:]); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q after SLUG", atlas.SafeText(fs.Arg(0)))
	}

	res, err := atlas.Install(context.Background(), slug, atlas.Options{
		ArchiveURL: *source,
		Target:     *target,
	})
	if err != nil {
		return err
	}

	for _, rel := range res.Files {
		fmt.Fprintf(out, "  wrote  %s\n", filepath.Join(res.Target, filepath.FromSlash(rel)))
	}
	fmt.Fprintf(out, "\nInstalled %s: %s → %s. Next:\n  sightmap validate\n",
		atlas.SafeText(res.Slug), plural(len(res.Files), "file", "files"), res.Target)
	return nil
}
