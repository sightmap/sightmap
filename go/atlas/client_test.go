package atlas

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The HTTPS gate is worthless if it only covers the URL the user typed: a
// mirror (or anything on the path to one) can answer 302 with a plaintext
// Location and have its bytes installed. The policy has to hold on every hop.
func TestClientFetch_redirectCannotDowngradeScheme(t *testing.T) {
	plaintext := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "attacker payload")
	}))
	defer plaintext.Close()

	// A redirect to plain http on a NON-loopback host is what a real
	// downgrade looks like; rewrite the loopback test server's URL to a name
	// the policy does not exempt.
	target := strings.Replace(plaintext.URL, "127.0.0.1", "attacker.example.com", 1)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target, http.StatusFound)
	}))
	defer redirector.Close()

	body, err := NewClient().Fetch(context.Background(), redirector.URL+"/main/index.json", MaxIndexBytes)
	if err == nil {
		t.Fatalf("redirect to %s was followed, got body %q", target, body)
	}
	if !strings.Contains(err.Error(), "non-HTTPS") {
		t.Errorf("error = %v, want a non-HTTPS refusal", err)
	}
}

func TestClientFetch_redirectChainIsBounded(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+r.URL.Path+"/again", http.StatusFound)
	}))
	defer srv.Close()

	_, err := NewClient().Fetch(context.Background(), srv.URL+"/main/index.json", MaxIndexBytes)
	if err == nil {
		t.Fatal("expected an error for an endless redirect chain")
	}
	if !strings.Contains(err.Error(), "redirect") {
		t.Errorf("error = %v, want it to mention redirects", err)
	}
}

// A same-policy redirect (loopback http → loopback http here) is still
// followed; the check refuses downgrades, it does not refuse redirects.
func TestClientFetch_allowedRedirectIsFollowed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/moved", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "arrived")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("/main/index.json", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/moved", http.StatusFound)
	})

	body, err := NewClient().Fetch(context.Background(), srv.URL+"/main/index.json", MaxIndexBytes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body) != "arrived" {
		t.Errorf("body = %q, want %q", body, "arrived")
	}
}

// A 30-second timeout bounds how long a body may take, not how large it may
// be, and every body is held in memory until the whole install is staged.
func TestClientFetch_sizeCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, 4096))
	}))
	defer srv.Close()

	client := NewClient()
	if _, err := client.Fetch(context.Background(), srv.URL+"/x", 4096); err != nil {
		t.Fatalf("a body exactly at the limit was refused: %v", err)
	}
	_, err := client.Fetch(context.Background(), srv.URL+"/x", 4095)
	if err == nil {
		t.Fatal("expected an over-limit error")
	}
	for _, want := range []string{"limit", "refusing to install"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to mention %q", err, want)
		}
	}
}

func TestClientFetch_statusAndURLInError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(http.NotFound))
	defer srv.Close()

	url := srv.URL + "/main/entries/shop/.sightmap/config.yaml"
	_, err := NewClient().Fetch(context.Background(), url, MaxFileBytes)
	if err == nil {
		t.Fatal("expected an error for a 404")
	}
	for _, want := range []string{url, "404"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to mention %q", err, want)
		}
	}
}

func TestClientFetch_contextCancels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewClient().Fetch(ctx, srv.URL+"/x", MaxFileBytes); err == nil {
		t.Fatal("expected a cancelled-context error")
	}
}
