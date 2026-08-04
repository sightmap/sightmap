package sightmap

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/sightmap/sightmap/go/match"
)

// Corpus is the parsed, $ref-expanded, hierarchy-flattened sightmap data. It is
// pure, serializable data: load one with Load (or a Loader) and read it with the
// ComponentsForURL / ViewForURL / RequestsForURL queries. To match a live
// component tree against it, build a Matcher with NewMatcher(corpus).
type Corpus struct {
	// Memory holds file-level memory entries from every corpus file, in
	// file-path order. The spec activates these per source file ("applies
	// whenever any definition from that file is active"); we don't track
	// which flattened component or view came from which file, so entries
	// are concatenated corpus-wide instead of scoped to their file's
	// definitions. Tighten this if per-file activation is ever needed.
	Memory []string `json:"memory,omitempty"`

	// GlobalComponents is the flat list of globally-defined components
	// (from components.yaml), with children already flattened.
	GlobalComponents []match.ComponentDef `json:"globals,omitempty"`

	// Views contains per-route component lists, with $refs expanded and
	// children flattened.
	Views []View `json:"views,omitempty"`

	// Requests is the flat list of globally-defined API request definitions
	// (from a file-root `requests:` list). View-scoped requests live on each
	// View instead. Match against an observed network request via
	// RequestsForURL.
	Requests []RequestDef `json:"requests,omitempty"`

	// Messages is the flat list of console-output and exception patterns (from a
	// file-root `messages:` list). There is no view-scoped form.
	Messages []MessageDef `json:"messages,omitempty"`

	// Signals is the flat list of classification rules (from a file-root
	// `signals:` list), each referencing another entity by name. There is no
	// view-scoped form.
	Signals []SignalDef `json:"signals,omitempty"`

	// loadDiagnostics holds structural problems detected while loading that are
	// no longer visible in the flattened data (e.g. a circular $ref chain, which
	// is expanded away). Validate surfaces these alongside its own checks.
	loadDiagnostics []ValidationError
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
	Name       string               `json:"name"`
	Route      string               `json:"route,omitempty"`
	Memory     []string             `json:"memory,omitempty"`
	Components []match.ComponentDef `json:"components,omitempty"`
	Requests   []RequestDef         `json:"requests,omitempty"` // view-scoped API request definitions

	// Authoring/tooling fields — kept out of the serialized wire form.
	Stability  string     `json:"-"` // "" (default/active), "stub", or "deferred"
	Access     Access     `json:"-"` // reachability for the reference capture account
	URL        string     `json:"-"` // Representative URL for this view
	Snapshots  []Snapshot `json:"-"` // List of snapshots for this view
	SourceFile string     `json:"-"` // Source YAML filename (without .yaml extension)
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
func (c *Corpus) ComponentsForURL(pageURL string) []match.ComponentDef {
	v := c.ViewForURL(pageURL)
	if v == nil {
		return c.GlobalComponents
	}

	// view components first (they win on name collision), then non-colliding globals
	viewNames := make(map[string]bool, len(v.Components))
	for _, vc := range v.Components {
		viewNames[vc.Name] = true
	}

	result := make([]match.ComponentDef, 0, len(v.Components)+len(c.GlobalComponents))
	result = append(result, v.Components...)
	for _, gc := range c.GlobalComponents {
		if !viewNames[gc.Name] {
			result = append(result, gc)
		}
	}
	return result
}

// AllComponents returns every component definition in the corpus — GlobalComponents plus
// every View's Components — deduped by first-seen name. View lists include $ref-expanded
// globals, so a global reused in a view would otherwise appear twice; the first occurrence
// (global list first, then views in corpus order) wins. Route is not considered — this is
// for a whole-corpus consumer (a linter, a coverage report, an upload payload builder), not
// a per-page match; use ComponentsForURL for a single page's applicable list.
func (c *Corpus) AllComponents() []match.ComponentDef {
	seen := make(map[string]bool)
	var out []match.ComponentDef
	add := func(comp match.ComponentDef) {
		if comp.Name == "" || seen[comp.Name] {
			return
		}
		seen[comp.Name] = true
		out = append(out, comp)
	}
	for _, gc := range c.GlobalComponents {
		add(gc)
	}
	for _, v := range c.Views {
		for _, vc := range v.Components {
			add(vc)
		}
	}
	return out
}

// ViewForURL returns the View whose route most specifically matches pageURL's
// path, or nil if no view matches. Specificity follows the spec's per-segment
// scoring (literal > :param > * > **); when scores tie, the first-declared view
// wins. The returned View is a copy.
func (c *Corpus) ViewForURL(pageURL string) *View {
	u, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}
	path := normalizeRoutePath(u.Path)

	best := -1
	bestScore := -1
	for i := range c.Views {
		if !MatchRoute(c.Views[i].Route, path) {
			continue
		}
		if s := routeSpecificity(c.Views[i].Route); s > bestScore {
			bestScore = s
			best = i
		}
	}
	if best < 0 {
		return nil
	}
	v := c.Views[best] // copy so caller doesn't share Corpus internals
	return &v
}

// TiedViews returns the names of the views that tie for the most specific match
// of pageURL — i.e. share the top specificity score. A result of length >= 2
// means the winning view is decided only by declaration order (an ambiguity that
// depends on the exact URL, so it can't be caught statically). Length 0 or 1
// means no conflict. Names are in declaration order.
func (c *Corpus) TiedViews(pageURL string) []string {
	u, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}
	path := normalizeRoutePath(u.Path)
	topScore := -1
	var names []string
	for i := range c.Views {
		if !MatchRoute(c.Views[i].Route, path) {
			continue
		}
		s := routeSpecificity(c.Views[i].Route)
		switch {
		case s > topScore:
			topScore = s
			names = []string{c.Views[i].Name}
		case s == topScore:
			names = append(names, c.Views[i].Name)
		}
	}
	if len(names) < 2 {
		return nil
	}
	return names
}

// MatchRoute reports whether the glob-style route pattern matches path.
// trailing slash is normalized off both sides first (except the root "/"), so
// "/*/projects" matches "/acme/projects/".
//   - literal  matches itself
//   - *        matches exactly one path segment (no slashes)
//   - :param   matches exactly one path segment (like *, but scores higher)
//   - **       a whole "**" segment matches zero or more path segments, so
//     "/admin/**" matches "/admin", "/admin/x", and "/admin/x/y"
//   - a "**" glued into a segment (e.g. "/foo**") is treated as a regular *,
//     matching within that one segment only
func MatchRoute(pattern, path string) bool {
	re := regexp.MustCompile(routeToRegex(normalizeRoutePath(pattern)))
	return re.MatchString(normalizeRoutePath(path))
}

// normalizeRoutePath strips a trailing slash from a URL path or route pattern so
// that matching is insensitive to it. The root "/" (or an all-slash/empty path)
// normalizes to "/".
func normalizeRoutePath(p string) string {
	trimmed := strings.TrimRight(p, "/")
	if trimmed == "" {
		return "/"
	}
	return trimmed
}

// routeToRegex converts a normalized route glob pattern into an anchored regexp
// string. It is segment-oriented: the pattern is split on "/", and each segment
// becomes a regex fragment. A whole-segment "**" is a globstar that matches zero
// or more path segments (consuming the slash that would precede them, so
// "/a/**/b" matches "/a/b"); "*" and ":param" each match exactly one segment; any
// other segment is a literal in which a run of "*" (including a glued "**") is an
// in-segment wildcard that never crosses a slash.
func routeToRegex(pattern string) string {
	if pattern == "/" {
		return "^/$"
	}
	var sb strings.Builder
	sb.WriteByte('^')
	for i, seg := range strings.Split(pattern, "/") {
		if i == 0 && seg == "" {
			continue // leading slash
		}
		if seg == "**" {
			// Globstar: zero or more segments, including the leading slash. An
			// empty match collapses with the next segment's slash.
			sb.WriteString("(?:/.*)?")
			continue
		}
		sb.WriteByte('/')
		sb.WriteString(routeSegBody(seg))
	}
	sb.WriteByte('$')
	return sb.String()
}

// routeSegBody converts a single non-globstar route segment (without its leading
// slash) to a regex fragment. A bare "*" or ":param" is one whole segment; inside
// any other segment a run of "*" (including a glued "**") is an in-segment
// wildcard that never crosses a "/".
func routeSegBody(seg string) string {
	if seg == "*" || strings.HasPrefix(seg, ":") {
		return "[^/]+" // exactly one segment
	}
	var sb strings.Builder
	for i := 0; i < len(seg); {
		if seg[i] == '*' {
			for i < len(seg) && seg[i] == '*' { // collapse a run of stars (incl. glued **)
				i++
			}
			sb.WriteString("[^/]*") // in-segment wildcard
			continue
		}
		switch seg[i] {
		case '.', '+', '?', '(', ')', '[', ']', '{', '}', '\\', '|', '^', '$':
			sb.WriteByte('\\')
		}
		sb.WriteByte(seg[i])
		i++
	}
	return sb.String()
}

// routeSpecificity scores a route pattern per the spec's most-specific-wins
// table: literal segments score 3, :param 2, `*` 1, and `**` or empty 0. The
// score is summed over segments; the root route "/" scores 1.
func routeSpecificity(pattern string) int {
	pattern = normalizeRoutePath(pattern)
	if pattern == "/" {
		return 1
	}
	score := 0
	for _, seg := range strings.Split(pattern, "/") {
		switch {
		case seg == "" || seg == "**":
			// 0
		case seg == "*":
			score++
		case strings.HasPrefix(seg, ":"):
			score += 2
		default:
			score += 3
		}
	}
	return score
}
