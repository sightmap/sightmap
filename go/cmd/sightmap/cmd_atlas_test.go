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

// atlasFixture is a local atlas: one index and one archive, served over
// loopback so the transport policy's http exception applies.
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
	// Every case gets its own HOME so the index cache never touches the
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

func findOut(t *testing.T, f *atlasFixture, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := runAtlasFindOut(append(args, f.indexFlag()...), &out); err != nil {
		t.Fatalf("atlas find %v: %v", args, err)
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

// The handoff is the point of the verb: a result carries the command that
// installs it, so an agent never has to assemble one.
func TestAtlasFind_printsAnInstallCommandWithEachHit(t *testing.T) {
	f := newAtlasFixture(t)
	out := findOut(t, f, "squareup.com")
	mustContain(t, out,
		"square-pos  Square POS",
		"Point-of-sale checkout and order history.",
		"squareup.com, app.squareup.com",
		"payments, commerce",
		"12 views, 48 components, 23 requests",
		"verified 2026-07-14",
		"sightmap atlas add square-pos",
	)
	if i, j := strings.Index(out, "square-pos"), strings.Index(out, "acme-shop"); j != -1 && i > j {
		t.Errorf("acme-shop outranked square-pos for a squareup.com query:\n%s", out)
	}
}

// An empty result is a successful search, not a failed command: the agent
// asked a question and got an answer.
func TestAtlasFind_exitsZeroWhenNothingMatches(t *testing.T) {
	f := newAtlasFixture(t)
	var out bytes.Buffer
	err := runAtlasFindOut(append([]string{"stripe.com"}, f.indexFlag()...), &out)
	if err != nil {
		t.Fatalf("a search with no results returned an error: %v", err)
	}
	mustContain(t, out.String(), `No atlas entry matches "stripe.com"`, "sightmap init")
}

func TestAtlasList_browsesTheWholeCatalog(t *testing.T) {
	f := newAtlasFixture(t)
	var out bytes.Buffer
	if err := runAtlasListOut(f.indexFlag(), &out); err != nil {
		t.Fatalf("atlas list: %v", err)
	}
	mustContain(t, out.String(), "acme-shop", "square-pos", "2 matches")
}

func TestAtlasList_filtersByCategory(t *testing.T) {
	f := newAtlasFixture(t)
	var out bytes.Buffer
	if err := runAtlasListOut(append([]string{"--category", "payments"}, f.indexFlag()...), &out); err != nil {
		t.Fatalf("atlas list: %v", err)
	}
	got := out.String()
	mustContain(t, got, "square-pos", "1 match")
	if strings.Contains(got, "acme-shop") {
		t.Errorf("--category payments listed acme-shop:\n%s", got)
	}
}

func TestAtlasFind_limitsResultsAndSaysHowManyThereAre(t *testing.T) {
	f := newAtlasFixture(t)
	var out bytes.Buffer
	if err := runAtlasFindOut(append([]string{"e", "--limit", "1"}, f.indexFlag()...), &out); err != nil {
		t.Fatalf("atlas find: %v", err)
	}
	mustContain(t, out.String(), "1 of 2 matches", "--limit")
}

func TestAtlasFind_jsonCarriesTheInstallCommand(t *testing.T) {
	f := newAtlasFixture(t)
	out := findOut(t, f, "squareup.com", "--json")
	var doc struct {
		Query   string `json:"query"`
		Total   int    `json:"total"`
		Results []struct {
			Slug       string   `json:"slug"`
			Domains    []string `json:"domains"`
			Views      int      `json:"views"`
			Components int      `json:"components"`
			Requests   int      `json:"requests"`
			MatchedOn  string   `json:"matched_on"`
			Install    string   `json:"install"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("--json is not valid JSON: %v\n%s", err, out)
	}
	if doc.Query != "squareup.com" || doc.Total != 1 || len(doc.Results) != 1 {
		t.Fatalf("doc = %+v", doc)
	}
	r := doc.Results[0]
	if r.Slug != "square-pos" || r.Install != "sightmap atlas add square-pos" || r.MatchedOn != "exact domain" {
		t.Errorf("result = %+v", r)
	}
	// The same three counts the text output and the gallery card show.
	if r.Views != 12 || r.Components != 48 || r.Requests != 23 {
		t.Errorf("stats = %d views, %d components, %d requests; want 12/48/23", r.Views, r.Components, r.Requests)
	}
}

// Names, descriptions, domains, and categories come straight off the index,
// which is more atlas-authored text than an install ever printed.
func TestAtlasFind_escapesHostileIndexText(t *testing.T) {
	f := newAtlasFixture(t)
	f.index = `{"schema_version": 1, "entries": [{"slug": "evil", "name": "Evil\u001b]0;pwned\u0007", "description": "drop\u001b[2K", "domains": ["evil.test"]}]}`
	out := findOut(t, f, "evil.test")
	if strings.ContainsAny(out, "\x1b\x07") {
		t.Fatalf("output carried a raw control character:\n%q", out)
	}
	mustContain(t, out, `Evil\x1b]0;pwned\x07`, `drop\x1b[2K`)
}

// A slug the index cannot install must not be offered with an install command.
func TestAtlasFind_dropsEntriesThatCouldNotBeInstalled(t *testing.T) {
	f := newAtlasFixture(t)
	f.index = `{"schema_version": 1, "entries": [{"slug": "../../etc/passwd", "name": "traversal", "domains": ["evil.test"]}]}`
	var out bytes.Buffer
	if err := runAtlasFindOut(append([]string{"evil.test"}, f.indexFlag()...), &out); err != nil {
		t.Fatalf("atlas find: %v", err)
	}
	if strings.Contains(out.String(), "etc/passwd") {
		t.Fatalf("an uninstallable slug was offered:\n%s", out.String())
	}
}

// A schema bump has to say what to do about it, wherever it is read from.
func TestAtlasFind_reportsAFutureIndexSchema(t *testing.T) {
	f := newAtlasFixture(t)
	f.index = `{"schema_version": 99, "entries": []}`
	var out bytes.Buffer
	err := runAtlasFindOut(append([]string{"anything"}, f.indexFlag()...), &out)
	if err == nil {
		t.Fatal("expected the schema gate to refuse the index")
	}
	mustContain(t, err.Error(), "schema_version 99", "upgrade sightmap")
}

func TestAtlasFind_needsAQuery(t *testing.T) {
	var out bytes.Buffer
	err := runAtlasFindOut(nil, &out)
	if err == nil {
		t.Fatal("expected a missing-query refusal")
	}
	mustContain(t, err.Error(), "expected a QUERY argument", "sightmap atlas list")
}

// ── add ───────────────────────────────────────────────────────────────────────

func TestAtlasAdd_installsAndSaysWhatToRunNext(t *testing.T) {
	f := newAtlasFixture(t)
	dir := t.TempDir()
	target := filepath.Join(dir, ".sightmap")

	var out bytes.Buffer
	args := append([]string{"square-pos", "--target", target}, f.sourceFlag()...)
	if err := runAtlasAddOut(args, &out); err != nil {
		t.Fatalf("atlas add: %v", err)
	}
	mustContain(t, out.String(), "config.yaml", "views/checkout.yaml", "Installed square-pos: 2 files", "sightmap validate")
	if _, err := os.Stat(filepath.Join(target, "views", "checkout.yaml")); err != nil {
		t.Errorf("view file: %v", err)
	}
}

// Flags have to work on either side of the slug: an agent copying an install
// command off a gallery page appends --target to it.
func TestAtlasAdd_acceptsFlagsAfterTheSlug(t *testing.T) {
	f := newAtlasFixture(t)
	target := filepath.Join(t.TempDir(), "vendor-map")
	args := append(append(f.sourceFlag(), "square-pos"), "--target", target)

	var out bytes.Buffer
	if err := runAtlasAddOut(args, &out); err != nil {
		t.Fatalf("atlas add: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "config.yaml")); err != nil {
		t.Errorf("config.yaml: %v", err)
	}
}

func TestAtlasAdd_needsASlug(t *testing.T) {
	var out bytes.Buffer
	err := runAtlasAddOut(nil, &out)
	if err == nil {
		t.Fatal("expected a missing-slug refusal")
	}
	mustContain(t, err.Error(), "expected a SLUG argument")
}

func TestAtlasAdd_refusesAnExistingCorpus(t *testing.T) {
	f := newAtlasFixture(t)
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "components.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	args := append([]string{"square-pos", "--target", target}, f.sourceFlag()...)
	err := runAtlasAddOut(args, &out)
	if err == nil {
		t.Fatal("expected a refusal")
	}
	mustContain(t, err.Error(), "already exists and is not empty", "delete it")
	if out.Len() != 0 {
		t.Errorf("a refused install printed %q", out.String())
	}
}

// ── dispatch ──────────────────────────────────────────────────────────────────

func TestRunAtlas_reportsAnUnknownSubcommand(t *testing.T) {
	err := runAtlas([]string{"instal"})
	if err == nil {
		t.Fatal("expected an unknown-subcommand error")
	}
	mustContain(t, err.Error(), `unknown subcommand "instal"`)
}
