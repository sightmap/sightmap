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

// A capture waiting behind another is bounded by its own request: when the
// caller gives up (the overlay does after 25s), it gets a 503 that says why
// rather than queueing until the earlier capture ends.
func TestCaptureHandlerWaitIsBounded(t *testing.T) {
	held := make(chan struct{})    // closed once the first capture holds the slot
	release := make(chan struct{}) // closed to let it finish
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(held) })
		<-release // the first capture is stuck listing tabs, holding the slot
		json.NewEncoder(w).Encode([]map[string]string{})
	}))
	t.Cleanup(srv.Close)
	mux := http.NewServeMux()
	registerCaptureHandler(mux, t.TempDir(), strings.TrimPrefix(srv.URL, "http://"))
	post := func(ctx context.Context) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/capture", strings.NewReader(`{"url":"https://a.test/x"}`)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { firstDone <- post(firstCtx) }()

	<-held
	secondCtx, cancelSecond := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancelSecond()
	rec := post(secondCtx)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "another capture is still running") {
		t.Errorf("queued capture: status %d (%s), want 503 naming the running capture", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	close(release)
	if first := <-firstDone; first.Code != http.StatusConflict {
		t.Errorf("first capture: status %d, want 409 (no tab)", first.Code)
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

// /capture accepts only a JSON POST naming a URL, from the overlay extension
// or a local tool: a web page's request (an http(s) Origin, or a body type
// that skips the CORS preflight) is refused before anything is captured, and
// no response invites cross-origin use.
func TestCaptureHandlerRejectsBadRequests(t *testing.T) {
	mux := http.NewServeMux()
	registerCaptureHandler(mux, t.TempDir(), fakeTabs(t, "https://a.test/x"))
	const json = "application/json"
	for _, c := range []struct {
		method, origin, ctype, body string
		want                        int
	}{
		{http.MethodOptions, "", "", "", http.StatusMethodNotAllowed},
		{http.MethodGet, "", "", "", http.StatusMethodNotAllowed},
		{http.MethodPost, "https://evil.test", json, `{"url":"https://a.test/x"}`, http.StatusForbidden},
		{http.MethodPost, "null", json, `{"url":"https://a.test/x"}`, http.StatusForbidden},
		{http.MethodPost, "", "text/plain", `{"url":"https://a.test/x"}`, http.StatusUnsupportedMediaType},
		{http.MethodPost, "", "", `{"url":"https://a.test/x"}`, http.StatusUnsupportedMediaType},
		{http.MethodPost, "", json, `{}`, http.StatusBadRequest},
		{http.MethodPost, "", json, `not json`, http.StatusBadRequest},
		// Past the checks (a CLI with no Origin, or the extension) the
		// request reaches the tab lookup, which finds no such tab.
		{http.MethodPost, "", "application/json; charset=utf-8", `{"url":"https://a.test/elsewhere"}`, http.StatusConflict},
		{http.MethodPost, "chrome-extension://abcdefghijklmnop", json, `{"url":"https://a.test/elsewhere"}`, http.StatusConflict},
	} {
		req := httptest.NewRequest(c.method, "/capture", strings.NewReader(c.body))
		if c.origin != "" {
			req.Header.Set("Origin", c.origin)
		}
		if c.ctype != "" {
			req.Header.Set("Content-Type", c.ctype)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s origin=%q type=%q %s: status %d, want %d (%s)", c.method, c.origin, c.ctype, c.body, rec.Code, c.want, strings.TrimSpace(rec.Body.String()))
		}
		if h := rec.Header().Get("Access-Control-Allow-Origin"); h != "" {
			t.Errorf("%s origin=%q: Access-Control-Allow-Origin %q, want none", c.method, c.origin, h)
		}
	}
}
