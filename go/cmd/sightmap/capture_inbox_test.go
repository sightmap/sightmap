package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sightmap/sightmap/go/observe"
	"github.com/sightmap/sightmap/go/sightmap"
	"github.com/sightmap/sightmap/go/viewset"
)

// Two captures in the same second must not overwrite each other: the inbox is
// the only copy of a page someone chose to keep. Reserving a name must claim
// it on disk, so a second reservation (from any process) skips it.
func TestReserveInboxPathNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 20, 1, 2, 3, 0, time.UTC)
	first, err := reserveInboxPath(dir, now)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "inbox", "20261020T010203Z.snap"); first != want {
		t.Fatalf("first path %s, want %s", first, want)
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatalf("reservation did not create %s: %v", first, err)
	}
	second, err := reserveInboxPath(dir, now)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "inbox", "20261020T010203Z-2.snap"); second != want {
		t.Errorf("second path %s, want %s", second, want)
	}
}

// Concurrent reservations for the same stamp (the daemon and a CLI capture
// landing in one second) must each get their own name.
func TestReserveInboxPathConcurrent(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 20, 1, 2, 3, 0, time.UTC)
	const n = 16
	paths := make([]string, n)
	var wg sync.WaitGroup
	for i := range paths {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p, err := reserveInboxPath(dir, now)
			if err != nil {
				t.Error(err)
			}
			paths[i] = p
		}(i)
	}
	wg.Wait()
	seen := map[string]bool{}
	for _, p := range paths {
		if seen[p] {
			t.Errorf("%s reserved twice", p)
		}
		seen[p] = true
	}
	if len(seen) != n {
		t.Errorf("%d distinct paths, want %d", len(seen), n)
	}
}

// pageResult is an observed page small enough to write in a test.
func pageResult() *observe.Result {
	return &observe.Result{
		URL:  "https://a.test/x",
		Root: &sightmap.ComponentNode{Id: "1", Role: "main", Name: "page"},
	}
}

// The .snap is the capture: its tree sibling failing is a warning (the
// regular capture path's behaviour), never a half-reported capture.
func TestWriteInboxCaptureTreeFailureIsWarning(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 20, 1, 2, 3, 0, time.UTC)
	snap := filepath.Join(dir, "inbox", "20261020T010203Z.snap")
	// A directory where writeTreeJSON stages its file makes the tree write fail.
	if err := os.MkdirAll(viewset.TreePath(snap)+".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := writeInboxCapture(dir, now, pageResult(), observe.FormatOpts{})
	if err != nil {
		t.Fatalf("writeInboxCapture: %v", err)
	}
	if got != snap {
		t.Fatalf("path %s, want %s", got, snap)
	}
	data, err := os.ReadFile(snap)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "url: https://a.test/x\n") {
		t.Errorf(".snap does not start with the url header:\n%s", data)
	}
	if _, err := os.Stat(viewset.TreePath(snap)); err == nil {
		t.Errorf("tree file written despite the staged failure")
	}
}

// A .snap that cannot be written leaves nothing in the inbox: the reserved
// name is released rather than left as an empty capture.
func TestWriteInboxCaptureSnapFailureLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 20, 1, 2, 3, 0, time.UTC)
	snap := filepath.Join(dir, "inbox", "20261020T010203Z.snap")
	if err := os.MkdirAll(snap+".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := writeInboxCapture(dir, now, pageResult(), observe.FormatOpts{}); err == nil {
		t.Fatal("writeInboxCapture succeeded with an unwritable .snap")
	}
	if _, err := os.Stat(snap); !os.IsNotExist(err) {
		t.Errorf("reservation %s left behind (stat: %v)", snap, err)
	}
}

// fakeTabs serves a CDP /json/list with the given page URLs.
func fakeTabs(t *testing.T, urls ...string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var out []map[string]string
		for i, u := range urls {
			out = append(out, map[string]string{"id": "tab-" + string(rune('a'+i)), "type": "page", "url": u})
		}
		json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

// The overlay names its page by URL; the daemon must find exactly that tab and
// refuse rather than guess when none or several show it.
func TestTabShowing(t *testing.T) {
	addr := fakeTabs(t, "https://a.test/x", "https://a.test/y", "https://a.test/y", "chrome-extension://abc/side-panel.html")
	ctx := context.Background()
	if id, err := tabShowing(ctx, addr, "https://a.test/x"); err != nil || id != "tab-a" {
		t.Errorf("x: got %q, %v; want tab-a", id, err)
	}
	if _, err := tabShowing(ctx, addr, "https://a.test/y"); err == nil || !strings.Contains(err.Error(), "2 tabs") {
		t.Errorf("y: got %v; want an ambiguity error", err)
	}
	if _, err := tabShowing(ctx, addr, "https://a.test/z"); err == nil || !strings.Contains(err.Error(), "no tab") {
		t.Errorf("z: got %v; want a not-found error", err)
	}
}

// /capture accepts only a POST naming a URL, and answers CORS preflight so the
// extension can call it.
func TestCaptureHandlerRejectsBadRequests(t *testing.T) {
	mux := http.NewServeMux()
	registerCaptureHandler(mux, t.TempDir(), fakeTabs(t, "https://a.test/x"))
	for _, c := range []struct {
		method, body string
		want         int
	}{
		{http.MethodOptions, "", http.StatusOK},
		{http.MethodGet, "", http.StatusMethodNotAllowed},
		{http.MethodPost, `{}`, http.StatusBadRequest},
		{http.MethodPost, `not json`, http.StatusBadRequest},
		{http.MethodPost, `{"url":"https://a.test/elsewhere"}`, http.StatusConflict},
	} {
		req := httptest.NewRequest(c.method, "/capture", strings.NewReader(c.body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s %s: status %d, want %d (%s)", c.method, c.body, rec.Code, c.want, strings.TrimSpace(rec.Body.String()))
		}
		if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("%s %s: no CORS header", c.method, c.body)
		}
	}
}
