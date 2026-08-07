// Package atlas finds and installs sightmap corpora published at
// sightmap.org/atlas.
//
// Searching and installing are independent. [LoadIndex] reads the catalog;
// [Install] fetches one archive from a URL template and never reads the index,
// so an install survives an index outage and the catalog can grow fields
// without a CLI release.
//
//	res, err := atlas.LoadIndex(ctx, atlas.IndexOptions{})
//	hits := res.Index.Search(atlas.Query{Text: "squareup.com"})
//	_, err = atlas.Install(ctx, hits[0].Entry.Slug, atlas.Options{Target: ".sightmap"})
//
// Everything the catalog publishes is untrusted. [ParseIndex] escapes control
// characters out of every display field as it decodes, so index text is safe to
// print from then on, and [ValidateSlug] gates the one field that reaches a URL
// and the filesystem.
package atlas

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// DefaultIndexURL is the published catalog. It is served from sightmap.org
// rather than the atlas git repository because the gallery rebuild is what
// enforces removed.yaml: a CLI reading the repository would keep finding
// entries the atlas has taken down. IndexOptions.URL overrides it.
const DefaultIndexURL = "https://sightmap.org/atlas/index.json"

// AtlasURL is the gallery a person browses. It is printed when a search finds
// nothing and when an install gets a 404.
const AtlasURL = "https://sightmap.org/atlas"

// SchemaVersion is the catalog schema this package understands. A higher
// version is refused before the rest of the document is decoded, so a
// restructured future catalog reports "upgrade sightmap" rather than a JSON
// error about a field that moved.
const SchemaVersion = 1

// Index is the subset of the catalog sightmap reads. Unknown fields are ignored
// so the atlas can publish more metadata without breaking shipped CLIs.
type Index struct {
	SchemaVersion int     `json:"schema_version"`
	Entries       []Entry `json:"entries"`
}

// Entry is one published corpus. Only Slug reaches a URL or the filesystem; the
// rest is display and search metadata, escaped by [ParseIndex].
type Entry struct {
	Slug         string   `json:"slug"`
	Name         string   `json:"name,omitempty"`
	Description  string   `json:"description,omitempty"`
	Domains      []string `json:"domains,omitempty"`
	Categories   []string `json:"categories,omitempty"`
	Stats        Stats    `json:"stats"`
	LastVerified string   `json:"last_verified,omitempty"`
}

// Stats is how much of a site an entry maps.
type Stats struct {
	Views      int `json:"views,omitempty"`
	Components int `json:"components,omitempty"`
	Requests   int `json:"requests,omitempty"`
}

// Counts renders the non-zero counts as display phrases: "12 views",
// "48 components". A count the catalog omits is left out rather than printed as
// zero, which would claim the corpus maps none.
func (s Stats) Counts() []string {
	var out []string
	for _, c := range []struct {
		n     int
		label string
	}{{s.Views, "views"}, {s.Components, "components"}, {s.Requests, "requests"}} {
		if c.n > 0 {
			out = append(out, fmt.Sprintf("%d %s", c.n, c.label))
		}
	}
	return out
}

// decodeIndex reads a catalog exactly as published, gating on schema_version
// before the document's shape is assumed. A v2 catalog that restructures its
// entries therefore reports "upgrade sightmap" rather than a JSON error about a
// field that moved.
func decodeIndex(data []byte) (*Index, error) {
	var gate struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &gate); err != nil {
		return nil, fmt.Errorf("parse atlas index: %w", err)
	}
	if gate.SchemaVersion > SchemaVersion {
		return nil, fmt.Errorf("atlas index has schema_version %d, but this sightmap understands %d — upgrade sightmap", gate.SchemaVersion, SchemaVersion)
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parse atlas index: %w", err)
	}
	return &idx, nil
}

// Validate reports every problem in a catalog as published: a schema version
// this sightmap cannot read, a slug that could never be installed, a duplicate
// slug, and any display field carrying a control character or invalid UTF-8.
//
// This is the publisher's check, and it runs against the raw bytes on purpose.
// [ParseIndex] escapes those fields as it decodes, so by the time anything else
// sees an entry the damage is already papered over: the publisher merging a
// community entry is the only one who can actually fix it.
func Validate(data []byte) []error {
	idx, err := decodeIndex(data)
	if err != nil {
		return []error{err}
	}
	var errs []error
	seen := make(map[string]bool, len(idx.Entries))
	for i := range idx.Entries {
		e := &idx.Entries[i]
		if err := ValidateSlug(e.Slug); err != nil {
			errs = append(errs, fmt.Errorf("entry %d: slug %q %w", i, SafeText(e.Slug), err))
			continue
		}
		if seen[e.Slug] {
			errs = append(errs, fmt.Errorf("entry %d: duplicate slug %q", i, e.Slug))
		}
		seen[e.Slug] = true
		// SafeText is the escaper, so a value it would change is a value that
		// needed escaping. The escaped form goes in the message unquoted: it is
		// already safe to print, and %q would escape the escapes.
		for _, v := range slices.Concat([]string{e.Name, e.Description, e.LastVerified}, e.Domains, e.Categories) {
			if safe := SafeText(v); safe != v {
				errs = append(errs, fmt.Errorf("entry %d (%s): control character or invalid UTF-8 in %s", i, e.Slug, safe))
			}
		}
	}
	return errs
}

// ParseIndex decodes a catalog for use, escaping control characters out of
// every display field so a caller can print index text without escaping it
// again. Use [Validate] to find the entries this quietly cleaned up.
func ParseIndex(data []byte) (*Index, error) {
	idx, err := decodeIndex(data)
	if err != nil {
		return nil, err
	}
	for i := range idx.Entries {
		e := &idx.Entries[i]
		e.Name = SafeText(e.Name)
		e.Description = SafeText(e.Description)
		e.LastVerified = SafeText(e.LastVerified)
		for j := range e.Domains {
			e.Domains[j] = SafeText(e.Domains[j])
		}
		for j := range e.Categories {
			e.Categories[j] = SafeText(e.Categories[j])
		}
	}
	return idx, nil
}

// Query is what a caller is looking for. An empty Text matches every entry,
// which is what `sightmap atlas list` runs.
type Query struct {
	Text     string
	Category string
}

// Hit is one search result. MatchedOn names the field the query matched, so a
// caller can show why an entry came back.
type Hit struct {
	Entry     Entry
	MatchedOn string
	rank      int
}

// Match ranks, best first. A caller usually has a URL, so an exact domain hit
// has to outrank an entry whose description happens to mention the same word.
const (
	rankDomainExact = iota
	rankSlugExact
	rankDomain
	rankSlug
	rankName
	rankCategory
	rankDescription
	rankNone
)

var rankNames = [...]string{"exact domain", "exact slug", "domain", "slug", "name", "category", "description"}

// Search returns the entries matching q, best first, ties broken by slug so the
// order is deterministic.
func (ix *Index) Search(q Query) []Hit {
	text := strings.ToLower(strings.TrimSpace(q.Text))
	domain := normalizeDomain(text)
	category := strings.ToLower(strings.TrimSpace(q.Category))

	var hits []Hit
	for _, e := range ix.Entries {
		// A hit carries an install command, so an entry that could never be
		// installed must never be offered as one.
		if ValidateSlug(e.Slug) != nil {
			continue
		}
		if category != "" && !anyField(e.Categories, category, contains) {
			continue
		}
		if r := rank(&e, text, domain); r != rankNone {
			hits = append(hits, Hit{Entry: e, MatchedOn: rankNames[r], rank: r})
		}
	}
	slices.SortStableFunc(hits, func(a, b Hit) int {
		return cmp.Or(cmp.Compare(a.rank, b.rank), strings.Compare(a.Entry.Slug, b.Entry.Slug))
	})
	return hits
}

// rank returns the best rank at which e matches text, or rankNone.
func rank(e *Entry, text, domain string) int {
	switch {
	case text == "":
		return rankDomainExact
	case domain != "" && anyField(e.Domains, domain, sameDomain):
		return rankDomainExact
	case strings.EqualFold(e.Slug, text):
		return rankSlugExact
	case anyField(e.Domains, text, pasted):
		return rankDomain
	case pasted(e.Slug, text):
		return rankSlug
	case contains(e.Name, text):
		return rankName
	case anyField(e.Categories, text, contains):
		return rankCategory
	case contains(e.Description, text):
		return rankDescription
	}
	return rankNone
}

func anyField(fields []string, want string, match func(field, want string) bool) bool {
	return slices.ContainsFunc(fields, func(f string) bool { return match(f, want) })
}

// contains reports whether field contains want, case-insensitively. want is
// already lowercased by the caller.
func contains(field, want string) bool {
	return field != "" && want != "" && strings.Contains(strings.ToLower(field), want)
}

// pasted reports containment in either direction. Only slugs and domains use
// it, because those are identifiers a caller pastes a longer real-world form
// of: "square-pos-terminal" has to find "square-pos". Reading a three-letter
// category the same way would turn "position tracking" into a hit for every
// point-of-sale corpus in the atlas.
func pasted(field, want string) bool {
	if field == "" || want == "" {
		return false
	}
	f := strings.ToLower(field)
	return strings.Contains(f, want) || strings.Contains(want, f)
}

func sameDomain(field, want string) bool {
	return normalizeDomain(strings.ToLower(field)) == want
}

// normalizeDomain reduces whatever the caller pasted to a bare hostname. An
// agent about to automate a site has a URL, so "https://squareup.com/pos",
// "www.squareup.com", and "squareup.com" all have to name the same entry.
func normalizeDomain(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "//" + s // so url.Parse reads it as a host, not a path
	}
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSuffix(u.Hostname(), "."), "www.")
}

// ValidateSlug reports whether s is usable as an atlas slug. A slug is spliced
// into a fetch URL and printed verbatim, so it must be non-empty, valid UTF-8,
// free of control characters, and unable to walk out of a URL path segment.
func ValidateSlug(s string) error {
	switch {
	case s == "":
		return errors.New("is empty")
	case !utf8.ValidString(s):
		return errors.New("is not valid UTF-8")
	case strings.ContainsFunc(s, unicode.IsControl):
		return errors.New("contains a control character")
	case strings.ContainsAny(s, `/\`):
		return errors.New("contains a path separator")
	case strings.Contains(s, ".."):
		return errors.New(`contains ".."`)
	}
	return nil
}

// SafeText renders untrusted text for a terminal: control characters, including
// the ESC that drives cursor movement, colour, and window titles, become their
// escaped form, and invalid UTF-8 becomes U+FFFD. [ParseIndex] applies it to
// every display field, so a caller needs it only for strings a user typed and
// URLs echoed back in errors.
func SafeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsControl(r) {
			fmt.Fprintf(&b, `\x%02x`, r)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
