// migrate-snapshots moves legacy .snap files from the site root directory
// into the organized .sightmap/snapshots/{view}/base.snap structure.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func runMigrateSnapshots(args []string) error {
	fs := flag.NewFlagSet("migrate-snapshots", flag.ContinueOnError)
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir")
	dryRunFlag := fs.Bool("dry-run", false, "Show what would be moved without actually moving files")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Find all legacy *.snap files in current directory
	legacySnaps, err := filepath.Glob("*.snap")
	if err != nil {
		return fmt.Errorf("glob *.snap: %v", err)
	}

	if len(legacySnaps) == 0 {
		fmt.Fprintln(os.Stderr, "migrate-snapshots: no legacy .snap files found in current directory")
		return nil
	}

	// Load probe-urls.yaml to map file basenames to view names
	probeURLsPath := defaultProbeURLsPath(*sightmapDirFlag)
	probes, err := loadProbeURLs(probeURLsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate-snapshots: warning: could not load probe-urls.yaml: %v\n", err)
		fmt.Fprintln(os.Stderr, "migrate-snapshots: will use file basename as view name")
	}

	// Build a map of file basename → view name
	fileToView := make(map[string]string)
	for _, p := range probes {
		basename := p.snapBasename()
		fileToView[basename] = basename // Use sanitized name as the directory name
	}

	moved := 0
	for _, snapFile := range legacySnaps {
		// Strip .snap extension to get basename
		basename := filepath.Base(snapFile)
		if len(basename) > 5 && basename[len(basename)-5:] == ".snap" {
			basename = basename[:len(basename)-5]
		}

		// Determine view directory name
		viewName := basename
		if mapped, ok := fileToView[basename]; ok {
			viewName = mapped
		}

		// Build new paths
		newSnapPath := snapshotPath(*sightmapDirFlag, viewName, "base")
		newTreePath := snapshotTreePath(newSnapPath)

		// Check if tree JSON exists
		oldTreePath := snapFile + ".tree.json"
		treeExists := false
		if _, err := os.Stat(oldTreePath); err == nil {
			treeExists = true
		}

		// Show what will happen
		fmt.Fprintf(os.Stderr, "%s → %s\n", snapFile, newSnapPath)
		if treeExists {
			fmt.Fprintf(os.Stderr, "%s → %s\n", oldTreePath, newTreePath)
		}

		if *dryRunFlag {
			continue
		}

		// Create target directory
		snapDir := filepath.Dir(newSnapPath)
		if err := os.MkdirAll(snapDir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %v", snapDir, err)
		}

		// Move .snap file
		if err := os.Rename(snapFile, newSnapPath); err != nil {
			return fmt.Errorf("move %s → %s: %v", snapFile, newSnapPath, err)
		}

		// Move .snap.tree.json if it exists
		if treeExists {
			if err := os.Rename(oldTreePath, newTreePath); err != nil {
				// Try to move the snap file back on error
				os.Rename(newSnapPath, snapFile)
				return fmt.Errorf("move %s → %s: %v", oldTreePath, newTreePath, err)
			}
		}

		moved++
	}

	if *dryRunFlag {
		fmt.Fprintf(os.Stderr, "\nmigrate-snapshots: dry run complete, %d files would be moved\n", len(legacySnaps))
	} else {
		fmt.Fprintf(os.Stderr, "\nmigrate-snapshots: successfully moved %d snapshot(s)\n", moved)
	}

	return nil
}
