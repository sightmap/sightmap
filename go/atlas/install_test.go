package atlas

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// installArchive runs Install against a server publishing exactly one archive.
func installArchive(t *testing.T, archive []byte, target string) (*Result, error) {
	t.Helper()
	srv := newFakeAtlas(t, map[string][]byte{"/entries/square-pos.tar.gz": archive})
	return Install(context.Background(), "square-pos", Options{ArchiveURL: srv.archiveTemplate(), Target: target})
}

// assertNothingLanded checks that a refused install left neither a target nor a
// staging directory behind.
func assertNothingLanded(t *testing.T, parent, target string) {
	t.Helper()
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("a refused install created the target anyway")
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".sightmap-add-") {
			t.Errorf("staging directory %s survived", e.Name())
		}
	}
}

// ── the happy path ────────────────────────────────────────────────────────────

// One archive, extracted whole, with the .sightmap/ prefix stripped as each
// file lands so --target may be named anything. The staging directory must not
// survive, and no index is read: an install works through a catalog outage.
func TestInstall_writesTheCorpusFromOneArchive(t *testing.T) {
	srv := newFakeAtlas(t, map[string][]byte{
		"/entries/square-pos.tar.gz": corpusArchive(t, map[string]string{
			"config.yaml":         configYAML,
			"views/checkout.yaml": viewYAML,
		}),
	})
	parent := t.TempDir()
	target := filepath.Join(parent, "corpus-elsewhere")

	res, err := Install(context.Background(), "square-pos", Options{ArchiveURL: srv.archiveTemplate(), Target: target})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !slices.Equal(res.Files, []string{"config.yaml", "views/checkout.yaml"}) {
		t.Errorf("Files = %v, want the corpus-relative paths sorted", res.Files)
	}
	if got, err := os.ReadFile(filepath.Join(target, "views", "checkout.yaml")); err != nil || string(got) != viewYAML {
		t.Errorf("views/checkout.yaml = %q, %v", got, err)
	}
	if got := srv.paths(); !slices.Equal(got, []string{"/entries/square-pos.tar.gz"}) {
		t.Errorf("fetched %v, want only the archive", got)
	}
	assertNothingLanded(t, parent, filepath.Join(parent, ".sightmap-add-nope"))
}

// An empty directory is not an existing corpus, so scaffolding one first must
// not block the install.
func TestInstall_installsIntoAnExistingEmptyDirectory(t *testing.T) {
	target := filepath.Join(t.TempDir(), ".sightmap")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := installArchive(t, corpusArchive(t, map[string]string{"config.yaml": configYAML}), target); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "config.yaml")); err != nil {
		t.Errorf("config.yaml: %v", err)
	}
}

// ── refusals before the network ───────────────────────────────────────────────

// A user with an existing corpus and no connectivity has to get the actionable
// refusal, not a dial timeout, so the target is checked first.
func TestInstall_refusesANonEmptyTargetBeforeAnyNetworkIO(t *testing.T) {
	target := filepath.Join(t.TempDir(), ".sightmap")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "mine.yaml"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := newFakeAtlas(t, map[string][]byte{"/entries/square-pos.tar.gz": corpusArchive(t, map[string]string{"config.yaml": configYAML})})

	_, err := Install(context.Background(), "square-pos", Options{ArchiveURL: srv.archiveTemplate(), Target: target})
	if !errors.Is(err, ErrTargetNotEmpty) {
		t.Fatalf("Install err = %v, want ErrTargetNotEmpty", err)
	}
	mustContain(t, err.Error(), "delete it and run this again")
	if got := srv.paths(); len(got) != 0 {
		t.Errorf("fetched %v, want no request before the refusal", got)
	}
	if got, _ := os.ReadFile(filepath.Join(target, "mine.yaml")); string(got) != "mine\n" {
		t.Errorf("mine.yaml = %q, want it untouched", got)
	}
}

// The slug and the archive URL both reach the network, so both are checked
// before anything is dialled.
func TestInstall_refusesBadInput(t *testing.T) {
	for _, tc := range []struct {
		name, slug, archiveURL, wantErr string
	}{
		{"an unsafe slug", "../../etc/passwd", "", "invalid slug"},
		{"a plain-http archive URL", "square-pos", "http://atlas.example.com/{slug}.tar.gz", "refusing non-HTTPS URL"},
		{"a template with no placeholder", "square-pos", "https://atlas.example.com/all.tar.gz", "no {slug} placeholder"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), ".sightmap")
			_, err := Install(context.Background(), tc.slug, Options{ArchiveURL: tc.archiveURL, Target: target})
			if err == nil {
				t.Fatal("expected a refusal")
			}
			mustContain(t, err.Error(), tc.wantErr)
		})
	}
}

// A slug the atlas does not publish is a 404, and the message has to point
// somewhere useful rather than at an HTTP status.
func TestInstall_reportsAnUnpublishedSlugWithSomewhereToLook(t *testing.T) {
	srv := newFakeAtlas(t, nil)
	_, err := Install(context.Background(), "not-published", Options{
		ArchiveURL: srv.archiveTemplate(), Target: filepath.Join(t.TempDir(), ".sightmap"),
	})
	if !errors.Is(err, ErrNoSuchEntry) {
		t.Fatalf("Install err = %v, want ErrNoSuchEntry", err)
	}
	mustContain(t, err.Error(), AtlasURL, "sightmap atlas find")
}

// ── the archive is untrusted ──────────────────────────────────────────────────

// Every axis an archive controls is bounded, and crossing any of them refuses
// the whole install rather than skipping the member: an install that silently
// drops content produces something other than what the atlas published.
func TestInstall_refusesAHostileArchive(t *testing.T) {
	oversizeFile := corpusArchive(t, map[string]string{"config.yaml": strings.Repeat("y", maxCorpusFileBytes+1)})

	manyMembers := make(map[string]string, maxArchiveEntries+1)
	for i := range maxArchiveEntries + 1 {
		manyMembers[fmt.Sprintf("v%d.yaml", i)] = viewYAML
	}

	// A gzip bomb: a few hundred kilobytes on the wire, over the decompressed
	// cap on disk. Every member stays under the per-file cap, so only the total
	// can catch it.
	var bomb bytes.Buffer
	zw := gzip.NewWriter(&bomb)
	tw := tar.NewWriter(zw)
	for i := range maxCorpusBytes/maxCorpusFileBytes + 1 {
		hdr := &tar.Header{Name: fmt.Sprintf(".sightmap/big%d.yaml", i), Mode: 0o644, Typeflag: tar.TypeReg, Size: maxCorpusFileBytes}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := io.CopyN(tw, zeroes{}, maxCorpusFileBytes); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	zw.Close()

	for _, tc := range []struct {
		name    string
		archive []byte
		wantErr string
	}{
		{"a traversal member", withMember(t, member{name: ".sightmap/../../../etc/passwd", body: "x"}), "unsafe path"},
		{"an absolute member", withMember(t, member{name: "/etc/passwd", body: "x"}), "unsafe path"},
		{"a backslash member", withMember(t, member{name: `.sightmap/..\..\evil.yaml`, body: "x"}), "unsafe path"},
		{"an escape sequence in a member name", withMember(t, member{name: ".sightmap/\x1b[2Kconfig.yaml", body: "x"}), "unsafe path"},
		{"a member outside the corpus prefix", withMember(t, member{name: "evil/config.yaml", body: "x"}), "not under .sightmap/"},
		// A symlink is the other way out: a link to /etc and a file written
		// through it lands outside without any path ever looking wrong.
		{"a symlink", withMember(t, member{name: ".sightmap/escape", body: "/etc", typ: tar.TypeSymlink}), "not a regular file or a directory"},
		// Capped against the bytes that arrive, not the size the header claims.
		{"a file over the size cap", oversizeFile, "file limit"},
		{"more members than the cap", corpusArchive(t, manyMembers), "more than"},
		{"a decompression bomb", bomb.Bytes(), "decompresses to more than"},
		{"an archive with no corpus", tarGz(t, []member{{name: "README.md", body: "hi"}}), "not under .sightmap/"},
		{"an empty archive", tarGz(t, nil), "publishes no files"},
		{"something that is not an archive", []byte("<!doctype html>this is a 404 page"), "read archive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			target := filepath.Join(parent, ".sightmap")
			_, err := installArchive(t, tc.archive, target)
			if err == nil {
				t.Fatal("expected the archive to be refused")
			}
			mustContain(t, err.Error(), tc.wantErr)
			if strings.Contains(err.Error(), "\x1b") {
				t.Errorf("error emitted a raw escape: %q", err)
			}
			assertNothingLanded(t, parent, target)
		})
	}
}

// withMember packs a valid corpus plus one hostile member.
func withMember(t *testing.T, m member) []byte {
	t.Helper()
	return tarGz(t, []member{{name: ".sightmap/config.yaml", body: configYAML}, m})
}

// zeroes is an endless run of NUL bytes: highly compressible, so a tar of it
// is a bomb.
type zeroes struct{}

func (zeroes) Read(p []byte) (int, error) { return len(p), nil }

// ── the load gate ─────────────────────────────────────────────────────────────

// A published-but-broken entry would otherwise install cleanly and fail later
// against the user's own files, with nothing pointing at the atlas.
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
	assertNothingLanded(t, parent, target)
}
