package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// findSnapshots returns a list of snapshot files to process.
// If args are provided, use those paths.
// Otherwise, search for snapshots in both legacy (*.snap) and new (.sightmap/snapshots/**/*.snap) locations.
func findSnapshots(sightmapDir string, args []string) ([]string, error) {
	if len(args) > 0 {
		return args, nil
	}

	var snapFiles []string

	// Search new organized location: .sightmap/snapshots/**/*.snap
	snapshotsDir := filepath.Join(sightmapDir, "snapshots")
	if info, err := os.Stat(snapshotsDir); err == nil && info.IsDir() {
		err := filepath.Walk(snapshotsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(path, ".snap") {
				snapFiles = append(snapFiles, path)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk snapshots directory: %v", err)
		}
	}

	// Also search legacy location: *.snap (for backward compatibility during migration)
	legacySnaps, globErr := filepath.Glob("*.snap")
	if globErr != nil {
		return nil, fmt.Errorf("glob *.snap: %v", globErr)
	}
	snapFiles = append(snapFiles, legacySnaps...)

	if len(snapFiles) == 0 {
		return nil, fmt.Errorf("no .snap files found in %s/snapshots/ or current directory", sightmapDir)
	}

	return snapFiles, nil
}
