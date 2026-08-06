package atlas

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func indexBody(slug string) []byte {
	return []byte(`{"schema_version": 1, "entries": [{"slug": "` + slug + `", "domains": ["` + slug + `.test"]}]}`)
}

// loadIndex runs LoadIndex against srv with the cache pinned to dir.
func loadIndex(t *testing.T, srv *fakeAtlas, dir string, opts IndexOptions) *IndexResult {
	t.Helper()
	opts.URL = srv.indexURL()
	opts.CacheDir = dir
	res, err := LoadIndex(context.Background(), opts)
	if err != nil {
		t.Fatalf("LoadIndex: %v", err)
	}
	return res
}

// A search should not spend a round trip per invocation. An agent runs `find`
// several times while it works out which entry it wants.
func TestLoadIndex_servesAFreshCacheWithoutFetching(t *testing.T) {
	srv := newFakeAtlas(t, map[string][]byte{"/index.json": indexBody("square-pos")})
	dir := t.TempDir()

	first := loadIndex(t, srv, dir, IndexOptions{})
	if first.FromCache {
		t.Error("the first load came from a cache that did not exist")
	}
	second := loadIndex(t, srv, dir, IndexOptions{})
	if !second.FromCache {
		t.Error("the second load re-fetched instead of using the cache")
	}
	if got := len(srv.paths()); got != 1 {
		t.Errorf("fetched %d times, want 1", got)
	}
	if len(second.Index.Entries) != 1 || second.Index.Entries[0].Slug != "square-pos" {
		t.Errorf("cached index = %+v", second.Index.Entries)
	}
	if _, err := os.Stat(filepath.Join(dir, "index.json")); err != nil {
		t.Errorf("cache file: %v", err)
	}
}

// Past the TTL the catalog is re-read, so a corpus published yesterday is
// findable today without anyone clearing a cache.
func TestLoadIndex_refetchesPastTheTTL(t *testing.T) {
	srv := newFakeAtlas(t, map[string][]byte{"/index.json": indexBody("square-pos")})
	dir := t.TempDir()

	now := time.Now()
	loadIndex(t, srv, dir, IndexOptions{Now: func() time.Time { return now }})

	stale := loadIndex(t, srv, dir, IndexOptions{Now: func() time.Time { return now.Add(IndexTTL + time.Minute) }})
	if stale.FromCache {
		t.Error("served a cache older than the TTL")
	}
	if got := len(srv.paths()); got != 2 {
		t.Errorf("fetched %d times, want 2", got)
	}

	fresh := loadIndex(t, srv, dir, IndexOptions{Now: func() time.Time { return now.Add(IndexTTL + 2*time.Minute) }})
	if !fresh.FromCache {
		t.Error("the re-fetched index was not cached")
	}
}

// --refresh is the escape hatch for the window where the TTL is wrong: a
// corpus published minutes ago.
func TestLoadIndex_refreshBustsAFreshCache(t *testing.T) {
	bodies := map[string][]byte{"/index.json": indexBody("square-pos")}
	srv := newFakeAtlas(t, bodies)
	dir := t.TempDir()

	loadIndex(t, srv, dir, IndexOptions{})
	bodies["/index.json"] = indexBody("newly-published")

	res := loadIndex(t, srv, dir, IndexOptions{Refresh: true})
	if res.FromCache {
		t.Fatal("--refresh served the cache")
	}
	if res.Index.Entries[0].Slug != "newly-published" {
		t.Errorf("index = %+v, want the re-fetched entry", res.Index.Entries)
	}
	// And the refreshed bytes replace what was cached.
	if next := loadIndex(t, srv, dir, IndexOptions{}); next.Index.Entries[0].Slug != "newly-published" {
		t.Errorf("cache = %+v, want the refreshed entry", next.Index.Entries)
	}
}

// A cache written from one --index must never answer for another.
func TestLoadIndex_ignoresACacheFromADifferentIndexURL(t *testing.T) {
	first := newFakeAtlas(t, map[string][]byte{"/index.json": indexBody("from-first")})
	second := newFakeAtlas(t, map[string][]byte{"/index.json": indexBody("from-second")})
	dir := t.TempDir()

	loadIndex(t, first, dir, IndexOptions{})
	res := loadIndex(t, second, dir, IndexOptions{})
	if res.FromCache || res.Index.Entries[0].Slug != "from-second" {
		t.Fatalf("index = %+v (cached=%v), want a fresh read of the second atlas", res.Index.Entries, res.FromCache)
	}
}

// The cache is an optimization. Anything wrong with it is a miss, never the
// error the user sees.
func TestLoadIndex_treatsACorruptCacheAsAMiss(t *testing.T) {
	srv := newFakeAtlas(t, map[string][]byte{"/index.json": indexBody("square-pos")})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := loadIndex(t, srv, dir, IndexOptions{})
	if res.FromCache || len(res.Index.Entries) != 1 {
		t.Fatalf("index = %+v (cached=%v), want a fresh fetch", res.Index.Entries, res.FromCache)
	}
}

// A cache holding an index this CLI cannot read must not pin the user to the
// error for a day: the fetch decides, and the fetch is what reports it.
func TestLoadIndex_reportsTheSchemaGateFromTheNetworkNotTheCache(t *testing.T) {
	srv := newFakeAtlas(t, map[string][]byte{"/index.json": []byte(`{"schema_version": 99, "entries": []}`)})
	dir := t.TempDir()

	_, err := LoadIndex(context.Background(), IndexOptions{URL: srv.indexURL(), CacheDir: dir})
	if err == nil {
		t.Fatal("expected the schema gate to refuse the index")
	}
	mustContain(t, err.Error(), "schema_version 99", "upgrade sightmap")
	if _, statErr := os.Stat(filepath.Join(dir, "index.json")); statErr == nil {
		t.Error("an index this CLI cannot read was cached")
	}
}

// A read-only HOME costs a round trip, not a search.
func TestLoadIndex_worksWithNoUsableCacheDirectory(t *testing.T) {
	srv := newFakeAtlas(t, map[string][]byte{"/index.json": indexBody("square-pos")})
	res, err := LoadIndex(context.Background(), IndexOptions{URL: srv.indexURL(), CacheDir: "-"})
	if err != nil {
		t.Fatalf("LoadIndex: %v", err)
	}
	if res.FromCache {
		t.Error("served a cache that is disabled")
	}
}

func TestLoadIndex_reportsAnIndexOutage(t *testing.T) {
	srv := newFakeAtlas(t, map[string][]byte{})
	_, err := LoadIndex(context.Background(), IndexOptions{URL: srv.indexURL(), CacheDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected the missing index to be an error")
	}
	mustContain(t, err.Error(), "fetch atlas index", "404")
}
