// Wires --grow into browser explore: the grow package writes components into
// the corpus as pages are visited, and the driver picks up the reloaded corpus
// after every successful write.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sightmap/sightmap/go/explore"
	"github.com/sightmap/sightmap/go/grow"
	"github.com/sightmap/sightmap/go/sightmap"
)

func init() {
	newGrowHook = func(dir string, drv *explore.CDPDriver) (explore.PageHook, func() string, error) {
		if _, err := os.Stat(dir); err != nil {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, nil, fmt.Errorf("--grow: create %s: %w", dir, err)
			}
			if err := os.WriteFile(filepath.Join(dir, "components.yaml"), []byte("version: 1\ncomponents: []\n"), 0o644); err != nil {
				return nil, nil, err
			}
		}
		jev, err := explore.NewJevPicker("")
		if err != nil {
			return nil, nil, fmt.Errorf("--grow: %w", err)
		}
		g := grow.New(dir, jev)
		g.Reload = func() error {
			c, err := sightmap.Load(dir)
			if err != nil {
				return err
			}
			if errs := sightmap.Validate(c); len(errs) > 0 {
				return fmt.Errorf("%d validation errors (first: %v)", len(errs), errs[0])
			}
			drv.Corpus = c
			return nil
		}
		g.Log = func(msg string) { fmt.Fprintln(os.Stderr, msg) }
		report := func() string {
			st := g.Stats()
			return fmt.Sprintf("grow: %d components, %d views, %d promoted to global, %d pages, %d model calls, %d ms", st.Added, st.Views, st.Promoted, st.Pages, st.Calls, st.Ms)
		}
		return g, report, nil
	}
}
