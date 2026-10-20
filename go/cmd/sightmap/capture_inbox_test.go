package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Two captures in the same second must not overwrite each other: the inbox is
// the only copy of a page someone chose to keep.
func TestInboxPathNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 20, 1, 2, 3, 0, time.UTC)
	first := inboxPath(dir, now)
	if want := filepath.Join(dir, "inbox", "20261020T010203Z.snap"); first != want {
		t.Fatalf("first path %s, want %s", first, want)
	}
	if err := os.MkdirAll(filepath.Dir(first), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	second := inboxPath(dir, now)
	if want := filepath.Join(dir, "inbox", "20261020T010203Z-2.snap"); second != want {
		t.Errorf("second path %s, want %s", second, want)
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
