package sightmap

import (
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/sightmap/sightmap/go/match"
)

// Corpus is the parsed, $ref-expanded, hierarchy-flattened sightmap data, plus a
// lazily-populated cache of compiled match queries. It is the single operational
// handle callers hold: load one with Load (or a Loader), then MatchTree a live
// component tree against it. Safe for concurrent use.
type Corpus struct {
	// GlobalComponents is the flat list of globally-defined components
	// (from components.yaml), with children already flattened.
	GlobalComponents []match.SightmapComponent

	// Views contains per-route component lists, with $refs expanded and
	// children flattened.
	Views []View

	// mu guards the lazily-built compiled-query cache.
	mu    sync.Mutex
	cache map[string]*queryCacheEntry
}

// Access describes the reachability of a view for the reference capture account.
// Status "" or "open" means reachable; "blocked" means gated by a
// flag/permission/plan the account lacks; "needs-data" means ungated but requires
// a content instance to exist.
type Access struct {
	Status string // "", "open", "blocked", or "needs-data"
	Reason string // why it's gated, e.g. "requires admin role" or "enterprise-plan only"
}

// IsOpen reports whether the view is reachable by the reference capture account.
func (a Access) IsOpen() bool {
	return a.Status == "" || a.Status == "open"
}

// View is a per-URL-pattern view definition from the sightmap corpus.
type View struct {
	Name       string
	Route      string
	Memory     []string
	Components []match.SightmapComponent
	Stability  string     // "" (default/active), "stub", or "deferred"
	Access     Access     // reachability for the reference capture account
	URL        string     // Representative URL for this view
	Snapshots  []Snapshot // List of snapshots for this view
	SourceFile string     // Source YAML filename (without .yaml extension)
}

// ViewByName returns a pointer to the first View with the given name, or nil.
func (c *Corpus) ViewByName(name string) *View {
	for i := range c.Views {
		if c.Views[i].Name == name {
			return &c.Views[i]
		}
	}
	return nil
}

// Snapshot represents a named snapshot of a view with optional reproduction notes.
type Snapshot struct {
	Name  string // Snapshot name (e.g., "base", "with-modal-open")
	Notes string // Optional notes on how to reproduce this snapshot state
	URL   string // Optional representative URL for this snapshot; falls back to the view's URL
}

// SnapBasename returns the output file basename for this view.
// Uses SourceFile if available (preferred), otherwise lowercases and sanitises the Name.
func (v *View) SnapBasename() string {
	if v.SourceFile != "" {
		return v.SourceFile
	}
	// Fallback: Sanitise name: lowercase, replace spaces with hyphens, keep only alphanumeric and hyphens.
	s := strings.ToLower(v.Name)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else if r == ' ' || r == '_' {
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return v.Name
	}
	return b.String()
}

// ComponentsForURL returns the merged flat component list for a given URL:
// view components first, then global components that don't collide on name.
// Returns GlobalComponents only if no view matches.
func (c *Corpus) ComponentsForURL(pageURL string) []match.SightmapComponent {
	v := c.ViewForURL(pageURL)
	if v == nil {
		return c.GlobalComponents
	}

	// view components first (they win on name collision), then non-colliding globals
	viewNames := make(map[string]bool, len(v.Components))
	for _, vc := range v.Components {
		viewNames[vc.Name] = true
	}

	result := make([]match.SightmapComponent, 0, len(v.Components)+len(c.GlobalComponents))
	result = append(result, v.Components...)
	for _, gc := range c.GlobalComponents {
		if !viewNames[gc.Name] {
			result = append(result, gc)
		}
	}
	return result
}

// ViewForURL returns the first View whose route matches pageURL's path,
// or nil if no view matches. The returned View is a copy.
func (c *Corpus) ViewForURL(pageURL string) *View {
	u, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}
	path := u.Path
	if path == "" {
		path = "/"
	}
	for i := range c.Views {
		if MatchRoute(c.Views[i].Route, path) {
			v := c.Views[i] // copy so caller doesn't share Corpus internals
			return &v
		}
	}
	return nil
}

// MatchRoute reports whether the glob-style route pattern matches path.
// ** matches any sequence of characters including slashes (one or more).
// *  matches a single path segment (no slashes).
func MatchRoute(pattern, path string) bool {
	re := regexp.MustCompile(routeToRegex(pattern))
	return re.MatchString(path)
}

// routeToRegex converts a route glob pattern to an anchored regexp string.
func routeToRegex(pattern string) string {
	var sb strings.Builder
	sb.WriteByte('^')
	for i := 0; i < len(pattern); {
		if pattern[i] == '*' {
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				sb.WriteString("(.+)")
				i += 2
				continue
			}
			sb.WriteString("([^/]+)")
			i++
			continue
		}
		c := pattern[i]
		// Escape regex metacharacters that can appear in URL patterns.
		switch c {
		case '.', '+', '?', '(', ')', '[', ']', '{', '}', '\\', '|', '^', '$':
			sb.WriteByte('\\')
		}
		sb.WriteByte(c)
		i++
	}
	sb.WriteByte('$')
	return sb.String()
}
