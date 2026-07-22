// migrate-visible-config removes the deprecated 'snapshot.visible: true' line
// from .sightmap/config.yaml (visible is now the default since go-i8c3).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func runMigrateVisibleConfig(args []string) error {
	fs := flag.NewFlagSet("migrate-visible-config", flag.ContinueOnError)
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir")
	dryRunFlag := fs.Bool("dry-run", false, "Show what would change without modifying files")
	if err := fs.Parse(args); err != nil {
		return err
	}

	configPath := filepath.Join(*sightmapDirFlag, "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "migrate-visible-config: no config.yaml found, nothing to migrate")
			return nil
		}
		return fmt.Errorf("read %s: %v", configPath, err)
	}

	lines := strings.Split(string(data), "\n")
	var kept []string
	removed := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "visible: true" || trimmed == "visible: false" {
			fmt.Fprintf(os.Stderr, "removing: %q\n", line)
			removed++
		} else {
			kept = append(kept, line)
		}
	}

	if removed == 0 {
		fmt.Fprintln(os.Stderr, "migrate-visible-config: nothing to remove")
		return nil
	}

	if *dryRunFlag {
		fmt.Fprintf(os.Stderr, "migrate-visible-config: dry run — would remove %d line(s)\n", removed)
		return nil
	}

	if err := os.WriteFile(configPath, []byte(strings.Join(kept, "\n")), 0o644); err != nil {
		return fmt.Errorf("write %s: %v", configPath, err)
	}
	fmt.Fprintf(os.Stderr, "migrate-visible-config: removed %d line(s) from %s\n", removed, configPath)
	return nil
}
