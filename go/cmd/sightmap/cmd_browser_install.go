package main

import (
	"context"
	"fmt"
	"os"

	"github.com/sightmap/sightmap/go/browser"
)

// runBrowserInstall implements `sightmap browser install`.
//
// Downloads and installs the latest stable Chrome for Testing into
// ~/.sightmap/browsers/. Idempotent: if the current stable version is already
// present it just prints the binary path and exits 0.
func runBrowserInstall(_ []string) error {
	binPath, err := browser.InstallCfT(context.Background(), os.Stderr)
	if err != nil {
		return err
	}
	fmt.Println(binPath)
	return nil
}
