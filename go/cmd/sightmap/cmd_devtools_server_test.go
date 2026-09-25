package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sightmap/sightmap/go/browser"
)

// newInjectServer wires registerDevtoolsHandlers against a collector that has
// not been started. There are no attached tabs, so POST /inject's size guard
// runs in front of AddPersistentScript without needing a live CDP connection —
// AddPersistentScript has nothing to apply to and just records the source.
func newInjectServer(t *testing.T) (*httptest.Server, *browser.Collector) {
	t.Helper()
	c := browser.NewCollector("127.0.0.1:0")
	var ptr atomic.Pointer[browser.Collector]
	ptr.Store(c)
	mux := http.NewServeMux()
	registerDevtoolsHandlers(mux, &ptr, t.TempDir())
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, c
}

// postInject POSTs body to /inject and returns the status code and response
// body. The body is sent with a Content-Length (bytes.Reader exposes Len), the
// path the in-tree CLI takes.
func postInject(t *testing.T, srv *httptest.Server, body []byte) (status int, respBody string) {
	t.Helper()
	resp, err := http.Post(srv.URL+"/inject", "application/javascript", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /inject: %v", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return resp.StatusCode, string(b)
}

// TestInject_PostRejectsAnOversizeBody guards the fix: an upload strictly
// larger than maxInjectBytes must be refused with 413, not silently truncated
// to the cap and stored. Before the fix, io.ReadAll(io.LimitReader(r.Body,
// maxInjectBytes)) returned (cap bytes, nil) for an oversized body, the handler
// proceeded to AddPersistentScript, and the client got 200 with the truncated
// byte count. Mirrors go/atlas/fetch_test.go:TestFetch_refusesAnOversizeBody.
func TestInject_PostRejectsAnOversizeBody(t *testing.T) {
	srv, c := newInjectServer(t)

	status, body := postInject(t, srv, bytes.Repeat([]byte{'a'}, maxInjectBytes+1))
	if status != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body = %q", status, body)
	}
	if !strings.Contains(body, "too large") {
		t.Errorf("body = %q, want it to mention the oversize refusal", body)
	}
	// The oversized script must NOT have been registered. A truncated
	// registration (the bug) would surface here as one entry with
	// Bytes == maxInjectBytes.
	if got := c.PersistentScripts(); len(got) != 0 {
		t.Errorf("PersistentScripts = %+v, want none registered for a refused upload", got)
	}
}

// TestInject_PostRejectsAnOversizeChunkedBody confirms the cap+1 read rejects
// a Content-Length-free client too. The cap+1 idiom (rather than an
// r.ContentLength pre-check) is what protects chunked uploads; without this
// guard a future "simplification" to a ContentLength check would pass the test
// above (bytes.Reader sets Content-Length) yet re-open truncation for a client
// that sends no Content-Length.
func TestInject_PostRejectsAnOversizeChunkedBody(t *testing.T) {
	srv, c := newInjectServer(t)

	// Wrapping bytes.Reader in a plain io.Reader strips the Len()/Seek
	// methods net/http keys Content-Length off, so the body is transferred
	// chunked (Transfer-Encoding: chunked, no Content-Length).
	body := struct{ io.Reader }{bytes.NewReader(bytes.Repeat([]byte{'a'}, maxInjectBytes+1))}
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/inject", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/javascript")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /inject: %v", err)
	}
	defer resp.Body.Close()
	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body = %q", resp.StatusCode, string(rb))
	}
	if got := c.PersistentScripts(); len(got) != 0 {
		t.Errorf("PersistentScripts = %+v, want none registered for a refused chunked upload", got)
	}
}

// TestInject_PostAcceptsTheCap is the off-by-one guard on the accept side: a
// body of exactly maxInjectBytes is accepted whole and the echoed/stored size
// matches. A regression to `len(src) >= maxInjectBytes` (reject) would wrongly
// drop the at-the-cap case, so this also doubles as the happy-path smoke test
// that a legitimate POST still returns 200 and registers the script bytes
// verbatim (no truncation).
func TestInject_PostAcceptsTheCap(t *testing.T) {
	srv, c := newInjectServer(t)
	payload := bytes.Repeat([]byte{'a'}, maxInjectBytes)

	status, body := postInject(t, srv, payload)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %q", status, body)
	}
	var got struct {
		ID    string `json:"id"`
		Bytes int    `json:"bytes"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("unmarshal response %q: %v", body, err)
	}
	if got.ID == "" {
		t.Errorf("response = %q, want a non-empty id", body)
	}
	if got.Bytes != maxInjectBytes {
		t.Errorf("echoed bytes = %d, want %d (the submitted size, not a truncated one)", got.Bytes, maxInjectBytes)
	}
	scripts := c.PersistentScripts()
	if len(scripts) != 1 {
		t.Fatalf("PersistentScripts = %+v, want exactly one registered", scripts)
	}
	if scripts[0].Bytes != maxInjectBytes {
		t.Errorf("stored bytes = %d, want %d", scripts[0].Bytes, maxInjectBytes)
	}
}
