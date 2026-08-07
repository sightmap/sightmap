package atlas

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ── transport policy ──────────────────────────────────────────────────────────

// Plain HTTP must be refused on the URL a caller hands in and on every redirect
// hop. The first hop matters because --index and --source are untrusted input;
// the hops matter because without a per-hop check, "302 Location: http://…"
// installs attacker bytes over a connection the caller believed was TLS.
func TestFetch_transportPolicy(t *testing.T) {
	// redirectTo serves a chain that ends at dest after hops redirects.
	redirectTo := func(t *testing.T, hops int, dest func(srv *httptest.Server) string) string {
		var srv *httptest.Server
		srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := 0
			fmt.Sscanf(r.URL.Path, "/hop/%d", &n)
			if n < hops {
				http.Redirect(w, r, fmt.Sprintf("%s/hop/%d", srv.URL, n+1), http.StatusFound)
				return
			}
			if to := dest(srv); to != "" {
				http.Redirect(w, r, to, http.StatusFound)
				return
			}
			w.Write([]byte("arrived"))
		}))
		t.Cleanup(srv.Close)
		return srv.URL + "/hop/0"
	}
	toPlainHTTP := func(*httptest.Server) string { return "http://atlas.example.com/evil.tar.gz" }
	stayPut := func(*httptest.Server) string { return "" }

	for _, tc := range []struct {
		name    string
		url     func(t *testing.T) string
		wantErr []string
	}{{
		name:    "plain http on the first hop",
		url:     func(*testing.T) string { return "http://atlas.example.com/index.json" },
		wantErr: []string{"refusing non-HTTPS URL", "http://atlas.example.com/index.json", "localhost"},
	}, {
		name:    "a downgrade on the first redirect",
		url:     func(t *testing.T) string { return redirectTo(t, 0, toPlainHTTP) },
		wantErr: []string{"redirected to a URL the atlas policy refuses", "http://atlas.example.com/evil.tar.gz"},
	}, {
		// A chain that starts honestly and turns late is the interesting case:
		// the check has to run on every hop, not only the first.
		name:    "a downgrade late in the chain",
		url:     func(t *testing.T) string { return redirectTo(t, 3, toPlainHTTP) },
		wantErr: []string{"redirected to a URL the atlas policy refuses"},
	}, {
		name:    "an endless redirect chain",
		url:     func(t *testing.T) string { return redirectTo(t, 99, stayPut) },
		wantErr: []string{fmt.Sprintf("stopped after %d redirects", maxRedirects)},
	}, {
		// The refusal is about plaintext reaching a public host, not about
		// redirects: a loopback mirror that redirects within itself still works.
		name: "a loopback redirect is followed",
		url:  func(t *testing.T) string { return redirectTo(t, 2, stayPut) },
	}} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := fetch(context.Background(), tc.url(t))
			if len(tc.wantErr) == 0 {
				if err != nil {
					t.Fatalf("fetch: %v", err)
				}
				if string(body) != "arrived" {
					t.Errorf("body = %q, want the final hop's body", body)
				}
				return
			}
			if err == nil {
				t.Fatal("expected a refusal")
			}
			mustContain(t, err.Error(), tc.wantErr...)
		})
	}
}

func TestFetch_refusesAnOversizeBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, maxFetchBytes+1))
	}))
	defer srv.Close()

	_, err := fetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected the body to be refused")
	}
	mustContain(t, err.Error(), "exceeds the", "limit")
}

// A 404 has to be distinguishable from a network failure without matching on
// message text: Install turns it into "no corpus published for this slug".
func TestFetch_reportsANon200AsATypedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := fetch(context.Background(), srv.URL+"/entries/missing.tar.gz")
	var httpErr *httpError
	if !errors.As(err, &httpErr) {
		t.Fatalf("fetch err = %v, want an *httpError", err)
	}
	if httpErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404", httpErr.StatusCode)
	}
}

// A hostile URL reaches the terminal through the error, so it is escaped there
// the same as index text.
func TestFetch_escapesAHostileURLInItsRefusal(t *testing.T) {
	_, err := fetch(context.Background(), "http://evil.test/\x1b[2Kwiped")
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if strings.Contains(err.Error(), "\x1b") {
		t.Errorf("err = %q, want the escape rendered", err)
	}
}

// ── index cache ───────────────────────────────────────────────────────────────

func indexBody(slug string) []byte {
	return []byte(`{"schema_version": 1, "entries": [{"slug": "` + slug + `", "domains": ["` + slug + `.test"]}]}`)
}

// loadIndex runs LoadIndex against srv with the cache pinned to dir.
func loadIndex(t *testing.T, srv *fakeAtlas, dir string, opts IndexOptions) *IndexResult {
	t.Helper()
	opts.URL, opts.CacheDir = srv.indexURL(), dir
	res, err := LoadIndex(context.Background(), opts)
	if err != nil {
		t.Fatalf("LoadIndex: %v", err)
	}
	return res
}

// A search should not spend a round trip per invocation: an agent runs `find`
// several times while it narrows down which entry it wants.
func TestLoadIndex_servesAFreshCacheWithoutFetching(t *testing.T) {
	srv := newFakeAtlas(t, map[string][]byte{"/index.json": indexBody("square-pos")})
	dir := t.TempDir()

	if first := loadIndex(t, srv, dir, IndexOptions{}); first.FromCache {
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
}

// Every one of these has to re-fetch, or a corpus published yesterday stays
// invisible today and no flag brings it back.
func TestLoadIndex_cacheMisses(t *testing.T) {
	for _, tc := range []struct {
		name string
		// setup primes the cache and returns the options for the second load.
		setup func(t *testing.T, dir string) IndexOptions
	}{{
		name: "past the TTL",
		setup: func(t *testing.T, dir string) IndexOptions {
			return IndexOptions{Now: func() time.Time { return time.Now().Add(IndexTTL + time.Minute) }}
		},
	}, {
		name: "--refresh on a fresh cache",
		setup: func(t *testing.T, dir string) IndexOptions {
			return IndexOptions{Refresh: true}
		},
	}, {
		name: "a corrupt cache file",
		setup: func(t *testing.T, dir string) IndexOptions {
			if err := os.WriteFile(filepath.Join(dir, "index.json"), []byte("{not json"), 0o644); err != nil {
				t.Fatal(err)
			}
			return IndexOptions{}
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newFakeAtlas(t, map[string][]byte{"/index.json": indexBody("square-pos")})
			dir := t.TempDir()
			loadIndex(t, srv, dir, IndexOptions{})
			loadIndex(t, srv, dir, tc.setup(t, dir))
			if got := len(srv.paths()); got != 2 {
				t.Errorf("fetched %d times, want 2 (the cache should have missed)", got)
			}
		})
	}
}

// A cache written for one catalog must not answer for another, or --index
// against a mirror silently returns the default catalog's entries.
func TestLoadIndex_ignoresACacheFromADifferentIndexURL(t *testing.T) {
	dir := t.TempDir()
	first := newFakeAtlas(t, map[string][]byte{"/index.json": indexBody("square-pos")})
	loadIndex(t, first, dir, IndexOptions{})

	second := newFakeAtlas(t, map[string][]byte{"/index.json": indexBody("acme-shop")})
	res := loadIndex(t, second, dir, IndexOptions{})
	if res.FromCache || res.Index.Entries[0].Slug != "acme-shop" {
		t.Errorf("got %+v from cache=%v, want a fresh fetch of acme-shop", res.Index.Entries, res.FromCache)
	}
}

// A cache that cannot be written costs a round trip, not a search.
func TestLoadIndex_worksWithNoUsableCacheDirectory(t *testing.T) {
	srv := newFakeAtlas(t, map[string][]byte{"/index.json": indexBody("square-pos")})
	blocked := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if res := loadIndex(t, srv, blocked, IndexOptions{}); res.FromCache {
			t.Error("served from a cache that could not have been written")
		}
	}
}

// An outage and a future schema both have to reach the caller rather than being
// swallowed into an empty result that reads as "the atlas has nothing".
func TestLoadIndex_reportsFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body, wantErr string
		status              int
	}{
		{name: "an outage", status: http.StatusServiceUnavailable, wantErr: "fetch atlas index"},
		{name: "a future schema", body: `{"schema_version": 99}`, wantErr: "upgrade sightmap"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.status != 0 {
					http.Error(w, "down", tc.status)
					return
				}
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			_, err := LoadIndex(context.Background(), IndexOptions{URL: srv.URL, CacheDir: t.TempDir()})
			if err == nil {
				t.Fatal("expected an error")
			}
			mustContain(t, err.Error(), tc.wantErr)
		})
	}
}
