package atlas

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
)

// ── fixtures shared by every test in the package ──────────────────────────────

// member is one tar entry, spelled out so a test can publish something an
// honest packer never would: an absolute path, a symlink, a lying size.
type member struct {
	name string
	body string
	typ  byte  // tar.TypeReg when zero
	size int64 // overrides the declared size when non-zero
}

func tarGz(t *testing.T, members []member) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for _, m := range members {
		typ := m.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		hdr := &tar.Header{Name: m.name, Mode: 0o644, Typeflag: typ, Size: int64(len(m.body))}
		switch typ {
		case tar.TypeDir:
			hdr.Mode, hdr.Size = 0o755, 0
		case tar.TypeSymlink:
			hdr.Linkname, hdr.Size = m.body, 0
		}
		if m.size != 0 {
			hdr.Size = m.size
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header %s: %v", m.name, err)
		}
		if hdr.Typeflag == tar.TypeReg && hdr.Size > 0 {
			if _, err := tw.Write([]byte(m.body)); err != nil {
				t.Fatalf("write body %s: %v", m.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

// corpusArchive packs corpus-relative files under the .sightmap/ prefix an
// atlas archive publishes.
func corpusArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	members := []member{{name: corpusPrefix, typ: tar.TypeDir}}
	for name, body := range files {
		members = append(members, member{name: corpusPrefix + name, body: body})
	}
	return tarGz(t, members)
}

const (
	configYAML = "version: 1\n"
	viewYAML   = "version: 1\nviews: []\n"
)

// fakeAtlas serves bodies keyed by URL path, so a test spells out exactly which
// URLs exist and records which were asked for.
type fakeAtlas struct {
	*httptest.Server
	mu        sync.Mutex
	requested []string
}

func newFakeAtlas(t *testing.T, bodies map[string][]byte) *fakeAtlas {
	t.Helper()
	f := &fakeAtlas{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requested = append(f.requested, r.URL.Path)
		f.mu.Unlock()
		body, ok := bodies[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(body)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeAtlas) indexURL() string { return f.URL + "/index.json" }

func (f *fakeAtlas) archiveTemplate() string { return f.URL + "/entries/{slug}.tar.gz" }

func (f *fakeAtlas) paths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.requested)
}

func mustContain(t *testing.T, got string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("output = %q, want it to mention %q", got, w)
		}
	}
}

// ── defaults ──────────────────────────────────────────────────────────────────

// Both URLs have to resolve to the domain the gallery rebuild controls.
// removed.yaml promises that a removed entry stops being reachable, and only
// that rebuild honors it: a default pointing anywhere else keeps a taken-down
// entry findable and installable.
func TestDefaults_comeFromTheGallery(t *testing.T) {
	for name, raw := range map[string]string{
		"DefaultIndexURL":   DefaultIndexURL,
		"DefaultArchiveURL": DefaultArchiveURL,
		"AtlasURL":          AtlasURL,
	} {
		if !strings.HasPrefix(raw, "https://sightmap.org/") {
			t.Errorf("%s = %q, want it served from https://sightmap.org/", name, raw)
		}
	}
}

// ── parsing ───────────────────────────────────────────────────────────────────

func TestParseIndex(t *testing.T) {
	for _, tc := range []struct {
		name    string
		json    string
		wantErr string
		check   func(*testing.T, *Index)
	}{{
		name:    "a future schema is refused before the entries are decoded",
		json:    `{"schema_version": 2, "entries": "restructured, not a list at all"}`,
		wantErr: "upgrade sightmap",
	}, {
		name:    "malformed JSON is reported as such",
		json:    `{"schema_version":`,
		wantErr: "parse atlas index",
	}, {
		name: "unknown fields are ignored so the atlas can grow metadata",
		json: `{"schema_version": 1, "stars": 9, "entries": [
		  {"slug": "square-pos", "name": "Square POS", "screenshots": ["a.png"],
		   "stats": {"views": 12, "components": 48, "requests": 23}}
		]}`,
		check: func(t *testing.T, ix *Index) {
			e := ix.Entries[0]
			if e.Slug != "square-pos" || e.Stats.Requests != 23 {
				t.Errorf("entry = %+v, want the known fields decoded", e)
			}
		},
	}, {
		name: "an older schema still parses",
		json: `{"schema_version": 0, "entries": [{"slug": "old"}]}`,
		check: func(t *testing.T, ix *Index) {
			if len(ix.Entries) != 1 {
				t.Errorf("entries = %d, want 1", len(ix.Entries))
			}
		},
	}, {
		// Escaping at the boundary is what lets every caller print index text
		// without escaping it again.
		name: "control characters are escaped out of every display field",
		json: `{"schema_version": 1, "entries": [{
		  "slug": "evil", "name": "Evil\u001b]0;pwned\u0007",
		  "description": "desc\u001b[2K", "domains": ["evil.test\u001b[2K"],
		  "categories": ["pay\u0007ments"], "last_verified": "2026-01-01\u001b[31m"
		}]}`,
		check: func(t *testing.T, ix *Index) {
			e := ix.Entries[0]
			joined := e.Name + e.Description + e.LastVerified + strings.Join(e.Domains, "") + strings.Join(e.Categories, "")
			if strings.ContainsAny(joined, "\x1b\x07") {
				t.Fatalf("entry = %+v, want no control characters left", e)
			}
			mustContain(t, joined, `Evil\x1b]0;pwned\x07`, `evil.test\x1b[2K`, `pay\x07ments`, `2026-01-01\x1b[31m`)
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			idx, err := ParseIndex([]byte(tc.json))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("ParseIndex err = %v, want it to mention %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseIndex: %v", err)
			}
			tc.check(t, idx)
		})
	}
}

// Validate is the publisher's check, so it has to see the catalog as published.
// ParseIndex escapes the same fields as it decodes, which means nothing
// downstream can notice them and the publisher is the only one who can fix them.
func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		name     string
		json     string
		wantErrs []string // a substring of each expected problem, in order
	}{{
		name: "a clean catalog",
		json: `{"schema_version": 1, "entries": [
		  {"slug": "square-pos", "name": "Square POS", "domains": ["squareup.com"]},
		  {"slug": "acme-shop", "name": "Acme Shop"}
		]}`,
	}, {
		name:     "a schema this sightmap cannot read",
		json:     `{"schema_version": 99, "entries": []}`,
		wantErrs: []string{"upgrade sightmap"},
	}, {
		// The second entry would silently shadow the first at install time.
		name: "a duplicate slug",
		json: `{"schema_version": 1, "entries": [
		  {"slug": "square-pos"}, {"slug": "square-pos", "name": "Impostor"}
		]}`,
		wantErrs: []string{`entry 1: duplicate slug "square-pos"`},
	}, {
		name: "a slug that could never be installed",
		json: `{"schema_version": 1, "entries": [
		  {"slug": "../../etc/passwd"}, {"slug": ""}
		]}`,
		wantErrs: []string{"entry 0: slug", "path separator", "entry 1: slug", "is empty"},
	}, {
		// ParseIndex would escape these away, so this is the only place a
		// publisher hears about the byte they cannot see in their editor.
		name: "control characters in display text",
		json: `{"schema_version": 1, "entries": [{
		  "slug": "evil", "name": "Evil\u001b]0;pwned\u0007",
		  "domains": ["ok.test", "evil.test\u001b[2K"]
		}]}`,
		wantErrs: []string{`Evil\x1b]0;pwned\x07`, `evil.test\x1b[2K`},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			problems := Validate([]byte(tc.json))
			if len(tc.wantErrs) == 0 {
				if len(problems) != 0 {
					t.Fatalf("Validate = %v, want no problems", problems)
				}
				return
			}
			var joined []string
			for _, p := range problems {
				joined = append(joined, p.Error())
			}
			got := strings.Join(joined, "\n")
			mustContain(t, got, tc.wantErrs...)
			// A bad entry must not hide the rest of the catalog.
			if len(problems) < 1 {
				t.Fatalf("Validate = %v, want at least one problem", problems)
			}
		})
	}
}

// ── search ────────────────────────────────────────────────────────────────────

// searchIndex is the catalog the ranking tests search. "square-pos" and
// "block-dashboard" deliberately collide: both mention Square, and only one of
// them owns squareup.com.
func searchIndex() *Index {
	return &Index{SchemaVersion: 1, Entries: []Entry{
		{
			Slug: "block-dashboard", Name: "Block Dashboard",
			Description: "Merchant dashboard for Square sellers.",
			Domains:     []string{"block.xyz"},
			Categories:  []string{"payments"},
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
	}}
}

func slugs(hits []Hit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.Entry.Slug
	}
	return out
}

func TestSearch(t *testing.T) {
	for _, tc := range []struct {
		name      string
		query     Query
		want      []string
		matchedOn string // of the first hit, when it matters
	}{
		// The consumer that matters is an agent holding a URL. However the URL
		// is spelled, the entry that owns the domain has to come first, or the
		// agent installs the corpus for a different product.
		{"a bare hostname", Query{Text: "squareup.com"}, []string{"square-pos"}, "exact domain"},
		{"a pasted URL", Query{Text: "https://squareup.com/dashboard/orders"}, []string{"square-pos"}, "exact domain"},
		{"a www host", Query{Text: "www.squareup.com"}, []string{"square-pos"}, "exact domain"},
		{"mixed case", Query{Text: "SquareUp.com"}, []string{"square-pos"}, "exact domain"},
		{"a host with a port", Query{Text: "squareup.com:443"}, []string{"square-pos"}, "exact domain"},
		// An agent lands on app.squareup.com as often as on the apex.
		{"a published subdomain", Query{Text: "app.squareup.com"}, []string{"square-pos"}, "exact domain"},

		// A word in a description still matches, below the entries carrying it
		// in a field that identifies them.
		{"identity fields outrank prose", Query{Text: "square"}, []string{"square-pos", "block-dashboard"}, "domain"},

		// A slug matches in either direction: a caller pastes a longer
		// real-world product string around it.
		{"a slug fragment", Query{Text: "pos"}, []string{"square-pos"}, "slug"},
		{"a longer string around a slug", Query{Text: "square-pos-terminal"}, []string{"square-pos"}, "slug"},
		{"an exact slug", Query{Text: "acme-shop"}, []string{"acme-shop"}, "exact slug"},

		{"an empty query lists everything alphabetically", Query{}, []string{"acme-shop", "block-dashboard", "square-pos"}, ""},
		{"a category filter", Query{Category: "commerce"}, []string{"acme-shop", "square-pos"}, ""},
		{"a query and a category together", Query{Text: "square", Category: "commerce"}, []string{"square-pos"}, ""},
		{"an unmapped site", Query{Text: "stripe.com"}, nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hits := searchIndex().Search(tc.query)
			if got := slugs(hits); !slices.Equal(got, tc.want) {
				t.Fatalf("Search(%+v) = %v, want %v", tc.query, got, tc.want)
			}
			if tc.matchedOn != "" && hits[0].MatchedOn != tc.matchedOn {
				t.Errorf("MatchedOn = %q, want %q", hits[0].MatchedOn, tc.matchedOn)
			}
		})
	}
}

// The reverse direction stops at slugs and domains. Read into a category it
// inverts the question: "pos" is a substring of "position tracking", so every
// point-of-sale corpus in the atlas would answer a query about tracking.
func TestSearch_shortFieldsDoNotMatchALongerQueryThatSpellsThem(t *testing.T) {
	ix := &Index{SchemaVersion: 1, Entries: []Entry{
		{Slug: "toast-terminal", Name: "Toast Terminal", Categories: []string{"pos"}},
		{Slug: "lightspeed-retail", Name: "POS", Categories: []string{"retail"}},
	}}
	for _, q := range []Query{{Text: "position tracking"}, {Category: "position tracking"}} {
		if hits := ix.Search(q); len(hits) != 0 {
			t.Errorf("Search(%+v) = %v, want no hits", q, slugs(hits))
		}
	}
	// Forward containment is untouched: "pos" still finds both, by name ahead
	// of by category.
	hits := ix.Search(Query{Text: "pos"})
	if got := slugs(hits); !slices.Equal(got, []string{"lightspeed-retail", "toast-terminal"}) {
		t.Fatalf("Search(pos) = %v, want [lightspeed-retail toast-terminal]", got)
	}
	if hits[0].MatchedOn != "name" || hits[1].MatchedOn != "category" {
		t.Errorf("matched on %q/%q, want name then category", hits[0].MatchedOn, hits[1].MatchedOn)
	}
}

// A hit carries an install command, so an entry that could never be installed
// must never be offered as one. A hostile *name* is different: it is escaped at
// parse time, not grounds to hide a site the caller is looking at.
func TestSearch_dropsEntriesThatCouldNotBeInstalled(t *testing.T) {
	ix := &Index{Entries: []Entry{
		{Slug: "../../etc/passwd", Name: "traversal"},
		{Slug: "esc\x1b[2Kok", Name: "escape sequence"},
		{Slug: "", Name: "empty"},
		{Slug: "real-entry", Name: "Real"},
	}}
	if got := slugs(ix.Search(Query{})); !slices.Equal(got, []string{"real-entry"}) {
		t.Fatalf("Search({}) = %v, want only real-entry", got)
	}
}

// ── slugs and escaping ────────────────────────────────────────────────────────

func TestValidateSlug(t *testing.T) {
	for _, tc := range []struct{ slug, wantErr string }{
		{"square-pos", ""},
		{"", "is empty"},
		{"../../etc/passwd", "path separator"},
		{"a/b", "path separator"},
		{`a\b`, "path separator"},
		{"up..down", `".."`},
		{"esc\x1b[2K", "control character"},
		{"\xff\xfe", "valid UTF-8"},
	} {
		err := ValidateSlug(tc.slug)
		switch {
		case tc.wantErr == "" && err != nil:
			t.Errorf("ValidateSlug(%q) = %v, want nil", tc.slug, err)
		case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
			t.Errorf("ValidateSlug(%q) = %v, want it to mention %q", tc.slug, err, tc.wantErr)
		}
	}
}

// ESC drives cursor movement, colour, and window titles, so it must never reach
// a terminal raw. Everything else survives unchanged.
func TestSafeText(t *testing.T) {
	for in, want := range map[string]string{
		"Square POS":        "Square POS",
		"Évoluer — naïve":   "Évoluer — naïve",
		"esc\x1b[2K":        `esc\x1b[2K`,
		"bell\x07":          `bell\x07`,
		"title\x1b]0;x\x1b": `title\x1b]0;x\x1b`,
		"\x00\x7f":          `\x00\x7f`,
	} {
		if got := SafeText(in); got != want {
			t.Errorf("SafeText(%q) = %q, want %q", in, got, want)
		}
	}
	if got := SafeText("bad\xffbyte"); strings.Contains(got, "\xff") {
		t.Errorf("SafeText(invalid utf-8) = %q, want the byte replaced", got)
	}
}

// The gallery card and `find` read the same index.json, so they summarize an
// entry the same way. A count the catalog omits is left out rather than printed
// as zero, which would claim the corpus maps none.
func TestStatsCounts(t *testing.T) {
	for _, tc := range []struct {
		stats Stats
		want  []string
	}{
		{Stats{Views: 12, Components: 48, Requests: 23}, []string{"12 views", "48 components", "23 requests"}},
		{Stats{Views: 12, Components: 48}, []string{"12 views", "48 components"}},
		{Stats{}, nil},
	} {
		if got := tc.stats.Counts(); !slices.Equal(got, tc.want) {
			t.Errorf("Stats%+v.Counts() = %v, want %v", tc.stats, got, tc.want)
		}
	}
}
