package atlas

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// DefaultIndexURL is the published index of the community atlas. Every fetch
// URL is derived from the index URL (see the layout rule in the package doc),
// so overriding it is enough to redirect a whole install at a mirror or a test
// server.
const DefaultIndexURL = "https://raw.githubusercontent.com/sightmap/atlas/main/index.json"

// SchemaVersion is the index schema version this package understands. An index
// declaring a higher version is refused before its entries are decoded.
const SchemaVersion = 1

// MaxEntryFiles caps how many files one entry may list. Corpora are a handful
// of YAML files; anything larger is a malformed or hostile index, not a
// corpus.
const MaxEntryFiles = 256

// Index is the subset of index.json that sightmap consumes. Unknown fields are
// deliberately ignored so the atlas can grow metadata without breaking
// already-shipped CLIs.
type Index struct {
	SchemaVersion int     `json:"schema_version"`
	Entries       []Entry `json:"entries"`
}

// Entry is one published corpus.
type Entry struct {
	Slug   string   `json:"slug"`
	Name   string   `json:"name,omitempty"`
	Commit string   `json:"commit,omitempty"` // 40-char sha pinning the entry's content; empty = the index's own ref
	Files  []string `json:"files"`            // corpus-relative paths, all under .sightmap/
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

// Entry returns the entry with the given slug, or nil when the atlas publishes
// none. The first match wins; a well-formed index has no duplicate slugs (see
// [Index.Validate]).
func (ix *Index) Entry(slug string) *Entry {
	for i := range ix.Entries {
		if ix.Entries[i].Slug == slug {
			return &ix.Entries[i]
		}
	}
	return nil
}

// maxSuggestions caps the "did you mean" list on a slug miss.
const maxSuggestions = 5

// SuggestSlugs returns up to five published slugs resembling want, for the
// "did you mean" line on a miss. Resemblance is substring containment in
// either direction; ties — and everything else — break alphabetically, so the
// list is deterministic. Slugs that would be refused at install time are never
// suggested, which also keeps unvalidated index strings out of the message.
func (ix *Index) SuggestSlugs(want string) []string {
	w := strings.ToLower(want)
	if w == "" {
		return nil
	}
	seen := make(map[string]bool, len(ix.Entries))
	var out []string
	for _, e := range ix.Entries {
		if ValidateSlug(e.Slug) != nil || seen[e.Slug] {
			continue
		}
		s := strings.ToLower(e.Slug)
		if !strings.Contains(s, w) && !strings.Contains(w, s) {
			continue
		}
		seen[e.Slug] = true
		out = append(out, e.Slug)
	}
	sort.Strings(out)
	if len(out) > maxSuggestions {
		out = out[:maxSuggestions]
	}
	return out
}

// Validate reports every problem in an index: a schema version this sightmap
// cannot read, duplicate slugs, and anything [Entry.Validate] rejects. It is
// the check the atlas publisher CI runs so a merged entry is one every shipped
// CLI will install. [Install] does not call it — one broken entry must not
// block installing a good one.
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
