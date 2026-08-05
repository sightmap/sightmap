package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// addIndexJSON is a minimal atlas index used by the add tests. It carries
// unknown fields (generated_at, stars, description) on purpose — add must
// ignore them, not choke on them.
const addIndexJSON = `{
  "schema_version": 1,
  "generated_at": "2026-08-01T00:00:00Z",
  "entries": [
    {
      "slug": "square-pos",
      "name": "Square POS",
      "description": "point of sale demo",
      "stars": 12,
      "files": [".sightmap/config.yaml", ".sightmap/views/checkout.yaml"]
    },
    {
      "slug": "pinned-shop",
      "name": "Pinned Shop",
      "commit": "0123456789abcdef0123456789abcdef01234567",
      "files": [".sightmap/config.yaml"]
    },
    {
      "slug": "square-web",
      "name": "Square Web",
      "files": [".sightmap/config.yaml"]
    }
  ]
}`

// startAtlasServer serves index at /index.json and files (keyed by full URL
// path, e.g. "/main/entries/square-pos/.sightmap/config.yaml") mimicking the
// raw.githubusercontent.com layout that atlasRawBase derives for non-GitHub
// hosts.
func startAtlasServer(t *testing.T, index string, files map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, index)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if body, ok := files[r.URL.Path]; ok {
			fmt.Fprint(w, body)
			return
		}
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// runAddCapture runs runAddOut and returns (stdout, error).
func runAddCapture(args []string) (string, error) {
	var buf bytes.Buffer
	err := runAddOut(args, &buf)
	return buf.String(), err
}

// ── happy path ────────────────────────────────────────────────────────────────

func TestAdd_happyPath(t *testing.T) {
	srv := startAtlasServer(t, addIndexJSON, map[string]string{
		"/main/entries/square-pos/.sightmap/config.yaml":         "version: 1\n",
		"/main/entries/square-pos/.sightmap/views/checkout.yaml": "version: 1\nviews: []\n",
	})
	target := filepath.Join(t.TempDir(), ".sightmap")

	// Flags after the slug, matching the documented invocation shape.
	out, err := runAddCapture([]string{"square-pos", "--index", srv.URL + "/index.json", "--target", target})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	config, readErr := os.ReadFile(filepath.Join(target, "config.yaml"))
	if readErr != nil {
		t.Fatalf("config.yaml not written: %v", readErr)
	}
	if string(config) != "version: 1\n" {
		t.Errorf("config.yaml = %q, want %q", config, "version: 1\n")
	}
	view, readErr := os.ReadFile(filepath.Join(target, "views", "checkout.yaml"))
	if readErr != nil {
		t.Fatalf("views/checkout.yaml not written: %v", readErr)
	}
	if string(view) != "version: 1\nviews: []\n" {
		t.Errorf("views/checkout.yaml = %q, want %q", view, "version: 1\nviews: []\n")
	}

	// Summary: slug, file count, target, next-step hint.
	for _, want := range []string{"square-pos", "2 files", target, "sightmap validate"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestAdd_commitPinsFetch(t *testing.T) {
	// The file exists ONLY under the entry's commit, not under main — so a
	// success proves the pinned sha was used as the ref.
	srv := startAtlasServer(t, addIndexJSON, map[string]string{
		"/0123456789abcdef0123456789abcdef01234567/entries/pinned-shop/.sightmap/config.yaml": "version: 1\n",
	})
	target := filepath.Join(t.TempDir(), ".sightmap")

	if _, err := runAddCapture([]string{"pinned-shop", "--index", srv.URL + "/index.json", "--target", target}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "config.yaml")); err != nil {
		t.Errorf("config.yaml not written: %v", err)
	}
}

// ── slug miss ─────────────────────────────────────────────────────────────────

func TestAdd_slugMissSuggests(t *testing.T) {
	srv := startAtlasServer(t, addIndexJSON, nil)

	_, err := runAddCapture([]string{"square", "--index", srv.URL + "/index.json", "--target", filepath.Join(t.TempDir(), ".sightmap")})
	if err == nil {
		t.Fatal("expected an error for an unknown slug")
	}
	for _, want := range []string{`"square"`, "square-pos", "square-web"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected %q in error, got: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "pinned-shop") {
		t.Errorf("unrelated slug suggested: %v", err)
	}
}

func TestAdd_slugMissNoNeighbours(t *testing.T) {
	srv := startAtlasServer(t, addIndexJSON, nil)

	_, err := runAddCapture([]string{"zzz", "--index", srv.URL + "/index.json", "--target", filepath.Join(t.TempDir(), ".sightmap")})
	if err == nil {
		t.Fatal("expected an error for an unknown slug")
	}
	if !strings.Contains(err.Error(), `"zzz"`) {
		t.Errorf("expected the slug in the error, got: %v", err)
	}
}

func TestClosestAtlasSlugs_capsAtFive(t *testing.T) {
	var entries []atlasEntry
	for i := 0; i < 8; i++ {
		entries = append(entries, atlasEntry{Slug: fmt.Sprintf("shop-%d", i)})
	}
	got := closestAtlasSlugs("shop", entries)
	if len(got) != 5 {
		t.Errorf("expected 5 suggestions, got %d: %v", len(got), got)
	}
}

// ── unsafe entries fail closed ────────────────────────────────────────────────

func TestAdd_rejectsUnsafeFilePaths(t *testing.T) {
	cases := []struct{ name, file string }{
		{"traversal", ".sightmap/../evil.yaml"},
		{"absolute", "/etc/passwd"},
		{"backslash", `.sightmap\evil.yaml`},
		{"outside-sightmap", "README.md"},
		{"empty-segment", ".sightmap//x.yaml"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			index := fmt.Sprintf(`{"schema_version": 1, "entries": [{"slug": "bad", "files": [%q]}]}`, tc.file)
			srv := startAtlasServer(t, index, nil)
			target := filepath.Join(t.TempDir(), ".sightmap")

			_, err := runAddCapture([]string{"bad", "--index", srv.URL + "/index.json", "--target", target})
			if err == nil {
				t.Fatalf("expected an error for unsafe file path %q", tc.file)
			}
			if !strings.Contains(err.Error(), "refusing to install") {
				t.Errorf("expected a refusing-to-install error, got: %v", err)
			}
			// Fail closed: nothing may have been written.
			if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
				t.Errorf("target %s should not exist after a rejected entry", target)
			}
		})
	}
}

func TestAdd_rejectsMalformedCommit(t *testing.T) {
	index := `{"schema_version": 1, "entries": [{"slug": "bad", "commit": "not-a-sha", "files": [".sightmap/config.yaml"]}]}`
	srv := startAtlasServer(t, index, nil)

	_, err := runAddCapture([]string{"bad", "--index", srv.URL + "/index.json", "--target", filepath.Join(t.TempDir(), ".sightmap")})
	if err == nil {
		t.Fatal("expected an error for a malformed commit")
	}
	if !strings.Contains(err.Error(), "commit") {
		t.Errorf("expected the commit in the error, got: %v", err)
	}
}

// ── existing target ───────────────────────────────────────────────────────────

func TestAdd_refusesNonEmptyTarget(t *testing.T) {
	srv := startAtlasServer(t, addIndexJSON, map[string]string{
		"/main/entries/square-web/.sightmap/config.yaml": "version: 1\n",
	})
	target := filepath.Join(t.TempDir(), ".sightmap")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	stray := filepath.Join(target, "hand-authored.yaml")
	if err := os.WriteFile(stray, []byte("precious\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := runAddCapture([]string{"square-web", "--index", srv.URL + "/index.json", "--target", target})
	if err == nil {
		t.Fatal("expected an error for a non-empty target")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("expected the error to point at --force, got: %v", err)
	}
	data, readErr := os.ReadFile(stray)
	if readErr != nil || string(data) != "precious\n" {
		t.Errorf("existing file was touched: %q, %v", data, readErr)
	}
	if _, statErr := os.Stat(filepath.Join(target, "config.yaml")); !os.IsNotExist(statErr) {
		t.Error("config.yaml should not have been written without --force")
	}
}

func TestAdd_forceOverwrites(t *testing.T) {
	srv := startAtlasServer(t, addIndexJSON, map[string]string{
		"/main/entries/square-web/.sightmap/config.yaml": "version: 1\n",
	})
	target := filepath.Join(t.TempDir(), ".sightmap")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "config.yaml"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := runAddCapture([]string{"square-web", "--force", "--index", srv.URL + "/index.json", "--target", target}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, readErr := os.ReadFile(filepath.Join(target, "config.yaml"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "version: 1\n" {
		t.Errorf("config.yaml = %q, want overwritten content", data)
	}
}

// ── network and URL errors ────────────────────────────────────────────────────

func TestAdd_httpsOnlyForRemoteHosts(t *testing.T) {
	_, err := runAddCapture([]string{"square-pos", "--index", "http://example.com/index.json", "--target", filepath.Join(t.TempDir(), ".sightmap")})
	if err == nil {
		t.Fatal("expected an error for a plain-http remote index")
	}
	if !strings.Contains(err.Error(), "non-HTTPS") {
		t.Errorf("expected a non-HTTPS refusal, got: %v", err)
	}
}

func TestAdd_indexFetchErrorNamesURLAndStatus(t *testing.T) {
	srv := startAtlasServer(t, addIndexJSON, nil)
	missing := srv.URL + "/nope.json"

	_, err := runAddCapture([]string{"square-pos", "--index", missing, "--target", filepath.Join(t.TempDir(), ".sightmap")})
	if err == nil {
		t.Fatal("expected an error for a 404 index")
	}
	for _, want := range []string{missing, "404"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected %q in error, got: %v", want, err)
		}
	}
}

func TestAdd_fileFetchErrorNamesURLAndStatus(t *testing.T) {
	// Index is fine, but the entry's second file is not served.
	srv := startAtlasServer(t, addIndexJSON, map[string]string{
		"/main/entries/square-pos/.sightmap/config.yaml": "version: 1\n",
	})
	target := filepath.Join(t.TempDir(), ".sightmap")

	_, err := runAddCapture([]string{"square-pos", "--index", srv.URL + "/index.json", "--target", target})
	if err == nil {
		t.Fatal("expected an error for a missing corpus file")
	}
	for _, want := range []string{"/main/entries/square-pos/.sightmap/views/checkout.yaml", "404"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected %q in error, got: %v", want, err)
		}
	}
	// All files are fetched before any is written, so nothing landed.
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Errorf("target %s should not exist after a failed fetch", target)
	}
}

// ── arguments ─────────────────────────────────────────────────────────────────

func TestAdd_missingSlugError(t *testing.T) {
	_, err := runAddCapture([]string{})
	if err == nil {
		t.Fatal("expected an error when no slug is given")
	}
	if !strings.Contains(err.Error(), "SLUG") {
		t.Errorf("expected 'SLUG' in error, got: %v", err)
	}
}

func TestAdd_extraArgumentError(t *testing.T) {
	_, err := runAddCapture([]string{"square-pos", "extra"})
	if err == nil {
		t.Fatal("expected an error for a second positional argument")
	}
	if !strings.Contains(err.Error(), "extra") {
		t.Errorf("expected the stray argument in the error, got: %v", err)
	}
}

// ── atlasRawBase ──────────────────────────────────────────────────────────────

func TestAtlasRawBase_defaultIndex(t *testing.T) {
	base, err := atlasRawBase(defaultAtlasIndexURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if base != "https://raw.githubusercontent.com/sightmap/atlas" {
		t.Errorf("base = %q, want https://raw.githubusercontent.com/sightmap/atlas", base)
	}
}

func TestAtlasRawBase_localhostIndex(t *testing.T) {
	base, err := atlasRawBase("http://127.0.0.1:8080/index.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if base != "http://127.0.0.1:8080" {
		t.Errorf("base = %q, want http://127.0.0.1:8080", base)
	}
}
