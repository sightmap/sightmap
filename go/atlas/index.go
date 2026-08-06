package atlas

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DefaultIndexURL is the published index of the community atlas. `find` and
// `list` read it; an install does not, so an index outage or a schema bump
// never blocks `sightmap atlas add`.
//
// It is served from sightmap.org rather than read out of the atlas git repo,
// for two reasons. Moderation is one: removed.yaml and the atlas POLICY.md
// promise that a removed entry stops being reachable, and only the gallery
// rebuild honors that. A CLI reading the repo directly would keep finding
// entries the atlas has taken down. The other is that the gallery already
// serves this document, so a second copy read from somewhere else is a second
// answer to the same question.
//
// `--index` still overrides it, which is how a mirror or a private catalog
// works.
const DefaultIndexURL = "https://sightmap.org/atlas/index.json"

// AtlasURL is where a person browses the published corpora. `find` prints it
// when nothing matches and `add` prints it on a 404, so it points at the
// gallery a person can read rather than at a repository file tree.
const AtlasURL = "https://sightmap.org/atlas"

// SchemaVersion is the index schema version this package understands. An index
// declaring a higher version is refused before its entries are decoded.
const SchemaVersion = 1

// Index is the subset of index.json that sightmap consumes. Unknown fields are
// deliberately ignored so the atlas can grow metadata without breaking
// already-shipped CLIs.
type Index struct {
	SchemaVersion int     `json:"schema_version"`
	Entries       []Entry `json:"entries"`
}

// Entry is one published corpus. Everything but the slug is display and search
// metadata: an install needs only the slug.
type Entry struct {
	Slug         string   `json:"slug"`
	Name         string   `json:"name,omitempty"`
	Description  string   `json:"description,omitempty"`
	Domains      []string `json:"domains,omitempty"`
	Categories   []string `json:"categories,omitempty"`
	Stats        Stats    `json:"stats,omitempty"`
	LastVerified string   `json:"last_verified,omitempty"`
}

// Stats is how much of a site an entry maps. The three counts are the ones
// the index schema publishes and the ones the gallery card shows, so `find`
// and the gallery summarize an entry the same way from the same document.
type Stats struct {
	Views      int `json:"views,omitempty"`
	Components int `json:"components,omitempty"`
	Requests   int `json:"requests,omitempty"`
}

// summary renders the counts for one display line, or "" when the entry
// publishes none. Requests is dropped when it is zero: an index written before
// the field existed says nothing about requests, and "0 requests" would report
// that as a corpus that maps none.
func (s Stats) summary() string {
	if s.Views == 0 && s.Components == 0 && s.Requests == 0 {
		return ""
	}
	out := fmt.Sprintf("%d views, %d components", s.Views, s.Components)
	if s.Requests > 0 {
		out += fmt.Sprintf(", %d requests", s.Requests)
	}
	return out
}

// Detail renders one terminal-safe line describing what an entry covers: its
// domains, its categories, how much of the site it maps, and when someone last
// checked it against the live site. It lives here rather than in the CLI
// because the strings it composes are untrusted and the escaping belongs with
// the type that owns them.
func (e *Entry) Detail() string {
	var parts []string
	if d := safeList(e.Domains); d != "" {
		parts = append(parts, d)
	}
	if c := safeList(e.Categories); c != "" {
		parts = append(parts, c)
	}
	if s := e.Stats.summary(); s != "" {
		parts = append(parts, s)
	}
	if v := SafeText(e.LastVerified); v != "" {
		parts = append(parts, "verified "+v)
	}
	if len(parts) == 0 {
		return "(the atlas publishes no details for this entry)"
	}
	return strings.Join(parts, " · ")
}

// ParseIndex decodes an atlas index, gating on schema_version *before* the
// document's shape is assumed. A v2 index that restructures entries therefore
// reports "upgrade sightmap" instead of dying on a field-level JSON error.
func ParseIndex(data []byte) (*Index, error) {
	var gate struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &gate); err != nil {
		return nil, fmt.Errorf("parse atlas index: %w", err)
	}
	if gate.SchemaVersion > SchemaVersion {
		return nil, fmt.Errorf("atlas index has schema_version %d, but this sightmap only understands %d — upgrade sightmap", gate.SchemaVersion, SchemaVersion)
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parse atlas index: %w", err)
	}
	return &idx, nil
}

// Validate reports every problem in an index: a schema version this sightmap
// cannot read, duplicate slugs, and anything [Entry.Validate] rejects. It is
// the check the atlas publisher CI runs so a merged entry is one every shipped
// CLI can present. [Index.Search] does not call it wholesale — one broken entry
// must not hide the rest — it drops the offending entry instead.
func (ix *Index) Validate() []error {
	var errs []error
	if ix.SchemaVersion > SchemaVersion {
		errs = append(errs, fmt.Errorf("schema_version %d exceeds the supported version %d", ix.SchemaVersion, SchemaVersion))
	}
	seen := make(map[string]bool, len(ix.Entries))
	for i := range ix.Entries {
		e := &ix.Entries[i]
		if err := e.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("entry %d: %w", i, err))
			continue
		}
		if seen[e.Slug] {
			errs = append(errs, fmt.Errorf("entry %d: duplicate slug %q", i, SafeText(e.Slug)))
		}
		seen[e.Slug] = true
	}
	return errs
}
