package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// runInit scaffolds a schema-correct .sightmap/ corpus so the first files an
// author sees are already valid (version:, the views: wrapper, correct nesting)
// rather than written from memory. Existing files are never overwritten.
func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	sightmapDir := fs.String("sightmap-dir", ".sightmap", "Path to the .sightmap/ directory to create")
	if err := fs.Parse(args); err != nil {
		return err
	}

	files := []struct{ path, content string }{
		{"components.yaml", initComponentsYAML},
		{filepath.Join("views", "example.yaml"), initViewYAML},
	}

	if err := os.MkdirAll(*sightmapDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", *sightmapDir, err)
	}

	var created, skipped []string
	for _, f := range files {
		abs := filepath.Join(*sightmapDir, f.path)
		if _, err := os.Stat(abs); err == nil {
			skipped = append(skipped, f.path)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return fmt.Errorf("create dir for %s: %w", f.path, err)
		}
		if err := os.WriteFile(abs, []byte(f.content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", f.path, err)
		}
		created = append(created, f.path)
	}

	for _, p := range created {
		fmt.Printf("  created  %s\n", filepath.Join(*sightmapDir, p))
	}
	for _, p := range skipped {
		fmt.Printf("  skipped  %s (already exists)\n", filepath.Join(*sightmapDir, p))
	}
	if len(created) == 0 {
		fmt.Printf("%s is already scaffolded — nothing to do.\n", *sightmapDir)
		return nil
	}
	fmt.Printf("\nScaffolded %s. Next:\n"+
		"  1. edit %s for your app (route:, url:, component selectors)\n"+
		"  2. sightmap validate\n"+
		"  3. sightmap browser start  &&  sightmap snapshot --coverage\n",
		*sightmapDir, filepath.Join(*sightmapDir, "views", "example.yaml"))
	return nil
}

const initComponentsYAML = `# .sightmap/components.yaml — global components, matched on every view.
# Put shared UI (nav, footer, …) here so one definition improves every page.
# Full schema: https://sightmap.org/docs
#
# components:
#   - name: SiteNav
#     selector: 'nav[data-component="SiteNav"]'
version: 1
`

const initViewYAML = `# A view is one screen of your app, identified by its URL route. Copy this file
# per screen (e.g. views/home.yaml) and edit the fields below.
# Full schema: https://sightmap.org/docs
version: 1
views:
  - name: Home
    route: "/"                       # glob matched against the URL path (** = any depth)
    url: "http://localhost:3000/"    # a representative URL, used by report/capture
    components:
      - name: ExampleButton
        selector: '[data-testid="example"]'   # ← replace with a real selector
`
