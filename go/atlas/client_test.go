package atlas

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The policy has to hold on the URL a caller hands in, not only on redirects:
// a --index or --source someone pasted is untrusted input.
func TestFetch_refusesPlainHTTPOnTheFirstHop(t *testing.T) {
	_, err := NewClient().Fetch(context.Background(), "http://atlas.example.com/index.json", MaxIndexBytes)
	if err == nil {
		t.Fatal("expected a transport refusal")
	}
	mustContain(t, err.Error(), "refusing non-HTTPS URL", "http://atlas.example.com/index.json", "localhost")
}

// A single archive fetch still follows redirects, so the downgrade is still
// reachable: without a per-hop check, `302 Location: http://…` installs
// attacker bytes over a connection the caller believed was TLS.
func TestFetch_refusesAPlainHTTPRedirectHop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://atlas.example.com/evil.tar.gz", http.StatusFound)
	}))
	defer srv.Close()

	_, err := NewClient().Fetch(context.Background(), srv.URL+"/entries/square-pos.tar.gz", MaxArchiveBytes)
	if err == nil {
		t.Fatal("expected the redirect hop to be refused")
	}
	mustContain(t, err.Error(), "redirected to a URL the atlas policy refuses", "http://atlas.example.com/evil.tar.gz")
}

// The refusal is about the scheme reaching a public host, not about redirects.
// A loopback mirror that redirects within itself keeps working.
func TestFetch_followsALoopbackRedirect(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/moved" {
			http.Redirect(w, r, srv.URL+"/final", http.StatusFound)
			return
		}
		fmt.Fprint(w, "arrived")
	}))
	defer srv.Close()

	body, err := NewClient().Fetch(context.Background(), srv.URL+"/moved", MaxIndexBytes)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(body) != "arrived" {
		t.Errorf("body = %q, want %q", body, "arrived")
	}
}

// Every hop is checked, not only the first one: a chain that stays on https
// until the last hop is the interesting case.
func TestFetch_refusesADowngradeLateInTheChain(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a":
			http.Redirect(w, r, srv.URL+"/b", http.StatusFound)
		case "/b":
			http.Redirect(w, r, srv.URL+"/c", http.StatusFound)
		case "/c":
			http.Redirect(w, r, "http://atlas.example.com/evil", http.StatusFound)
		}
	}))
	defer srv.Close()

	_, err := NewClient().Fetch(context.Background(), srv.URL+"/a", MaxIndexBytes)
	if err == nil {
		t.Fatal("expected the third hop to be refused")
	}
	mustContain(t, err.Error(), "atlas policy refuses", "http://atlas.example.com/evil")
}

func TestFetch_stopsAnEndlessRedirectChain(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/again", http.StatusFound)
	}))
	defer srv.Close()

	_, err := NewClient().Fetch(context.Background(), srv.URL+"/start", MaxIndexBytes)
	if err == nil {
		t.Fatal("expected the chain to be stopped")
	}
	mustContain(t, err.Error(), fmt.Sprintf("stopped after %d redirects", MaxRedirects))
}

// A timeout bounds how long a response takes, not how large it is.
func TestFetch_refusesAnOversizeBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("x", 64)))
	}))
	defer srv.Close()

	_, err := NewClient().Fetch(context.Background(), srv.URL+"/index.json", 16)
	if err == nil {
		t.Fatal("expected the size cap to refuse the body")
	}
	mustContain(t, err.Error(), "exceeds the 16-byte limit")
}

// The status is typed so an install can turn "404 on the archive URL" into
// "the atlas publishes no such slug" instead of an HTTP code.
func TestFetch_reportsANon200AsATypedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	_, err := NewClient().Fetch(context.Background(), srv.URL+"/missing.tar.gz", MaxArchiveBytes)
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("err = %v, want an *HTTPError", err)
	}
	if httpErr.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", httpErr.StatusCode)
	}
}

// A URL is index-supplied text too, so an error message about one is escaped
// like every other line the atlas can influence.
func TestFetch_escapesAHostileURLInItsRefusal(t *testing.T) {
	_, err := NewClient().Fetch(context.Background(), "ftp://evil.test/\x1b[2Kdrop", MaxIndexBytes)
	if err == nil {
		t.Fatal("expected a transport refusal")
	}
	if strings.Contains(err.Error(), "\x1b") {
		t.Fatalf("error = %q, want the escape sequence rendered rather than emitted", err.Error())
	}
}
