package main

// What belongs here is CLI shape: argument parsing, the flags-after-slug
// re-parse, the formatted output, and the one error the adapter reshapes.
// Everything about the atlas itself — the index schema, the URL layout, the
// validators, the fetch policy, atomic install — is tested in go/atlas.

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const addIndexJSON = `{
  "schema_version": 1,
  "entries": [
    {
      "slug": "square-pos",
      "name": "Square POS",
      "files": [".sightmap/config.yaml", ".sightmap/views/checkout.yaml"]
    },
    {
      "slug": "square-web",
      "files": [".sightmap/config.yaml"]
    }
  ]
}`

// startAtlasServer serves an atlas under the documented <root>/<ref>/… layout:
// the index at /main/index.json, each file at its own path.
func startAtlasServer(t *testing.T, index string, files map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/main/index.json" {
			fmt.Fprint(w, index)
			return
		}
		if body, ok := files[r.URL.Path]; ok {
			io.WriteString(w, body)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// runAddCapture runs runAddOut and returns (stdout, error).
func runAddCapture(args []string) (string, error) {
	var buf bytes.Buffer
	err := runAddOut(args, &buf)
	return buf.String(), err
}

func TestAdd_happyPath(t *testing.T) {
	srv := startAtlasServer(t, addIndexJSON, map[string]string{
		"/main/entries/square-pos/.sightmap/config.yaml":         "version: 1\n",
		"/main/entries/square-pos/.sightmap/views/checkout.yaml": "version: 1\nviews: []\n",
	})
	target := filepath.Join(t.TempDir(), ".sightmap")

	// Flags after the slug, matching the documented invocation shape — the
	// flag package stops at the first positional argument, so `add SLUG
	// --target X` only works because the adapter re-parses.
	out, err := runAddCapture([]string{"square-pos", "--index", srv.URL + "/main/index.json", "--target", target})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, readErr := os.ReadFile(filepath.Join(target, "config.yaml")); readErr != nil || string(got) != "version: 1\n" {
		t.Errorf("config.yaml = %q, %v", got, readErr)
	}
	if _, readErr := os.ReadFile(filepath.Join(target, "views", "checkout.yaml")); readErr != nil {
		t.Errorf("views/checkout.yaml not written: %v", readErr)
	}

	// One line per file, then the summary: slug, name, file count, target,
	// next-step hint.
	for _, want := range []string{
		filepath.Join(target, "config.yaml"),
		filepath.Join(target, "views", "checkout.yaml"),
		"square-pos (Square POS)",
		"2 files",
		target,
		"sightmap validate",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestAdd_singleFileIsNotPluralized(t *testing.T) {
	srv := startAtlasServer(t, addIndexJSON, map[string]string{
		"/main/entries/square-web/.sightmap/config.yaml": "version: 1\n",
	})
	target := filepath.Join(t.TempDir(), ".sightmap")

	out, err := runAddCapture([]string{"square-web", "--index", srv.URL + "/main/index.json", "--target", target})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "1 file:") && !strings.Contains(out, "1 file ") {
		t.Errorf("expected a singular file count, got:\n%s", out)
	}
}

// The library refuses a non-empty target; the CLI is what knows the flag that
// overrides it.
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

	_, err := runAddCapture([]string{"square-web", "--index", srv.URL + "/main/index.json", "--target", target})
	if err == nil {
		t.Fatal("expected an error for a non-empty target")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("expected the error to point at --force, got: %v", err)
	}
	if data, readErr := os.ReadFile(stray); readErr != nil || string(data) != "precious\n" {
		t.Errorf("existing file was touched: %q, %v", data, readErr)
	}
	if _, statErr := os.Stat(filepath.Join(target, "config.yaml")); !os.IsNotExist(statErr) {
		t.Error("config.yaml should not have been written without --force")
	}
}

func TestAdd_forceReportsTheReplacement(t *testing.T) {
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

	out, err := runAddCapture([]string{"square-web", "--force", "--index", srv.URL + "/main/index.json", "--target", target})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data, readErr := os.ReadFile(filepath.Join(target, "config.yaml")); readErr != nil || string(data) != "version: 1\n" {
		t.Errorf("config.yaml = %q, want the fetched content", data)
	}
	if !strings.Contains(out, "replaced") {
		t.Errorf("expected the output to say the target was replaced, got:\n%s", out)
	}
}

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
