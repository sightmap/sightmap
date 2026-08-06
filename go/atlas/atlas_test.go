package atlas

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// ── archive fixtures ──────────────────────────────────────────────────────────

// member is one tar entry, spelled out so a test can publish something an
// honest packer never would: an absolute path, a symlink, a lying size.
type member struct {
	name string
	body string
	typ  byte  // tar.TypeReg when zero
	size int64 // overrides the declared size when non-zero
}

// tarGz packs members into a gzipped tar.
func tarGz(t *testing.T, members []member) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for _, m := range members {
		typ := m.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		hdr := &tar.Header{Name: m.name, Mode: 0o644, Typeflag: typ, Size: int64(len(m.body))}
		if typ == tar.TypeDir {
			hdr.Mode = 0o755
			hdr.Size = 0
		}
		if typ == tar.TypeSymlink {
			hdr.Linkname = m.body
			hdr.Size = 0
		}
		if m.size != 0 {
			hdr.Size = m.size
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header %s: %v", m.name, err)
		}
		if hdr.Typeflag == tar.TypeReg && hdr.Size > 0 {
			if _, err := tw.Write([]byte(m.body)); err != nil {
				t.Fatalf("write body %s: %v", m.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

// corpusArchive packs corpus-relative files under the .sightmap/ prefix an
// atlas archive publishes.
func corpusArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	members := []member{{name: ".sightmap/", typ: tar.TypeDir}}
	for name, body := range files {
		members = append(members, member{name: corpusPrefix + name, body: body})
	}
	return tarGz(t, members)
}

const (
	configYAML = "version: 1\n"
	viewYAML   = "version: 1\nviews: []\n"
)

// ── servers ───────────────────────────────────────────────────────────────────

// fakeAtlas serves bodies keyed by URL path, so a test spells out exactly which
// URLs exist and how many times each was asked for.
type fakeAtlas struct {
	*httptest.Server
	mu        sync.Mutex
	requested []string
}

func newFakeAtlas(t *testing.T, bodies map[string][]byte) *fakeAtlas {
	t.Helper()
	f := &fakeAtlas{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requested = append(f.requested, r.URL.Path)
		f.mu.Unlock()
		body, ok := bodies[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(body)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeAtlas) indexURL() string { return f.URL + "/index.json" }

func (f *fakeAtlas) archiveTemplate() string { return f.URL + "/entries/{slug}.tar.gz" }

func (f *fakeAtlas) paths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requested...)
}

// ── assertions ────────────────────────────────────────────────────────────────

func mustContain(t *testing.T, got string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("output = %q, want it to mention %q", got, w)
		}
	}
}
