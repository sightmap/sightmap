package atlas

import (
	"strings"
	"testing"
)

// One layout rule, <root>/<ref>/index.json, has to describe every host the
// atlas can be served from: GitHub raw (both URL shapes it hands out), a
// GitHub Enterprise raw host, and a plain copy of what either serves.
func TestParseSource_layout(t *testing.T) {
	cases := []struct {
		name     string
		indexURL string
		root     string
		ref      string
	}{
		{
			name:     "github-raw-default",
			indexURL: DefaultIndexURL,
			root:     "https://raw.githubusercontent.com/sightmap/atlas",
			ref:      "main",
		},
		{
			name:     "github-raw-refs-heads",
			indexURL: "https://raw.githubusercontent.com/sightmap/atlas/refs/heads/main/index.json",
			root:     "https://raw.githubusercontent.com/sightmap/atlas",
			ref:      "refs/heads/main",
		},
		{
			name:     "github-raw-refs-tags",
			indexURL: "https://raw.githubusercontent.com/sightmap/atlas/refs/tags/v1.2.0/index.json",
			root:     "https://raw.githubusercontent.com/sightmap/atlas",
			ref:      "refs/tags/v1.2.0",
		},
		{
			name:     "github-raw-branch-with-slashes",
			indexURL: "https://raw.githubusercontent.com/sightmap/atlas/refs/heads/release/2026-08/index.json",
			root:     "https://raw.githubusercontent.com/sightmap/atlas",
			ref:      "refs/heads/release/2026-08",
		},
		{
			name:     "github-raw-sha",
			indexURL: "https://raw.githubusercontent.com/sightmap/atlas/0123456789abcdef0123456789abcdef01234567/index.json",
			root:     "https://raw.githubusercontent.com/sightmap/atlas",
			ref:      "0123456789abcdef0123456789abcdef01234567",
		},
		{
			name:     "github-enterprise-raw",
			indexURL: "https://ghe.example.com/raw/platform/atlas/main/index.json",
			root:     "https://ghe.example.com/raw/platform/atlas",
			ref:      "main",
		},
		{
			name:     "plain-mirror-copy",
			indexURL: "https://mirror.example.com/sightmap-atlas/main/index.json",
			root:     "https://mirror.example.com/sightmap-atlas",
			ref:      "main",
		},
		{
			name:     "localhost-test-server",
			indexURL: "http://127.0.0.1:8080/main/index.json",
			root:     "http://127.0.0.1:8080",
			ref:      "main",
		},
		{
			name:     "non-index-json-filename",
			indexURL: "https://mirror.example.com/atlas/main/atlas-index.json",
			root:     "https://mirror.example.com/atlas",
			ref:      "main",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, err := ParseSource(tc.indexURL)
			if err != nil {
				t.Fatalf("ParseSource(%q): %v", tc.indexURL, err)
			}
			if src.Root != tc.root {
				t.Errorf("Root = %q, want %q", src.Root, tc.root)
			}
			if src.Ref != tc.ref {
				t.Errorf("Ref = %q, want %q", src.Ref, tc.ref)
			}
			// The index itself must be reachable under the layout the source
			// describes — that is what makes the rule coherent rather than a
			// pair of per-host special cases.
			if got := src.Root + "/" + src.Ref; !strings.HasPrefix(tc.indexURL, got+"/") {
				t.Errorf("%s is not <root>/<ref>/... of %q", got, tc.indexURL)
			}
		})
	}
}

func TestParseSource_rejects(t *testing.T) {
	cases := []struct{ name, indexURL, want string }{
		{"plain-http-remote", "http://example.com/main/index.json", "non-HTTPS"},
		{"ftp", "ftp://example.com/main/index.json", "non-HTTPS"},
		{"no-ref-segment", "https://mirror.example.com/index.json", "no ref segment"},
		{"directory", "https://mirror.example.com/atlas/main/", "index file"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSource(tc.indexURL)
			if err == nil {
				t.Fatalf("ParseSource(%q) = nil error", tc.indexURL)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestSourceFileURL(t *testing.T) {
	src, err := ParseSource(DefaultIndexURL)
	if err != nil {
		t.Fatal(err)
	}
	got := src.FileURL("main", "square-pos", ".sightmap/views/checkout.yaml")
	want := "https://raw.githubusercontent.com/sightmap/atlas/main/entries/square-pos/.sightmap/views/checkout.yaml"
	if got != want {
		t.Errorf("FileURL = %q, want %q", got, want)
	}
	// A commit-pinned entry replaces only the ref, so the fully-qualified ref
	// shape resolves to a real URL instead of doubling up on refs/heads.
	refsSrc, err := ParseSource("https://raw.githubusercontent.com/sightmap/atlas/refs/heads/main/index.json")
	if err != nil {
		t.Fatal(err)
	}
	sha := "0123456789abcdef0123456789abcdef01234567"
	got = refsSrc.FileURL(sha, "square-pos", ".sightmap/config.yaml")
	want = "https://raw.githubusercontent.com/sightmap/atlas/" + sha + "/entries/square-pos/.sightmap/config.yaml"
	if got != want {
		t.Errorf("pinned FileURL = %q, want %q", got, want)
	}
	// Spaces and other URL-unsafe bytes are escaped per segment, separators kept.
	got = src.FileURL("main", "a b", ".sightmap/views/my view.yaml")
	want = "https://raw.githubusercontent.com/sightmap/atlas/main/entries/a%20b/.sightmap/views/my%20view.yaml"
	if got != want {
		t.Errorf("escaped FileURL = %q, want %q", got, want)
	}
}

func TestSourceRefFor(t *testing.T) {
	src, err := ParseSource("https://raw.githubusercontent.com/sightmap/atlas/next/index.json")
	if err != nil {
		t.Fatal(err)
	}
	sha := "0123456789abcdef0123456789abcdef01234567"
	if got := src.RefFor(&Entry{Slug: "a", Commit: sha}); got != sha {
		t.Errorf("pinned entry ref = %q, want the commit", got)
	}
	// An unpinned entry follows the index's own ref — never a hardcoded "main".
	if got := src.RefFor(&Entry{Slug: "a"}); got != "next" {
		t.Errorf("unpinned entry ref = %q, want %q", got, "next")
	}
}
