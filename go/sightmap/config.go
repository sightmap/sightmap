package sightmap

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SiteConfig holds optional, TOOLING-ONLY per-site defaults loaded from
// .sightmap/config.yaml. It is NOT part of the sightmap spec (which covers only
// views, components, and requests) — but the file lives inside .sightmap/, so
// the sightmap package owns its parse. All fields are optional; a missing field
// leaves the corresponding CLI flag at its built-in default.
type SiteConfig struct {
	Name     string `yaml:"name"` // short site name; used for the Chrome profile directory
	Snapshot struct {
		Wait          float64 `yaml:"wait"`           // default for --wait
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

// VisibleOnly reports whether analysis should be restricted to visible nodes —
// the default, overridden by snapshot.include_hidden: true.
func (c SiteConfig) VisibleOnly() bool {
	return !c.Snapshot.IncludeHidden
}

// LoadConfig reads .sightmap/config.yaml from dir. It returns a zero-value
// SiteConfig when the file is absent (silently) or unparseable (logging a
// warning to stderr), so callers can use the result unconditionally.
func LoadConfig(dir string) SiteConfig {
	path := filepath.Join(dir, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return SiteConfig{} // absent — silent
	}
	var cfg SiteConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "sightmap: config %s: %v\n", path, err)
		return SiteConfig{}
	}
	return cfg
}
