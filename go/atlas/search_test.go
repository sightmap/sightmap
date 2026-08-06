package atlas

import (
	"strings"
	"testing"
)

// searchIndex is the catalog the ranking tests search. "square-pos" and
// "block-dashboard" deliberately collide: both mention Square, and only one of
// them owns squareup.com.
func searchIndex() *Index {
	return &Index{
		SchemaVersion: 1,
		Entries: []Entry{
			{
				Slug: "block-dashboard", Name: "Block Dashboard",
				Description: "Merchant dashboard for Square sellers.",
				Domains:     []string{"block.xyz"},
				Categories:  []string{"payments"},
				Stats:       Stats{Views: 4, Components: 19},
			},
			{
				Slug: "square-pos", Name: "Square POS",
				Description:  "Point-of-sale checkout and order history.",
				Domains:      []string{"squareup.com", "app.squareup.com"},
				Categories:   []string{"payments", "commerce"},
				Stats:        Stats{Views: 12, Components: 48, Requests: 23},
				LastVerified: "2026-07-14",
			},
			{
				Slug: "acme-shop", Name: "Acme Shop",
				Description: "Storefront, cart, and checkout.",
				Domains:     []string{"shop.acme.test"},
				Categories:  []string{"commerce"},
			},
		},
	}
}

func slugs(hits []Hit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.Entry.Slug
	}
	return out
}

// The consumer that matters is an agent holding a URL. Whatever else matches,
// the entry that owns the domain has to come first, or the agent installs the
// corpus for a different product.
func TestSearch_exactDomainRanksFirst(t *testing.T) {
	for _, query := range []string{
		"squareup.com",
		"https://squareup.com/dashboard/orders",
		"www.squareup.com",
		"SquareUp.com",
		"squareup.com:443",
	} {
		hits := searchIndex().Search(Query{Text: query})
		if len(hits) == 0 {
			t.Fatalf("Search(%q) found nothing", query)
		}
		if hits[0].Entry.Slug != "square-pos" {
			t.Errorf("Search(%q) ranked %v first, want square-pos", query, slugs(hits))
		}
		if hits[0].Rank != RankDomainExact {
			t.Errorf("Search(%q) rank = %v, want RankDomainExact", query, hits[0].Rank)
		}
	}
}

// A subdomain the entry publishes is an exact match too — an agent lands on
// app.squareup.com as often as on the apex.
func TestSearch_exactDomainMatchesAPublishedSubdomain(t *testing.T) {
	hits := searchIndex().Search(Query{Text: "app.squareup.com"})
	if len(hits) == 0 || hits[0].Entry.Slug != "square-pos" || hits[0].Rank != RankDomainExact {
		t.Fatalf("Search(app.squareup.com) = %v, want square-pos on an exact domain match", slugs(hits))
	}
}

// A word in a description still matches, but it ranks below the entries that
// carry the word in a field that identifies them.
func TestSearch_ranksIdentityFieldsAheadOfProse(t *testing.T) {
	hits := searchIndex().Search(Query{Text: "square"})
	if got := slugs(hits); len(got) != 2 || got[0] != "square-pos" || got[1] != "block-dashboard" {
		t.Fatalf("Search(square) = %v, want [square-pos block-dashboard]", got)
	}
	if hits[0].Rank != RankDomain || hits[1].Rank != RankDescription {
		t.Errorf("ranks = %v/%v, want domain then description", hits[0].Rank, hits[1].Rank)
	}
}

// A slug matches in either direction: `pos` finds `square-pos`, and so does a
// longer product string the caller pasted around it. This is the case the
// reverse direction exists for and the one the narrowing must not break.
func TestSearch_slugMatchesInEitherDirection(t *testing.T) {
	for query, want := range map[string]string{
		"pos":                 "square-pos",
		"square-pos-terminal": "square-pos",
		"square-pos-android":  "square-pos",
		"acme":                "acme-shop",
	} {
		hits := searchIndex().Search(Query{Text: query})
		if len(hits) == 0 || hits[0].Entry.Slug != want {
			t.Errorf("Search(%q) = %v, want %s first", query, slugs(hits), want)
		}
	}
}

// The reverse direction stops at slugs and domains. Read into a category it
// inverts the question: `pos` is a substring of `position tracking`, so every
// point-of-sale corpus in the atlas would answer a query about tracking.
func TestSearch_shortCategoryDoesNotMatchALongerQueryThatSpellsIt(t *testing.T) {
	ix := &Index{SchemaVersion: 1, Entries: []Entry{
		{Slug: "toast-terminal", Name: "Toast Terminal", Categories: []string{"pos"}},
		{Slug: "lightspeed-retail", Name: "POS", Categories: []string{"retail"}},
	}}
	if hits := ix.Search(Query{Text: "position tracking"}); len(hits) != 0 {
		t.Errorf("Search(position tracking) = %v, want no hits", slugs(hits))
	}
	if hits := ix.Search(Query{Category: "position tracking"}); len(hits) != 0 {
		t.Errorf("Search(--category position tracking) = %v, want no hits", slugs(hits))
	}
	// Forward containment is untouched: `pos` still finds both, by name ahead
	// of by category.
	hits := ix.Search(Query{Text: "pos"})
	if got := slugs(hits); len(got) != 2 || got[0] != "lightspeed-retail" || got[1] != "toast-terminal" {
		t.Fatalf("Search(pos) = %v, want [lightspeed-retail toast-terminal]", got)
	}
	if hits[0].Rank != RankName || hits[1].Rank != RankCategory {
		t.Errorf("ranks = %v/%v, want name then category", hits[0].Rank, hits[1].Rank)
	}
}

// An empty query is what `atlas list` runs: everything, in a stable order.
func TestSearch_emptyQueryListsEverythingAlphabetically(t *testing.T) {
	hits := searchIndex().Search(Query{})
	if got := slugs(hits); len(got) != 3 || got[0] != "acme-shop" || got[1] != "block-dashboard" || got[2] != "square-pos" {
		t.Fatalf("Search({}) = %v, want every slug alphabetically", got)
	}
}

func TestSearch_categoryFilters(t *testing.T) {
	hits := searchIndex().Search(Query{Category: "commerce"})
	if got := slugs(hits); len(got) != 2 || got[0] != "acme-shop" || got[1] != "square-pos" {
		t.Fatalf("Search(category=commerce) = %v, want [acme-shop square-pos]", got)
	}
	if hits := searchIndex().Search(Query{Text: "square", Category: "commerce"}); len(hits) != 1 || hits[0].Entry.Slug != "square-pos" {
		t.Fatalf("Search(square, category=commerce) = %v, want [square-pos]", slugs(hits))
	}
}

func TestSearch_noMatchReturnsNothing(t *testing.T) {
	if hits := searchIndex().Search(Query{Text: "stripe.com"}); len(hits) != 0 {
		t.Fatalf("Search(stripe.com) = %v, want no hits", slugs(hits))
	}
}

// Results carry an install command. An entry whose slug could never be
// installed must not be offered as one — and its ESC bytes must not reach a
// terminal on the way.
func TestSearch_dropsEntriesThatCouldNotBeInstalled(t *testing.T) {
	ix := &Index{Entries: []Entry{
		{Slug: "../../etc/passwd", Name: "traversal"},
		{Slug: "esc\x1b[2Kok", Name: "escape sequence"},
		{Slug: "", Name: "empty"},
		{Slug: "real-entry", Name: "Real"},
	}}
	hits := ix.Search(Query{})
	if got := slugs(hits); len(got) != 1 || got[0] != "real-entry" {
		t.Fatalf("Search({}) = %v, want only real-entry", got)
	}
}

// A stray byte in a name is the publisher's problem, not a reason the caller
// cannot find the site they are looking at. The entry stays; the escaping on
// the way out is what handles it.
func TestSearch_keepsAnInstallableEntryWithHostileDisplayText(t *testing.T) {
	ix := &Index{Entries: []Entry{{
		Slug:    "square-pos",
		Name:    "Square\x1b[2K POS",
		Domains: []string{"squareup.com"},
	}}}
	hits := ix.Search(Query{Text: "squareup.com"})
	if len(hits) != 1 {
		t.Fatalf("Search = %v, want the entry kept", slugs(hits))
	}
	if got := SafeText(hits[0].Entry.Name); strings.Contains(got, "\x1b") {
		t.Errorf("SafeText(name) = %q, want the escape rendered", got)
	}
}

// A hostile *name* is not a reason to hide an otherwise installable entry, so
// it is escaped rather than dropped — the display path is the last line of
// defence for everything the index says.
func TestEntryDetail_escapesHostileIndexStrings(t *testing.T) {
	e := Entry{
		Slug:         "evil",
		Name:         "Evil\x1b]0;pwned\x07",
		Domains:      []string{"evil.test\x1b[2K"},
		Categories:   []string{"pay\x07ments"},
		LastVerified: "2026-01-01\x1b[31m",
		Stats:        Stats{Views: 1, Components: 2},
	}
	detail := e.Detail()
	if strings.ContainsAny(detail, "\x1b\x07") {
		t.Fatalf("Detail() = %q, want no control characters", detail)
	}
	mustContain(t, detail, `evil.test\x1b[2K`, `pay\x07ments`, "1 views, 2 components", `verified 2026-01-01\x1b[31m`)
	if name := SafeText(e.Name); strings.ContainsAny(name, "\x1b\x07") {
		t.Errorf("SafeText(name) = %q, want no control characters", name)
	}
}

// SafeText bounds how much index text one line may carry, so a 40 KB "name"
// cannot push the install command off the screen.
func TestSafeText_truncates(t *testing.T) {
	got := SafeText(strings.Repeat("a", maxSafeTextRunes*3))
	if len([]rune(got)) != maxSafeTextRunes+1 || !strings.HasSuffix(got, "…") {
		t.Fatalf("SafeText(long) = %d runes, want %d plus an ellipsis", len([]rune(got)), maxSafeTextRunes)
	}
}

// The gallery card and `find` read the same index.json, so they have to
// summarize an entry the same way. Requests was the count `find` dropped.
func TestEntryDetail_countsRequests(t *testing.T) {
	e := Entry{Slug: "square-pos", Stats: Stats{Views: 12, Components: 48, Requests: 23}}
	if got := e.Detail(); !strings.Contains(got, "12 views, 48 components, 23 requests") {
		t.Fatalf("Detail() = %q, want all three counts", got)
	}
}

// An index published before requests existed does not claim the corpus maps
// none of them: the count is left off rather than printed as zero.
func TestEntryDetail_omitsRequestsWhenTheIndexPredatesTheField(t *testing.T) {
	idx, err := ParseIndex([]byte(`{"schema_version": 1, "entries": [
	  {"slug": "square-pos", "stats": {"views": 12, "components": 48}}
	]}`))
	if err != nil {
		t.Fatalf("ParseIndex: %v", err)
	}
	got := idx.Entries[0].Detail()
	if !strings.Contains(got, "12 views, 48 components") {
		t.Fatalf("Detail() = %q, want the counts it does publish", got)
	}
	if strings.Contains(got, "requests") {
		t.Errorf("Detail() = %q, want no requests count from an index that has none", got)
	}
}

func TestEntryDetail_saysSoWhenTheAtlasPublishesNothing(t *testing.T) {
	e := Entry{Slug: "bare"}
	if got := e.Detail(); !strings.Contains(got, "no details") {
		t.Fatalf("Detail() = %q, want it to say the atlas publishes no details", got)
	}
}
