package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/sightmap/sightmap/go/browser"
)

// runBrowserInstall implements `sightmap browser install`.
//
// Downloads and installs the latest Chrome for Testing into ~/.sightmap/browsers/.
// Prefers the Stable channel, falling back to Beta/Dev/Canary only for platforms
// Stable does not carry (currently just linux-arm64). Idempotent: if the resolved
// version is already present it just prints the binary path and exits 0.
func runBrowserInstall(args []string) error {
	// Parse args so `--help` (or any unknown flag) is handled before the
	// download starts, rather than being ignored while the 184 MB install runs.
	fs := flag.NewFlagSet("browser install", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: sightmap browser install")
		fmt.Fprintln(os.Stderr, "  Download the managed Chrome for Testing into ~/.sightmap/browsers/ (idempotent).")
	}
	if err := fs.Parse(args); err != nil {
		return err // flag.ErrHelp is treated as success (exit 0) by main
	}

	binPath, err := browser.InstallCfT(context.Background(), os.Stderr)
	if err != nil {
		return err
	}
	fmt.Println(binPath)
	return nil
}
