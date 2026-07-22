// migrate-probe-urls merges probe-urls.yaml content into views/*.yaml files
// and removes probe-urls.yaml.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type viewFileRaw struct {
	Version   int             `yaml:"version"`
	Views     []viewEntryRaw  `yaml:"views"`
	URL       string          `yaml:"url,omitempty"`
	Snapshots []snapshotEntry `yaml:"snapshots,omitempty"`
}

type viewEntryRaw struct {
	Name       string      `yaml:"name"`
	Route      string      `yaml:"route,omitempty"`
	Memory     []string    `yaml:"memory,omitempty"`
	Components []yaml.Node `yaml:"components,omitempty"`
	Stability  string      `yaml:"stability,omitempty"`
}

type snapshotEntry struct {
	Name  string `yaml:"name"`
	Notes string `yaml:"notes,omitempty"`
}

func runMigrateProbeURLs(args []string) error {
	fs := flag.NewFlagSet("migrate-probe-urls", flag.ContinueOnError)
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir")
	dryRunFlag := fs.Bool("dry-run", false, "Show what would change without modifying files")
	if err := fs.Parse(args); err != nil {
		return err
	}

	probeURLsPath := filepath.Join(*sightmapDirFlag, "probe-urls.yaml")

	// Check if probe-urls.yaml exists
	if _, err := os.Stat(probeURLsPath); os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "migrate-probe-urls: no probe-urls.yaml found, nothing to migrate")
		return nil
	}

	// Load probe-urls.yaml
	probes, err := loadProbeURLs(probeURLsPath)
	if err != nil {
		return fmt.Errorf("load probe-urls.yaml: %v", err)
	}

	// Build map of view name → probe info, and file basename → probe info
	probesByView := make(map[string]probeURL)
	probesByFile := make(map[string]probeURL)
	for _, p := range probes {
		probesByView[p.Name] = p
		basename := p.snapBasename()
		probesByFile[basename] = p
	}

	viewsDir := filepath.Join(*sightmapDirFlag, "views")
	entries, err := os.ReadDir(viewsDir)
	if err != nil {
		return fmt.Errorf("read views directory: %v", err)
	}

	modified := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		viewPath := filepath.Join(viewsDir, entry.Name())

		// Read existing view file
		data, err := os.ReadFile(viewPath)
		if err != nil {
			return fmt.Errorf("read %s: %v", viewPath, err)
		}

		var viewFile viewFileRaw
		if err := yaml.Unmarshal(data, &viewFile); err != nil {
			return fmt.Errorf("parse %s: %v", viewPath, err)
		}

		// Skip if already has URL field
		if viewFile.URL != "" {
			continue
		}

		// Find matching probe (by view name or by file basename)
		var matchedProbe *probeURL
		for i := range viewFile.Views {
			if probe, ok := probesByView[viewFile.Views[i].Name]; ok {
				matchedProbe = &probe
				break
			}
		}
		// Fall back to file basename match
		if matchedProbe == nil {
			fileBasename := strings.TrimSuffix(entry.Name(), ".yaml")
			if probe, ok := probesByFile[fileBasename]; ok {
				matchedProbe = &probe
			}
		}

		if matchedProbe == nil {
			continue
		}

		// Add URL field
		viewFile.URL = matchedProbe.URL

		// If probe has notes, convert to single base snapshot with notes
		if matchedProbe.Notes != "" {
			viewFile.Snapshots = []snapshotEntry{
				{Name: "base", Notes: matchedProbe.Notes},
			}
		}

		fmt.Fprintf(os.Stderr, "%-40s  +url: %s\n", entry.Name(), matchedProbe.URL)
		if matchedProbe.Notes != "" {
			fmt.Fprintf(os.Stderr, "%-40s  +snapshot notes\n", "")
		}

		if *dryRunFlag {
			modified++
			continue
		}

		// Write back
		out, err := yaml.Marshal(&viewFile)
		if err != nil {
			return fmt.Errorf("marshal %s: %v", viewPath, err)
		}

		if err := os.WriteFile(viewPath, out, 0o644); err != nil {
			return fmt.Errorf("write %s: %v", viewPath, err)
		}

		modified++
	}

	if modified == 0 {
		fmt.Fprintln(os.Stderr, "migrate-probe-urls: no views needed migration")
		return nil
	}

	if *dryRunFlag {
		fmt.Fprintf(os.Stderr, "\nmigrate-probe-urls: dry run complete, %d view(s) would be updated\n", modified)
		fmt.Fprintf(os.Stderr, "probe-urls.yaml would be deleted\n")
		return nil
	}

	// Remove probe-urls.yaml
	if err := os.Remove(probeURLsPath); err != nil {
		return fmt.Errorf("remove probe-urls.yaml: %v", err)
	}

	fmt.Fprintf(os.Stderr, "\nmigrate-probe-urls: successfully migrated %d view(s)\n", modified)
	fmt.Fprintf(os.Stderr, "probe-urls.yaml deleted\n")

	return nil
}
