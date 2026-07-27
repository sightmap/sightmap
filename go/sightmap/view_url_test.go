package sightmap_test

import (
	"path/filepath"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// TestViewURL_PerViewAndFileFallback verifies a per-view url wins and a
// file-level url is the default for views that omit their own.
func TestViewURL_PerViewAndFileFallback(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "views.yaml"), `
version: 1
url: https://example.com/default
views:
  - name: Home
    route: /
    url: https://example.com/
  - name: Search
    route: /search
`)
	c, err := sightmap.DirLoader(dir).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v := c.ViewByName("Home"); v == nil || v.URL != "https://example.com/" {
		got := "<nil>"
		if v != nil {
			got = v.URL
		}
		t.Errorf("Home.URL = %q, want the per-view url", got)
	}
	if v := c.ViewByName("Search"); v == nil || v.URL != "https://example.com/default" {
		got := "<nil>"
		if v != nil {
			got = v.URL
		}
		t.Errorf("Search.URL = %q, want the file-level default", got)
	}
}
