package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// atlasFixture is a local atlas: one catalog and one archive, served over
// loopback so the transport policy's plain-http exception applies.
type atlasFixture struct {
	*httptest.Server
	index string
}

const atlasIndexJSON = `{
  "schema_version": 1,
  "entries": [
    {
      "slug": "square-pos",
      "name": "Square POS",
      "description": "Point-of-sale checkout and order history.",
      "domains": ["squareup.com", "app.squareup.com"],
      "categories": ["payments", "commerce"],
      "stats": {"views": 12, "components": 48, "requests": 23},
      "last_verified": "2026-07-14"
    },
    {
      "slug": "acme-shop",
      "name": "Acme Shop",
      "description": "Storefront, cart, and checkout.",
      "domains": ["shop.acme.test"],
      "categories": ["commerce"],
      "stats": {"views": 3, "components": 11}
    }
  ]
}`

func newAtlasFixture(t *testing.T) *atlasFixture {
	t.Helper()
	// Every case gets its own HOME so the catalog cache never touches the
	// developer's ~/.sightmap.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	archive := corpusTarGz(t, map[string]string{
		"config.yaml":         "version: 1\n",
		"views/checkout.yaml": "version: 1\nviews: []\n",
	})
	f := &atlasFixture{index: atlasIndexJSON}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.json":
			w.Write([]byte(f.index))
		case "/entries/square-pos.tar.gz":
			w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *atlasFixture) indexFlag() []string { return []string{"--index", f.URL + "/index.json"} }

func (f *atlasFixture) sourceFlag() []string {
	return []string{"--source", f.URL + "/entries/{slug}.tar.gz"}
}

func corpusTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for name, body := range files {
		hdr := &tar.Header{Name: ".sightmap/" + name, Mode: 0o644, Size: int64(len(body))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// atlasOut runs a subcommand through the same dispatch main.go uses.
func atlasOut(t *testing.T, f *atlasFixture, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := runAtlasOut(append(args, f.indexFlag()...), &out); err != nil {
		t.Fatalf("atlas %v: %v", args, err)
	}
	return out.String()
}

func mustContain(t *testing.T, got string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("output = %q, want it to mention %q", got, w)
		}
	}
}

// A result has to carry the command that installs it, so nothing downstream has
// to assemble one from a slug.
func TestAtlasFind_printsAnInstallCommandWithEachHit(t *testing.T) {
	f := newAtlasFixture(t)
	out := atlasOut(t, f, "find", "squareup.com")
	mustContain(t, out,
		"square-pos  Square POS",
		"Point-of-sale checkout",
		"squareup.com, app.squareup.com",
		"payments, commerce",
		"12 views, 48 components, 23 requests",
		"verified 2026-07-14",
		"sightmap atlas add square-pos",
		"1 match.",
	)
	if strings.Contains(out, "acme-shop") {
		t.Errorf("output = %q, want only the matching entry", out)
	}
}

// A search that finds nothing is a successful search: an agent branches on the
// exit code, so a miss must not look like a failure. The message has to name
// every constraint that was applied, or a category with nothing filed under it
// reads as an empty atlas and the agent authors a corpus that already exists.
func TestAtlasSearch_emptyResults(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{{
		name: "an unmapped site",
		args: []string{"find", "stripe.com"},
		want: []string{`No atlas entry matches "stripe.com".`, "sightmap atlas list", "sightmap init"},
	}, {
		name: "a category nothing is filed under",
		args: []string{"list", "--category", "aerospace"},
		want: []string{`No atlas entry is in category "aerospace".`},
	}, {
		name: "a query and a category that exclude each other",
		args: []string{"find", "squareup.com", "--category", "aerospace"},
		want: []string{`matches "squareup.com"`, `is in category "aerospace"`},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			out := atlasOut(t, newAtlasFixture(t), tc.args...)
			mustContain(t, out, tc.want...)
			if strings.Contains(out, "publishes no entries") {
				t.Errorf("output = %q, want it not to claim the atlas is empty", out)
			}
		})
	}
}

// An atlas with no entries at all is the one case that should say so.
func TestAtlasList_saysWhenTheCatalogIsEmpty(t *testing.T) {
	f := newAtlasFixture(t)
	f.index = `{"schema_version": 1, "entries": []}`
	mustContain(t, atlasOut(t, f, "list"), "publishes no entries")
}

func TestAtlasList_browsesTheWholeCatalogAndFiltersByCategory(t *testing.T) {
	f := newAtlasFixture(t)
	mustContain(t, atlasOut(t, f, "list"), "acme-shop", "square-pos", "2 matches.")

	payments := atlasOut(t, f, "list", "--category", "payments")
	mustContain(t, payments, "square-pos", "1 match.")
	if strings.Contains(payments, "acme-shop") {
		t.Errorf("--category payments returned %q", payments)
	}
}

// A truncated list has to say how much it left out, or a caller reads the first
// page as the whole atlas.
func TestAtlasFind_limitsResultsAndSaysHowManyThereAre(t *testing.T) {
	out := atlasOut(t, newAtlasFixture(t), "list", "--limit", "1")
	mustContain(t, out, "1 of 2 matches", "--limit")
}

// The flag package stops at the first positional, so flags on either side of
// the query have to keep working.
func TestAtlas_acceptsFlagsOnEitherSideOfThePositional(t *testing.T) {
	f := newAtlasFixture(t)
	var out bytes.Buffer
	args := append([]string{"find", "squareup.com", "--limit", "1"}, f.indexFlag()...)
	if err := runAtlasOut(args, &out); err != nil {
		t.Fatalf("atlas find: %v", err)
	}
	mustContain(t, out.String(), "square-pos")
}

// The --json document is the agent contract: every field the catalog publishes,
// why the entry matched, and the command that installs it.
func TestAtlasFind_jsonCarriesTheInstallCommand(t *testing.T) {
	out := atlasOut(t, newAtlasFixture(t), "find", "squareup.com", "--json")

	var doc struct {
		Query   string `json:"query"`
		Total   int    `json:"total"`
		Shown   int    `json:"shown"`
		Results []struct {
			Slug       string   `json:"slug"`
			Name       string   `json:"name"`
			Domains    []string `json:"domains"`
			MatchedOn  string   `json:"matched_on"`
			Install    string   `json:"install"`
			Categories []string `json:"categories"`
			Stats      struct {
				Views, Components, Requests int
			} `json:"stats"`
		} `json:"results"`
		Index struct {
			Source string `json:"source"`
			Cached bool   `json:"cached"`
		} `json:"index"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("--json emitted %q: %v", out, err)
	}
	if doc.Query != "squareup.com" || doc.Total != 1 || doc.Shown != 1 || len(doc.Results) != 1 {
		t.Fatalf("doc = %+v, want one result for squareup.com", doc)
	}
	r := doc.Results[0]
	if r.Slug != "square-pos" || r.Install != "sightmap atlas add square-pos" || r.MatchedOn != "exact domain" {
		t.Errorf("result = %+v", r)
	}
	if r.Stats.Requests != 23 || len(r.Domains) != 2 {
		t.Errorf("result dropped catalog fields: %+v", r)
	}
	if doc.Index.Source == "" {
		t.Error("index.source is empty, so a caller cannot tell which catalog answered")
	}
}

// An empty result must be an empty JSON array, not null, so a caller can range
// over it without a nil check.
func TestAtlasFind_jsonEmptyResultsIsAnArray(t *testing.T) {
	out := atlasOut(t, newAtlasFixture(t), "find", "stripe.com", "--json")
	mustContain(t, out, `"results": []`)
}

// End to end, nothing the catalog authored reaches the terminal raw. The
// escaping happens when the catalog is parsed; this is the check that the CLI
// still benefits from it after formatting.
func TestAtlas_escapesHostileCatalogText(t *testing.T) {
	f := newAtlasFixture(t)
	f.index = `{"schema_version": 1, "entries": [{
	  "slug": "evil", "name": "Evil\u001b]0;pwned\u0007",
	  "description": "desc\u001b[2K", "domains": ["evil.test\u001b[2K"]
	}]}`
	out := atlasOut(t, f, "find", "evil")
	if strings.ContainsAny(out, "\x1b\x07") {
		t.Fatalf("output = %q, want no control characters", out)
	}
	mustContain(t, out, `Evil\x1b]0;pwned\x07`, `evil.test\x1b[2K`)
}

// A catalog this CLI cannot read has to say so, rather than reporting an atlas
// with nothing in it.
func TestAtlasFind_reportsAFutureCatalogSchema(t *testing.T) {
	f := newAtlasFixture(t)
	f.index = `{"schema_version": 99, "entries": []}`
	err := runAtlasOut(append([]string{"find", "square"}, f.indexFlag()...), &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected an error")
	}
	mustContain(t, err.Error(), "upgrade sightmap")
}

func TestAtlas_usageErrors(t *testing.T) {
	for _, tc := range []struct {
		name, wantErr string
		args          []string
	}{
		{"find with no query", "expected a QUERY", []string{"find"}},
		{"add with no slug", "expected a SLUG", []string{"add"}},
		{"add with two slugs", "unexpected argument", []string{"add", "square-pos", "acme-shop"}},
		{"list with a positional", "use 'sightmap atlas find", []string{"list", "square"}},
		{"an unknown subcommand", "unknown subcommand", []string{"browse"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := runAtlasOut(tc.args, &bytes.Buffer{})
			if err == nil {
				t.Fatal("expected an error")
			}
			mustContain(t, err.Error(), tc.wantErr)
		})
	}
}

// The caller is the atlas repository's CI checking an index.json before it
// merges, so the exit code is the result and every problem is listed at once:
// a publisher fixing one entry per run is a publisher waiting on N builds.
func TestAtlasValidate(t *testing.T) {
	write := func(t *testing.T, body string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "index.json")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("a clean catalog exits 0", func(t *testing.T) {
		var out bytes.Buffer
		if err := runAtlasOut([]string{"validate", write(t, atlasIndexJSON)}, &out); err != nil {
			t.Fatalf("atlas validate: %v", err)
		}
		mustContain(t, out.String(), "The catalog is valid.")
	})

	t.Run("every problem is reported at once", func(t *testing.T) {
		path := write(t, `{"schema_version": 1, "entries": [
		  {"slug": "dupe"}, {"slug": "dupe"}, {"slug": "../escape"}
		]}`)
		var out bytes.Buffer
		err := runAtlasOut([]string{"validate", path}, &out)
		if err == nil {
			t.Fatal("expected a non-zero exit for an invalid catalog")
		}
		mustContain(t, err.Error(), "2 problem(s)")
		mustContain(t, out.String(), `entry 1: duplicate slug "dupe"`, "entry 2: slug", "path separator")
	})

	t.Run("a missing file is reported, not treated as empty", func(t *testing.T) {
		err := runAtlasOut([]string{"validate", filepath.Join(t.TempDir(), "nope.json")}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected an error for a missing file")
		}
	})
}

func TestAtlasAdd_installsAndSaysWhatToRunNext(t *testing.T) {
	f := newAtlasFixture(t)
	target := filepath.Join(t.TempDir(), ".sightmap")

	var out bytes.Buffer
	args := append([]string{"add", "square-pos", "--target", target}, f.sourceFlag()...)
	if err := runAtlasOut(args, &out); err != nil {
		t.Fatalf("atlas add: %v", err)
	}
	mustContain(t, out.String(),
		filepath.Join(target, "config.yaml"),
		filepath.Join(target, "views", "checkout.yaml"),
		"Installed square-pos: 2 files",
		"sightmap validate",
	)
	if _, err := os.Stat(filepath.Join(target, "config.yaml")); err != nil {
		t.Errorf("config.yaml: %v", err)
	}
}

// The refusal names the flag-free way out, because there is no --force.
func TestAtlasAdd_refusesAnExistingCorpus(t *testing.T) {
	f := newAtlasFixture(t)
	target := filepath.Join(t.TempDir(), ".sightmap")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "mine.yaml"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	args := append([]string{"add", "square-pos", "--target", target}, f.sourceFlag()...)
	err := runAtlasOut(args, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected the existing corpus to be refused")
	}
	mustContain(t, err.Error(), "already exists and is not empty", "delete it")
	if got, _ := os.ReadFile(filepath.Join(target, "mine.yaml")); string(got) != "mine\n" {
		t.Errorf("mine.yaml = %q, want it untouched", got)
	}
}
