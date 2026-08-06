package atlas

import (
	"net/url"
	"sort"
	"strings"
)

// Query is what a caller is looking for. An empty Text matches every entry,
// which is what `sightmap atlas list` runs.
type Query struct {
	// Text is matched against the slug, the name, the domains, the categories,
	// and the description.
	Text string
	// Category, when set, keeps only entries publishing a matching category.
	Category string
}

// Rank is why an entry matched, and orders the results. Lower ranks come
// first, so an exact domain hit outranks a word that happens to appear in a
// description. The caller usually has a URL: `squareup.com` must land on the
// Square entry ahead of every entry whose prose mentions Square.
type Rank int

const (
	RankDomainExact Rank = iota
	RankSlugExact
	RankDomain
	RankSlug
	RankName
	RankCategory
	RankDescription
	rankNone
)

// String names a rank for display.
func (r Rank) String() string {
	switch r {
	case RankDomainExact:
		return "exact domain"
	case RankSlugExact:
		return "exact slug"
	case RankDomain:
		return "domain"
	case RankSlug:
		return "slug"
	case RankName:
		return "name"
	case RankCategory:
		return "category"
	case RankDescription:
		return "description"
	}
	return ""
}

// Hit is one search result.
type Hit struct {
	Entry Entry
	Rank  Rank
}

// Search returns the entries matching q, best first, ties broken by slug so
// the order is deterministic. An entry whose slug [ValidateSlug] rejects is
// dropped: results carry an install command, and a slug that cannot be
// installed must never be offered as one. Every other field is escaped on the
// way out ([SafeText], [Entry.Detail]) rather than being grounds to hide a
// corpus — a stray byte in a description is the publisher's problem to fix,
// which is what [Entry.Validate] is for, not a reason the caller cannot find
// the site they are looking at.
//
// Matching is case-insensitive substring containment. The field containing the
// query always matches, so `pos` finds `square-pos`. Slugs and domains match in
// the reverse direction too, so `square-pos-terminal` finds `square-pos`: those
// are the fields a caller pastes a longer real-world string around. Names,
// categories, and descriptions match forwards only — a category is three
// letters wide, and reading `position tracking` as a hit for `pos` returns
// every point-of-sale corpus in the atlas. Domains are additionally compared
// exactly after normalization, so a pasted URL, a bare hostname, and a `www.`
// hostname all resolve to the same entry.
func (ix *Index) Search(q Query) []Hit {
	text := strings.ToLower(strings.TrimSpace(q.Text))
	domain := normalizeDomain(text)
	category := strings.ToLower(strings.TrimSpace(q.Category))

	var hits []Hit
	for _, e := range ix.Entries {
		if ValidateSlug(e.Slug) != nil {
			continue
		}
		if category != "" && !containsAny(e.Categories, category) {
			continue
		}
		rank := rankEntry(&e, text, domain)
		if rank == rankNone {
			continue
		}
		hits = append(hits, Hit{Entry: e, Rank: rank})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Rank != hits[j].Rank {
			return hits[i].Rank < hits[j].Rank
		}
		return hits[i].Entry.Slug < hits[j].Entry.Slug
	})
	return hits
}

// rankEntry returns the best rank at which e matches text, or rankNone. An
// empty text matches everything at the lowest rank so `list` keeps the
// alphabetical order the tie-break gives it.
func rankEntry(e *Entry, text, domain string) Rank {
	if text == "" {
		return RankDomainExact
	}
	for _, d := range e.Domains {
		if normalizeDomain(strings.ToLower(d)) == domain && domain != "" {
			return RankDomainExact
		}
	}
	if strings.EqualFold(e.Slug, text) {
		return RankSlugExact
	}
	if matchesAny(e.Domains, text) {
		return RankDomain
	}
	if matchesText(e.Slug, text) {
		return RankSlug
	}
	if containsText(e.Name, text) {
		return RankName
	}
	if containsAny(e.Categories, text) {
		return RankCategory
	}
	if strings.Contains(strings.ToLower(e.Description), text) {
		return RankDescription
	}
	return rankNone
}

// matchesText is containment in either direction, case-insensitive. The
// reverse direction is the rule the old "did you mean" suggestion list used,
// and it belongs to identifiers the caller pastes a longer form of: slugs and
// hostnames. `square-pos-terminal` finding `square-pos` is what it buys.
//
// Only [rankEntry]'s slug and domain checks use it. Applied to a short field
// the reverse direction inverts the question — it asks whether the field is a
// substring of the query, and a three-letter category is a substring of a lot
// of queries.
func matchesText(field, want string) bool {
	if field == "" || want == "" {
		return false
	}
	f := strings.ToLower(field)
	return strings.Contains(f, want) || strings.Contains(want, f)
}

// containsText is plain containment: the field contains the query. Names,
// categories, and the `--category` filter use it. A caller typing `position
// tracking` is not asking for every corpus filed under `pos`, and a display
// name is prose the caller quotes from rather than an identifier they extend.
func containsText(field, want string) bool {
	if field == "" || want == "" {
		return false
	}
	return strings.Contains(strings.ToLower(field), want)
}

func matchesAny(fields []string, want string) bool {
	for _, f := range fields {
		if matchesText(f, want) {
			return true
		}
	}
	return false
}

func containsAny(fields []string, want string) bool {
	for _, f := range fields {
		if containsText(f, want) {
			return true
		}
	}
	return false
}

// normalizeDomain reduces whatever the caller pasted to a bare hostname: an
// agent about to automate a site has a URL, and `https://squareup.com/pos`,
// `www.squareup.com`, and `squareup.com` all name the same site.
func normalizeDomain(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	if strings.Contains(s, "://") {
		if u, err := url.Parse(s); err == nil && u.Hostname() != "" {
			s = u.Hostname()
		}
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	// Strip a port, but leave a bare IPv6 literal alone.
	if i := strings.LastIndex(s, ":"); i >= 0 && !strings.Contains(s, "]") {
		s = s[:i]
	}
	s = strings.TrimSuffix(s, ".")
	s = strings.TrimPrefix(s, "www.")
	return s
}
