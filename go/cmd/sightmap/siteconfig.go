package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SiteConfig holds optional per-site defaults loaded from .sightmap/config.yaml.
// All fields are optional; missing fields leave the corresponding CLI flag at its
// built-in default.
type SiteConfig struct {
	Name     string `yaml:"name"` // short site name; used for Chrome profile directory
	Snapshot struct {
		Wait          float64 `yaml:"wait"`           // default for --wait
		Visible       bool    `yaml:"visible"`        // DEPRECATED: visible is now the default; use include_hidden: true to opt out
		IncludeHidden bool    `yaml:"include_hidden"` // if true, include hidden nodes (overrides the visible-by-default behaviour)
		Trace         bool    `yaml:"trace"`          // default for --trace
	} `yaml:"snapshot"`
	Coverage struct {
		Trace bool `yaml:"trace"` // default for --trace
	} `yaml:"coverage"`
	Browser struct {
		Port int `yaml:"port"` // default for --port (browser start)
	} `yaml:"browser"`
}

// loadSiteConfig reads .sightmap/config.yaml from sightmapDir.
// It returns a zero-value SiteConfig if the file is absent or cannot be parsed,
// logging a warning to stderr only on a parse error (missing file is silent).
// VisibleOnly returns true when the site config says to filter to visible nodes only
// (the default since go-i8c3, overridden by include_hidden: true).
func (c SiteConfig) VisibleOnly() bool {
	return !c.Snapshot.IncludeHidden
}

func loadSiteConfig(sightmapDir string) SiteConfig {
	path := filepath.Join(sightmapDir, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return SiteConfig{} // absent — silent
	}
	var cfg SiteConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "siteconfig: warning: %s: %v\n", path, err)
		return SiteConfig{}
	}
	if cfg.Snapshot.Visible {
		fmt.Fprintf(os.Stderr, "siteconfig: %s: 'snapshot.visible: true' is now the default; remove this line (use 'snapshot.include_hidden: true' to include hidden nodes)\n", path)
	}
	return cfg
}
