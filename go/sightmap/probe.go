package sightmap

import "fmt"

// ProbeTarget is one (view, snapshot) page to navigate for whole-corpus
// operations such as `capture --all` and `sel-probe --all`. It resolves to a
// snapshot file at .sightmap/snapshots/{ViewDir}/{SnapName}.snap.
type ProbeTarget struct {
	ViewName string // the view's display name
	ViewDir  string // snapshots/ subdirectory (the view's SnapBasename)
	SnapName string // snapshot name / file basename
	URL      string // page to navigate to
}

// ProbeTargets builds the list of pages to navigate for whole-corpus commands:
// one target per view snapshots[] entry (using the snapshot's url:, falling back
// to the view's url:), or a single "base" target from the view url: when a view
// lists no snapshots. Views (and snapshots) with no resolvable URL are skipped.
// It errors only when the corpus yields no targets at all.
func (c *Corpus) ProbeTargets() ([]ProbeTarget, error) {
	var targets []ProbeTarget
	for i := range c.Views {
		v := &c.Views[i]
		viewDir := v.SnapBasename()
		if len(v.Snapshots) == 0 {
			if v.URL == "" {
				continue
			}
			targets = append(targets, ProbeTarget{ViewName: v.Name, ViewDir: viewDir, SnapName: "base", URL: v.URL})
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
			targets = append(targets, ProbeTarget{ViewName: v.Name, ViewDir: viewDir, SnapName: name, URL: url})
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no url: in any view — add url:/snapshots[].url to views/*.yaml")
	}
	return targets, nil
}
