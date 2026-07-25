package main

import (
	"fmt"
	"strings"

	"github.com/sightmap/sightmap/go/sightmap"
)

// probeTarget is one (view, snapshot) page to navigate for --all-style commands.
// It maps to a snapshot file at .sightmap/snapshots/{ViewDir}/{SnapName}.snap.
type probeTarget struct {
	ViewName string // display name (the view's name)
	ViewDir  string // snapshots/ subdirectory (the view's source-file basename)
	SnapName string // snapshot name / file basename
	URL      string // page to navigate to
}

// corpusProbeTargets builds the list of pages to navigate for `snapshot --all`
// and `sel-probe --all` from the corpus: one target per view snapshots[] entry
// (using the snapshot's url:, or the view's url: as fallback), or a single
// "base" target from the view url: when a view lists no snapshots.
func corpusProbeTargets(sightmapDir string) ([]probeTarget, error) {
	corpus, err := sightmap.DirLoader(sightmapDir).Load()
	if err != nil {
		return nil, fmt.Errorf("load corpus: %w", err)
	}

	var targets []probeTarget
	for _, v := range corpus.Views {
		viewDir := v.SourceFile
		if viewDir == "" {
			viewDir = sanitizeBasename(v.Name)
		}
		if len(v.Snapshots) == 0 {
			if v.URL == "" {
				continue
			}
			targets = append(targets, probeTarget{ViewName: v.Name, ViewDir: viewDir, SnapName: "base", URL: v.URL})
			continue
		}
		for _, s := range v.Snapshots {
			url := s.URL
			if url == "" {
				url = v.URL
			}
			if url == "" {
				continue
			}
			name := s.Name
			if name == "" {
				name = "base"
			}
			targets = append(targets, probeTarget{ViewName: v.Name, ViewDir: viewDir, SnapName: name, URL: url})
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no url: in any view — add url:/snapshots[].url to views/*.yaml")
	}
	return targets, nil
}

// sanitizeBasename lowercases and sanitises a view name for use as a directory.
func sanitizeBasename(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-':
			b.WriteRune(r)
		case r == ' ' || r == '_':
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return name
	}
	return b.String()
}
