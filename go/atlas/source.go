package atlas

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

// Source is an atlas repository's raw-content layout, derived from the URL of
// its index. It is the single place that knows where a file lives:
//
//	index:  <Root>/<Ref>/index.json
//	files:  <Root>/<Ref>/entries/<slug>/<path>
//
// One rule for every host — raw.githubusercontent.com, a GitHub Enterprise raw
// host, and a plain copy of what either serves all describe themselves this
// way. See the package doc for the full statement.
type Source struct {
	IndexURL string // the index URL this source was parsed from, for error messages
	Root     string // scheme://host[/prefix], no trailing slash
	Ref      string // the ref the index was read at: "main", "refs/heads/main", a sha
}

// ParseSource derives the raw-content layout from an index URL, enforcing the
// transport policy (HTTPS, or plain HTTP for loopback) while it is there.
func ParseSource(indexURL string) (Source, error) {
	u, err := url.Parse(indexURL)
	if err != nil {
		return Source{}, fmt.Errorf("invalid atlas index URL %q: %w", SafeText(indexURL), err)
	}
	if err := checkURL(u); err != nil {
		return Source{}, err
	}
	dir, file := path.Split(u.EscapedPath())
	if file == "" {
		return Source{}, fmt.Errorf("atlas index URL %s must point at the index file, not a directory", u)
	}
	segs := splitPath(dir)
	root, ref := splitRef(segs)
	if ref == "" {
		return Source{}, fmt.Errorf("atlas index URL %s has no ref segment — expected <root>/<ref>/%s (for example %s)", u, file, DefaultIndexURL)
	}
	base := u.Scheme + "://" + u.Host
	if len(root) > 0 {
		base += "/" + strings.Join(root, "/")
	}
	return Source{IndexURL: indexURL, Root: base, Ref: ref}, nil
}

// RefFor returns the ref an entry's files are fetched at: its pinning commit
// when it publishes one, otherwise the ref the index itself was read at. There
// is no hardcoded branch — an index served from a non-main ref installs that
// ref's content.
func (s Source) RefFor(e *Entry) string {
	if e.Commit != "" {
		return e.Commit
	}
	return s.Ref
}

// FileURL returns the raw URL of one corpus file of one entry, resolved at
// ref. Callers pass a validated slug and path (see [Entry.Validate]); the
// segments are percent-escaped on the way in regardless.
func (s Source) FileURL(ref, slug, corpusPath string) string {
	return s.Root + "/" + ref + "/entries/" + url.PathEscape(slug) + "/" + escapePathSegments(corpusPath)
}

// splitPath returns the non-empty segments of a URL path.
func splitPath(p string) []string {
	var segs []string
	for _, s := range strings.Split(p, "/") {
		if s != "" {
			segs = append(segs, s)
		}
	}
	return segs
}

// splitRef separates the ref from the root in the index URL's directory path.
// The ref is normally the last segment ("main", a sha). When a "refs" segment
// is present the ref is everything from it on, so GitHub's own
// .../atlas/refs/heads/main/index.json describes the same layout as
// .../atlas/main/index.json instead of yielding a root of .../atlas/refs/heads
// and 404ing every commit-pinned fetch.
func splitRef(segs []string) (root []string, ref string) {
	for i := len(segs) - 1; i >= 0; i-- {
		if segs[i] == "refs" && i+1 < len(segs) {
			return segs[:i], strings.Join(segs[i:], "/")
		}
	}
	if len(segs) == 0 {
		return nil, ""
	}
	return segs[: len(segs)-1 : len(segs)-1], segs[len(segs)-1]
}

// escapePathSegments percent-escapes each segment of a slash-separated path
// while keeping the separators.
func escapePathSegments(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

// checkURL enforces the transport policy: HTTPS everywhere, with a plain-HTTP
// exception for loopback hosts so tests and local mirrors work. It runs on the
// index URL and again on every redirect hop.
func checkURL(u *url.URL) error {
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" {
		switch u.Hostname() {
		case "localhost", "127.0.0.1", "::1":
			return nil
		}
	}
	return fmt.Errorf("refusing non-HTTPS URL %s (plain http is allowed only for localhost)", u)
}
