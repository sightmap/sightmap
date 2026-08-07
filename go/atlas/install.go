package atlas

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/sightmap/sightmap/go/sightmap"
)

// DefaultArchiveURL is where one entry's corpus archive lives, with {slug}
// substituted. It is served from sightmap.org for the same reason as
// [DefaultIndexURL]: a takedown that removes an entry from the catalog but
// leaves its archive installable binds half the surface. Options.ArchiveURL
// overrides it, which is how a mirror or a private corpus store works:
//
//	sightmap atlas add toast-pos --source https://internal.corp/{slug}.tar.gz
const DefaultArchiveURL = "https://sightmap.org/atlas/{slug}.tar.gz"

// corpusPrefix is the only directory an archive may publish files under. It is
// the corpus directory's canonical name, not the install target's: the prefix
// is stripped as a file lands, so Options.Target may be named anything.
const corpusPrefix = ".sightmap/"

// Extraction limits. The wire cap in fetch.go bounds the download; a gzip bomb
// is a few hundred kilobytes on the wire and gigabytes on disk, so what lands
// is capped separately.
const (
	maxCorpusBytes     = 32 << 20 // total decompressed
	maxCorpusFileBytes = 4 << 20  // one file
	maxArchiveEntries  = 512
)

// ErrTargetNotEmpty reports that the install target already has contents.
// Deleting the directory is the user's call, so no flag overrides it.
var ErrTargetNotEmpty = errors.New("install target already exists and is not empty")

// ErrNoSuchEntry reports that the atlas publishes no corpus for the slug.
var ErrNoSuchEntry = errors.New("no corpus published for this slug")

// Options configures [Install].
type Options struct {
	// ArchiveURL is the archive URL template, with {slug} substituted. Empty
	// means [DefaultArchiveURL].
	ArchiveURL string
	// Target is the directory the corpus is installed into. Required; an empty
	// Target is an error, never "the current directory".
	Target string
}

// Result describes a completed install.
type Result struct {
	Slug   string   // the installed slug
	Target string   // the target directory, as the caller named it
	Source string   // the archive URL the corpus came from
	Files  []string // installed paths relative to Target, slash-separated, sorted
}

// Install writes the corpus published for slug into opts.Target.
//
// The order is the contract. A non-empty target is refused before any network
// I/O, so a user with an existing corpus and no connectivity gets the
// actionable refusal rather than a dial timeout. One archive is then fetched,
// extracted into a temporary directory beside the target, and loaded; only a
// corpus that loads is renamed into place. The rename is the atomicity: until
// it runs the target does not exist, and it is one syscall.
func Install(ctx context.Context, slug string, opts Options) (*Result, error) {
	if err := ValidateSlug(slug); err != nil {
		return nil, fmt.Errorf("invalid slug %q: %w", SafeText(slug), err)
	}
	if opts.Target == "" {
		return nil, errors.New("no install target: name the directory to install the corpus into")
	}
	archiveURL, err := resolveArchiveURL(opts.ArchiveURL, slug)
	if err != nil {
		return nil, err
	}
	target, err := filepath.Abs(opts.Target)
	if err != nil {
		return nil, fmt.Errorf("resolve target %s: %w", opts.Target, err)
	}
	switch nonEmpty, err := dirHasContents(target); {
	case err != nil:
		return nil, err
	case nonEmpty:
		return nil, fmt.Errorf("%w: %s — delete it and run this again to install over it", ErrTargetNotEmpty, opts.Target)
	}

	archive, err := fetch(ctx, archiveURL)
	if err != nil {
		var httpErr *httpError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%w: %q — browse %s, or search for the site: sightmap atlas find <domain>",
				ErrNoSuchEntry, SafeText(slug), AtlasURL)
		}
		return nil, err
	}

	files, err := stage(archive, target, slug)
	if err != nil {
		return nil, err
	}
	return &Result{Slug: slug, Target: opts.Target, Source: archiveURL, Files: files}, nil
}

// resolveArchiveURL substitutes slug into an archive URL template and applies
// the transport policy, so a refused URL is reported before anything is
// dialled.
func resolveArchiveURL(template, slug string) (string, error) {
	if template == "" {
		template = DefaultArchiveURL
	}
	if !strings.Contains(template, "{slug}") {
		return "", fmt.Errorf("archive URL %s has no {slug} placeholder — it must name where one entry's archive lives, for example %s", SafeText(template), DefaultArchiveURL)
	}
	raw := strings.ReplaceAll(template, "{slug}", url.PathEscape(slug))
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid archive URL %s: %w", SafeText(raw), err)
	}
	if err := checkURL(u); err != nil {
		return "", err
	}
	return raw, nil
}

// stage extracts the archive into a temporary directory beside the target,
// proves the corpus loads, and renames it into place. The staging directory is
// a sibling so the rename stays within one filesystem.
func stage(archive []byte, target, slug string) ([]string, error) {
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", parent, err)
	}
	staging, err := os.MkdirTemp(parent, ".sightmap-add-*")
	if err != nil {
		return nil, fmt.Errorf("create staging directory in %s: %w", parent, err)
	}
	// A no-op once the rename below has moved the staging directory away.
	defer os.RemoveAll(staging)

	files, err := extract(archive, staging)
	if err != nil {
		return nil, fmt.Errorf("atlas entry %q: %w", SafeText(slug), err)
	}
	// A corpus that does not load is a defect in the published entry, not in
	// the user's project, and the message has to say so. The corpus-relative
	// path is what they would quote when reporting it, so the staging path is
	// rewritten out.
	if _, err := sightmap.Load(staging); err != nil {
		msg := strings.ReplaceAll(err.Error(), staging+string(filepath.Separator), corpusPrefix)
		return nil, fmt.Errorf("atlas entry %q publishes a corpus that does not load: %s — the atlas entry is broken, nothing was installed", SafeText(slug), msg)
	}
	if err := os.Chmod(staging, 0o755); err != nil {
		return nil, fmt.Errorf("set permissions on staging directory: %w", err)
	}
	// os.Remove succeeds only on an empty directory, so a target that gained
	// files while the archive was in flight becomes an error rather than a
	// silent overwrite.
	if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("install into %s: %w", target, err)
	}
	if err := os.Rename(staging, target); err != nil {
		return nil, fmt.Errorf("install into %s: %w", target, err)
	}
	return files, nil
}

// extract unpacks a .tar.gz into dest and returns the corpus-relative paths it
// wrote, sorted. Everything the archive controls is bounded here: how much it
// decompresses to, how large and how numerous its members are, what type they
// may be, and where they may land.
func extract(archive []byte, dest string) ([]string, error) {
	zr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("read archive: %w", err)
	}
	defer zr.Close()
	// The cap covers the whole tar stream, headers included, so a bomb fails on
	// the read that crosses it rather than after it lands.
	tr := tar.NewReader(&cappedReader{r: zr, max: maxCorpusBytes})

	var files []string
	for seen := 0; ; seen++ {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read archive: %w", err)
		}
		if seen == maxArchiveEntries {
			return nil, fmt.Errorf("holds more than %d files", maxArchiveEntries)
		}
		rel, err := extractMember(tr, hdr, dest)
		if err != nil {
			return nil, err
		}
		if rel != "" {
			files = append(files, rel)
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("publishes no files under %s", corpusPrefix)
	}
	slices.Sort(files)
	return files, nil
}

// extractMember writes one archive member and returns its corpus-relative path,
// or "" for a directory.
func extractMember(r io.Reader, hdr *tar.Header, dest string) (string, error) {
	rel, err := corpusPath(hdr.Name)
	if err != nil {
		return "", err
	}
	switch {
	case hdr.Typeflag == tar.TypeDir:
		if rel == "" {
			return "", nil // the corpus root; dest already exists
		}
		path, err := safeJoin(dest, rel)
		if err != nil {
			return "", err
		}
		return "", os.MkdirAll(path, 0o755)
	case hdr.Typeflag != tar.TypeReg:
		// Symlinks, hardlinks, devices and fifos are not corpus content, and
		// dropping them silently would install something other than what the
		// atlas published.
		return "", fmt.Errorf("publishes %q, which is not a regular file or a directory", SafeText(hdr.Name))
	case rel == "":
		return "", fmt.Errorf("publishes %s as a file", corpusPrefix)
	}

	path, err := safeJoin(dest, rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create the directory for %s: %w", SafeText(rel), err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", fmt.Errorf("write %s: %w", SafeText(rel), err)
	}
	defer f.Close()
	// Capped against the bytes actually copied rather than the header's
	// declared size, so a lying header cannot outrun the limit.
	n, err := io.Copy(f, io.LimitReader(r, maxCorpusFileBytes+1))
	switch {
	case err != nil:
		return "", fmt.Errorf("write %s: %w", SafeText(rel), err)
	case n > maxCorpusFileBytes:
		return "", fmt.Errorf("publishes %q over the %d-byte file limit", SafeText(rel), maxCorpusFileBytes)
	}
	return rel, f.Close()
}

// corpusPath validates one archive member's name and returns its path relative
// to the corpus root, or "" for the root directory itself.
func corpusPath(name string) (string, error) {
	p := strings.TrimPrefix(strings.TrimSuffix(name, "/"), "./")
	// fs.ValidPath rejects absolute paths, empty segments, "." and "..".
	if !fs.ValidPath(p) || strings.Contains(p, `\`) || strings.ContainsFunc(p, unicode.IsControl) {
		return "", fmt.Errorf("publishes an unsafe path %q", SafeText(name))
	}
	if p == strings.TrimSuffix(corpusPrefix, "/") {
		return "", nil
	}
	rel, ok := strings.CutPrefix(p, corpusPrefix)
	if !ok {
		return "", fmt.Errorf("publishes %q, which is not under %s", SafeText(name), corpusPrefix)
	}
	return rel, nil
}

// safeJoin resolves a corpus-relative path under dest and proves it stays
// there. This is the zip-slip guard, restated on the path actually written.
func safeJoin(dest, rel string) (string, error) {
	local, err := filepath.Localize(rel)
	if err != nil {
		return "", fmt.Errorf("publishes %q, which escapes the install directory", SafeText(rel))
	}
	return filepath.Join(dest, local), nil
}

// cappedReader fails the read that crosses max, so a compression bomb is caught
// while it decompresses rather than after it lands.
type cappedReader struct {
	r   io.Reader
	n   int64
	max int64
}

func (c *cappedReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	if c.n > c.max {
		return n, fmt.Errorf("decompresses to more than %d bytes", c.max)
	}
	return n, err
}

// dirHasContents reports whether dir exists and is non-empty. A missing
// directory is not an error: that is the normal case for a first install.
func dirHasContents(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("read target %s: %w", dir, err)
	}
	return len(entries) > 0, nil
}
