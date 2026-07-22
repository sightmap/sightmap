package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sightmap/sightmap/go/sightmap"
	"gopkg.in/yaml.v3"
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
// and `sel-probe --all` from the corpus itself (the canonical source since
// go-k1e5): one target per view snapshots[] entry (using the snapshot's url:, or
// the view's url: as fallback), or a single "base" target from the view url:
// when a view lists no snapshots. probe-urls.yaml is consulted ONLY as a
// deprecated fallback when no view defines any URL.
func corpusProbeTargets(sightmapDir, probeURLsPath string) ([]probeTarget, error) {
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
	if len(targets) > 0 {
		return targets, nil
	}

	// Deprecated fallback: no view defines a url:. Read probe-urls.yaml if present.
	if probeURLsPath == "" {
		probeURLsPath = defaultProbeURLsPath(sightmapDir)
	}
	probes, perr := loadProbeURLs(probeURLsPath)
	if perr != nil {
		return nil, fmt.Errorf("no url: in any view (add url:/snapshots[].url to views/*.yaml); "+
			"probe-urls.yaml fallback also unavailable: %w", perr)
	}
	fmt.Fprintln(os.Stderr, "warning: no url: in views/*.yaml — using DEPRECATED probe-urls.yaml. "+
		"Move these URLs into views/*.yaml (url: and snapshots[].url).")
	for _, p := range probes {
		if p.URL == "" {
			continue
		}
		targets = append(targets, probeTarget{ViewName: p.Name, ViewDir: p.snapBasename(), SnapName: "base", URL: p.URL})
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

type probeURLsFile struct {
	Version int        `yaml:"version"`
	Probes  []probeURL `yaml:"probes"`
	Views   []probeURL `yaml:"views"` // legacy alias, deprecated
}

type probeURL struct {
	Name  string `yaml:"name"`
	URL   string `yaml:"url"`
	File  string `yaml:"file,omitempty"` // optional output basename (overrides sanitised Name)
	Notes string `yaml:"notes"`
}

// snapBasename returns the output file basename for this view.
// Uses File if set, otherwise lowercases and sanitises the Name.
func (p probeURL) snapBasename() string {
	if p.File != "" {
		return p.File
	}
	// Sanitise: lowercase, replace spaces with hyphens, drop non-alphanumeric-or-hyphen.
	s := strings.ToLower(p.Name)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else if r == ' ' || r == '_' {
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return p.Name
	}
	return b.String()
}

// loadProbeURLs reads and parses a probe-urls.yaml file.
// Returns a clear error if the file is missing or malformed.
func loadProbeURLs(path string) ([]probeURL, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("probe-urls.yaml not found at %s\n"+
				"Create .sightmap/probe-urls.yaml with a list of view URLs to enable --all mode.", path)
		}
		return nil, fmt.Errorf("read probe-urls.yaml: %w", err)
	}
	var f probeURLsFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse probe-urls.yaml: %w", err)
	}
	// Prefer probes: key; fall back to legacy views: key.
	if len(f.Probes) > 0 {
		return f.Probes, nil
	}
	return f.Views, nil
}

// defaultProbeURLsPath returns the default path for probe-urls.yaml
// relative to the sightmap dir.
func defaultProbeURLsPath(sightmapDir string) string {
	return filepath.Join(sightmapDir, "probe-urls.yaml")
}
