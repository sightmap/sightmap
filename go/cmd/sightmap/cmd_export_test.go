package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// exportCorpus is a corpus exercising every top-level wire section the legacy
// Python collector dropped — views (with routes), view-scoped + global requests,
// and messages — so the round-trip asserts export emits the FULL canonical
// Corpus, not the lossy flattened-components shape.
var exportCorpus = map[string]string{
	"components.yaml": `version: 1
memory:
  - corpus-wide note
components:
  - name: Navigation
    selector: 'nav[data-component="Navigation"]'
requests:
  - name: GetCurrentUser
    route: /api/me
    method: GET
messages:
  - name: CartVersionMismatch
    level: error
    message: cart version mismatch
`,
	"views/checkout.yaml": `version: 1
views:
  - name: Checkout
    route: /checkout
    components:
      - name: PaymentForm
        selector: '[data-component="PaymentForm"]'
    requests:
      - name: CheckoutPayment
        route: /api/checkout/pay
        method: POST
        properties:
          - name: outcome
            source: rsp.body
            field: status
`,
}

func writeCorpus(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	sdir := filepath.Join(dir, ".sightmap")
	for rel, body := range files {
		path := filepath.Join(sdir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return sdir
}

func TestExport_ToFile_CanonicalWire(t *testing.T) {
	sdir := writeCorpus(t, exportCorpus)
	outPath := filepath.Join(t.TempDir(), "corpus.json")

	if err := runExport([]string{"--sightmap-dir", sdir, "-o", outPath}); err != nil {
		t.Fatalf("runExport: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read out: %v", err)
	}

	// The emitted bytes must parse back into the canonical Corpus type — proving
	// export and the library/server reader share one shape.
	var corp sightmap.Corpus
	if err := json.Unmarshal(data, &corp); err != nil {
		t.Fatalf("emitted JSON is not a canonical Corpus: %v", err)
	}

	if len(corp.GlobalComponents) != 1 || corp.GlobalComponents[0].Name != "Navigation" {
		t.Errorf("globals: got %+v", corp.GlobalComponents)
	}
	if len(corp.Requests) != 1 || corp.Requests[0].Name != "GetCurrentUser" {
		t.Errorf("global requests: got %+v", corp.Requests)
	}
	if len(corp.Messages) != 1 || corp.Messages[0].Name != "CartVersionMismatch" {
		t.Errorf("messages: got %+v", corp.Messages)
	}
	if len(corp.Views) != 1 || corp.Views[0].Route != "/checkout" {
		t.Fatalf("views: got %+v", corp.Views)
	}
	// View-scoped request + its property survive (the lossy collector dropped both).
	vr := corp.Views[0].Requests
	if len(vr) != 1 || vr[0].Name != "CheckoutPayment" || len(vr[0].Properties) != 1 || vr[0].Properties[0].Field != "status" {
		t.Errorf("view-scoped request/property: got %+v", vr)
	}
}

func TestExport_ToURL_PostsCanonicalWire(t *testing.T) {
	sdir := writeCorpus(t, exportCorpus)

	var (
		mu        sync.Mutex
		gotBody   []byte
		gotCT     string
		gotAuth   string
		gotMethod string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotBody, gotCT, gotAuth, gotMethod = body, r.Header.Get("Content-Type"), r.Header.Get("Authorization"), r.Method
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	if err := runExport([]string{"--sightmap-dir", sdir, "--url", srv.URL}); err != nil {
		t.Fatalf("runExport --url: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	if gotAuth != "" {
		t.Errorf("Authorization = %q, want empty (no auth headers)", gotAuth)
	}
	var corp sightmap.Corpus
	if err := json.Unmarshal(gotBody, &corp); err != nil {
		t.Fatalf("POSTed body is not a canonical Corpus: %v", err)
	}
	if len(corp.Views) != 1 || len(corp.Messages) != 1 {
		t.Errorf("POSTed corpus incomplete: views=%d messages=%d", len(corp.Views), len(corp.Messages))
	}
}

func TestExport_NoCorpus_Errors(t *testing.T) {
	empty := t.TempDir()
	err := runExport([]string{empty})
	if err == nil || !strings.Contains(err.Error(), "no .sightmap/ directory found") {
		t.Fatalf("want no-corpus error, got %v", err)
	}
}

func TestPush_FileAndStdin(t *testing.T) {
	var got []byte
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = body
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// From a file.
	f := filepath.Join(t.TempDir(), "c.json")
	if err := os.WriteFile(f, []byte(`{"hello":"file"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runPush([]string{srv.URL, f}); err != nil {
		t.Fatalf("push file: %v", err)
	}
	mu.Lock()
	if string(got) != `{"hello":"file"}` {
		t.Errorf("file body = %q", got)
	}
	mu.Unlock()

	// From stdin.
	withStdin(t, `{"hello":"stdin"}`, func() {
		if err := runPush([]string{srv.URL}); err != nil {
			t.Fatalf("push stdin: %v", err)
		}
	})
	mu.Lock()
	if string(got) != `{"hello":"stdin"}` {
		t.Errorf("stdin body = %q", got)
	}
	mu.Unlock()
}

func TestPush_Non2xxErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad token", http.StatusUnauthorized)
	}))
	defer srv.Close()

	err := runPush([]string{srv.URL, "-"}) // "-" reads (empty) stdin
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("want HTTP 401 error, got %v", err)
	}
}

func TestPostJSON_RejectsNonHTTPScheme(t *testing.T) {
	if err := postJSON("ftp://example.com/x", []byte("{}"), false); err == nil || !strings.Contains(err.Error(), "scheme") {
		t.Fatalf("want scheme error, got %v", err)
	}
}

func TestFindSightmapDir(t *testing.T) {
	root := t.TempDir()
	sdir := filepath.Join(root, ".sightmap")
	nested := filepath.Join(root, "a", "b", "c")
	for _, d := range []string{sdir, nested} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Upward walk from a deep descendant finds the ancestor .sightmap.
	if got := findSightmapDir(nested); got != sdir {
		t.Errorf("from nested: got %q, want %q", got, sdir)
	}
	// Passing the .sightmap dir itself returns it directly.
	if got := findSightmapDir(sdir); got != sdir {
		t.Errorf("from .sightmap: got %q, want %q", got, sdir)
	}
	// A tree with no .sightmap returns "".
	if got := findSightmapDir(t.TempDir()); got != "" {
		t.Errorf("no corpus: got %q, want empty", got)
	}
}

func TestResolveExportDir_FlagWins(t *testing.T) {
	dir, err := resolveExportDir("/explicit/.sightmap", []string{"/ignored"})
	if err != nil || dir != "/explicit/.sightmap" {
		t.Fatalf("explicit flag should win: got %q, %v", dir, err)
	}
}

func TestIsLocalHost(t *testing.T) {
	for _, h := range []string{"localhost", "127.0.0.1", "::1", "app.test", "foo.localhost"} {
		if !isLocalHost(h) {
			t.Errorf("isLocalHost(%q) = false, want true", h)
		}
	}
	for _, h := range []string{"example.com", "sightmap.org", "10.0.0.1"} {
		if isLocalHost(h) {
			t.Errorf("isLocalHost(%q) = true, want false", h)
		}
	}
}

// withStdin swaps os.Stdin for a pipe carrying content for the duration of fn.
func withStdin(t *testing.T, content string, fn func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = orig }()

	go func() {
		io.WriteString(w, content)
		w.Close()
	}()
	fn()
	r.Close()
}
