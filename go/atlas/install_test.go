package atlas

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// install runs Install against srv's archive layout.
func install(t *testing.T, srv *fakeAtlas, slug, target string) (*Result, error) {
	t.Helper()
	return Install(context.Background(), slug, Options{ArchiveURL: srv.archiveTemplate(), Target: target})
}

// installArchive runs Install against a server publishing exactly one archive.
func installArchive(t *testing.T, archive []byte, target string) (*Result, error) {
	t.Helper()
	srv := newFakeAtlas(t, map[string][]byte{"/entries/square-pos.tar.gz": archive})
	return install(t, srv, "square-pos", target)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// ── the happy path ────────────────────────────────────────────────────────────

func TestInstall_writesTheCorpusFromOneArchive(t *testing.T) {
	srv := newFakeAtlas(t, map[string][]byte{
		"/entries/square-pos.tar.gz": corpusArchive(t, map[string]string{
			"config.yaml":          configYAML,
			"views/checkout.yaml":  viewYAML,
			"views/orders/at.yaml": viewYAML,
		}),
	})
	target := filepath.Join(t.TempDir(), ".sightmap")

	res, err := install(t, srv, "square-pos", target)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got := fmt.Sprint(res.Files); got != "[config.yaml views/checkout.yaml views/orders/at.yaml]" {
		t.Errorf("files = %s", got)
	}
	if got := readFile(t, filepath.Join(target, "config.yaml")); got != configYAML {
		t.Errorf("config.yaml = %q", got)
	}
	if got := readFile(t, filepath.Join(target, "views", "orders", "at.yaml")); got != viewYAML {
		t.Errorf("nested view = %q", got)
	}
	// One request, not one per file.
	if got := srv.paths(); len(got) != 1 || got[0] != "/entries/square-pos.tar.gz" {
		t.Errorf("requested %v, want exactly the archive", got)
	}
}

// --target is not the corpus directory's name: the .sightmap/ prefix inside
// the archive is stripped, so the corpus can land anywhere.
func TestInstall_stripsTheCorpusPrefixSoTargetMayBeAnything(t *testing.T) {
	target := filepath.Join(t.TempDir(), "vendor", "square")
	res, err := installArchive(t, corpusArchive(t, map[string]string{"config.yaml": configYAML}), target)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if res.Files[0] != "config.yaml" {
		t.Errorf("files = %v", res.Files)
	}
	if got := readFile(t, filepath.Join(target, "config.yaml")); got != configYAML {
		t.Errorf("config.yaml = %q", got)
	}
}

func TestInstall_installsIntoAnExistingEmptyDirectory(t *testing.T) {
	target := filepath.Join(t.TempDir(), ".sightmap")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := installArchive(t, corpusArchive(t, map[string]string{"config.yaml": configYAML}), target); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got := readFile(t, filepath.Join(target, "config.yaml")); got != configYAML {
		t.Errorf("config.yaml = %q", got)
	}
}

// ── refusals ──────────────────────────────────────────────────────────────────

// The refusal has to be the same with or without connectivity, so it is
// decided before anything is dialled.
func TestInstall_refusesANonEmptyTargetBeforeAnyNetworkIO(t *testing.T) {
	srv := newFakeAtlas(t, map[string][]byte{
		"/entries/square-pos.tar.gz": corpusArchive(t, map[string]string{"config.yaml": configYAML}),
	})
	target := filepath.Join(t.TempDir(), ".sightmap")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	mine := filepath.Join(target, "components.yaml")
	if err := os.WriteFile(mine, []byte("version: 1\n# mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := install(t, srv, "square-pos", target)
	if !errors.Is(err, ErrTargetNotEmpty) {
		t.Fatalf("err = %v, want ErrTargetNotEmpty", err)
	}
	mustContain(t, err.Error(), "delete it and run this again")
	if got := srv.paths(); len(got) != 0 {
		t.Errorf("requested %v, want nothing fetched", got)
	}
	if got := readFile(t, mine); got != "version: 1\n# mine\n" {
		t.Errorf("the existing corpus changed: %q", got)
	}
}

// A slug that resolves to nothing is a failed action, so it exits nonzero —
// and points at the two ways to find the right one.
func TestInstall_reportsAnUnpublishedSlugWithSomewhereToLook(t *testing.T) {
	srv := newFakeAtlas(t, map[string][]byte{})
	_, err := install(t, srv, "no-such-entry", filepath.Join(t.TempDir(), ".sightmap"))
	if !errors.Is(err, ErrNoSuchEntry) {
		t.Fatalf("err = %v, want ErrNoSuchEntry", err)
	}
	mustContain(t, err.Error(), `"no-such-entry"`, AtlasURL, "sightmap atlas find")
}

func TestInstall_refusesAnUnsafeSlug(t *testing.T) {
	for _, slug := range []string{"../../etc/passwd", "a/b", "esc\x1bape", ""} {
		_, err := Install(context.Background(), slug, Options{Target: filepath.Join(t.TempDir(), ".sightmap")})
		if err == nil {
			t.Fatalf("Install(%q) succeeded, want a refusal", slug)
		}
		if strings.Contains(err.Error(), "\x1b") {
			t.Errorf("Install(%q) error emitted a raw escape: %q", slug, err.Error())
		}
	}
}

func TestInstall_refusesANonHTTPSArchiveURL(t *testing.T) {
	_, err := Install(context.Background(), "square-pos", Options{
		ArchiveURL: "http://atlas.example.com/entries/{slug}.tar.gz",
		Target:     filepath.Join(t.TempDir(), ".sightmap"),
	})
	if err == nil {
		t.Fatal("expected the transport policy to refuse the archive URL")
	}
	mustContain(t, err.Error(), "refusing non-HTTPS URL", "localhost")
}

// The archive fetch follows redirects like any other, so the downgrade has to
// be refused there too.
func TestInstall_refusesAPlainHTTPRedirectOnTheArchive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://atlas.example.com/evil.tar.gz", http.StatusFound)
	}))
	defer srv.Close()

	target := filepath.Join(t.TempDir(), ".sightmap")
	_, err := Install(context.Background(), "square-pos", Options{
		ArchiveURL: srv.URL + "/entries/{slug}.tar.gz",
		Target:     target,
	})
	if err == nil {
		t.Fatal("expected the redirect hop to be refused")
	}
	mustContain(t, err.Error(), "atlas policy refuses")
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Error("a refused fetch created the target")
	}
}

func TestResolveArchiveURL_requiresASlugPlaceholder(t *testing.T) {
	_, err := ResolveArchiveURL("https://atlas.example.com/all.tar.gz", "square-pos")
	if err == nil {
		t.Fatal("expected a refusal")
	}
	mustContain(t, err.Error(), "{slug}")
}

// ── archive contents ──────────────────────────────────────────────────────────

// The archive is untrusted input. A member that would land outside the corpus
// refuses the install rather than being skipped.
func TestInstall_refusesMembersThatEscapeTheTarget(t *testing.T) {
	cases := map[string]struct {
		member member
		want   string
	}{
		"traversal":       {member{name: ".sightmap/../../../etc/passwd", body: "x"}, "relative path segment"},
		"absolute":        {member{name: "/etc/passwd", body: "x"}, "absolute path"},
		"outside prefix":  {member{name: "evil/config.yaml", body: "x"}, "is not under .sightmap/"},
		"backslash":       {member{name: `.sightmap/..\..\evil.yaml`, body: "x"}, "backslash"},
		"escape sequence": {member{name: ".sightmap/\x1b[2Kconfig.yaml", body: "x"}, "control character"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), ".sightmap")
			archive := tarGz(t, []member{
				{name: ".sightmap/config.yaml", body: configYAML},
				tc.member,
			})
			_, err := installArchive(t, archive, target)
			if err == nil {
				t.Fatal("expected the member to be refused")
			}
			mustContain(t, err.Error(), "unsafe path", tc.want)
			if strings.Contains(err.Error(), "\x1b") {
				t.Errorf("error emitted a raw escape: %q", err.Error())
			}
			if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
				t.Error("a refused archive still created the target")
			}
		})
	}
}

// Symlinks are the other way out of the target: a link to /etc and a file
// written through it lands outside without any path ever looking wrong.
func TestInstall_refusesNonFileMembers(t *testing.T) {
	archive := tarGz(t, []member{
		{name: ".sightmap/config.yaml", body: configYAML},
		{name: ".sightmap/escape", body: "/etc", typ: tar.TypeSymlink},
	})
	_, err := installArchive(t, archive, filepath.Join(t.TempDir(), ".sightmap"))
	if err == nil {
		t.Fatal("expected the symlink to be refused")
	}
	mustContain(t, err.Error(), "not a regular file or a directory")
}

// A compression bomb is a few hundred kilobytes on the wire and gigabytes on
// disk, so the download cap cannot be the only one.
func TestInstall_refusesADecompressionBomb(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	const size = MaxCorpusBytes * 2
	if err := tw.WriteHeader(&tar.Header{Name: ".sightmap/bomb.yaml", Mode: 0o644, Size: size}); err != nil {
		t.Fatal(err)
	}
	// Highly compressible: this is a few KB on the wire.
	chunk := bytes.Repeat([]byte{0}, 1<<16)
	for written := int64(0); written < size; written += int64(len(chunk)) {
		if _, err := tw.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	zw.Close()
	if buf.Len() > MaxArchiveBytes {
		t.Fatalf("the bomb is %d bytes on the wire, so the download cap would catch it first", buf.Len())
	}

	target := filepath.Join(t.TempDir(), ".sightmap")
	_, err := installArchive(t, buf.Bytes(), target)
	if err == nil {
		t.Fatal("expected the decompressed-size cap to refuse the archive")
	}
	mustContain(t, err.Error(), "over the")
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Error("the bomb left a target behind")
	}
}

// The per-file cap is checked against the bytes that arrive, not only against
// the size the header claims.
func TestInstall_refusesAFileLargerThanItsHeaderClaims(t *testing.T) {
	body := strings.Repeat("y", MaxCorpusFileBytes+4096)
	archive := tarGz(t, []member{{name: ".sightmap/big.yaml", body: body, size: int64(len(body))}})
	_, err := installArchive(t, archive, filepath.Join(t.TempDir(), ".sightmap"))
	if err == nil {
		t.Fatal("expected the per-file cap to refuse the archive")
	}
	mustContain(t, err.Error(), "file limit")
}

func TestInstall_refusesTooManyMembers(t *testing.T) {
	members := make([]member, 0, MaxArchiveEntries+2)
	for i := 0; i < MaxArchiveEntries+2; i++ {
		members = append(members, member{name: fmt.Sprintf(".sightmap/views/v%03d.yaml", i), body: viewYAML})
	}
	_, err := installArchive(t, tarGz(t, members), filepath.Join(t.TempDir(), ".sightmap"))
	if err == nil {
		t.Fatal("expected the member-count cap to refuse the archive")
	}
	mustContain(t, err.Error(), fmt.Sprintf("more than %d files", MaxArchiveEntries))
}

func TestInstall_refusesAnArchiveWithNoCorpus(t *testing.T) {
	archive := tarGz(t, []member{{name: ".sightmap/", typ: tar.TypeDir}})
	_, err := installArchive(t, archive, filepath.Join(t.TempDir(), ".sightmap"))
	if err == nil {
		t.Fatal("expected an archive with no files to be refused")
	}
	mustContain(t, err.Error(), "publishes no files under .sightmap/")
}

func TestInstall_refusesSomethingThatIsNotAnArchive(t *testing.T) {
	_, err := installArchive(t, []byte("this is not a gzip stream"), filepath.Join(t.TempDir(), ".sightmap"))
	if err == nil {
		t.Fatal("expected a refusal")
	}
	mustContain(t, err.Error(), "read archive")
}

// ── the load gate ─────────────────────────────────────────────────────────────

// A published-but-broken entry used to install cleanly and fail later against
// the user's own files, with nothing pointing at the atlas.
func TestInstall_reportsACorpusThatDoesNotLoadAsAnAtlasDefect(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, ".sightmap")
	archive := corpusArchive(t, map[string]string{"views/checkout.yaml": "views:\n\t- name: bad tabs\n"})

	_, err := installArchive(t, archive, target)
	if err == nil {
		t.Fatal("expected the load gate to refuse the corpus")
	}
	mustContain(t, err.Error(), "square-pos", "does not load", "the atlas entry is broken", "nothing was installed")
	// The corpus-relative path is what the user quotes when reporting it.
	mustContain(t, err.Error(), ".sightmap/views/checkout.yaml")
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Error("a corpus that does not load was installed anyway")
	}
	assertNoLeftovers(t, parent)
}

// Nothing partial may land, and nothing may be left beside the target: until
// the rename the corpus lives in a staging directory that is cleaned up.
func TestInstall_leavesNothingBesideTheTarget(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, ".sightmap")
	if _, err := installArchive(t, corpusArchive(t, map[string]string{"config.yaml": configYAML}), target); err != nil {
		t.Fatalf("Install: %v", err)
	}
	assertNoLeftovers(t, parent)
}

func assertNoLeftovers(t *testing.T, parent string) {
	t.Helper()
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".sightmap-add-") {
			t.Errorf("staging directory %s survived the install", e.Name())
		}
	}
}
