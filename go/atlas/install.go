package atlas

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sightmap/sightmap/go/sightmap"
)

// DefaultArchiveURL is where an entry's corpus archive lives. {slug} is
// replaced by the requested slug. An install resolves this template and
// fetches exactly one URL, so it never reads the index: a search-index outage
// or a schema bump cannot stop an install, and the index can grow fields
// without a CLI release.
const DefaultArchiveURL = "https://raw.githubusercontent.com/sightmap/atlas/main/entries/{slug}.tar.gz"

// Extraction limits. The download cap ([MaxArchiveBytes]) bounds the wire; a
// gzip bomb is a few hundred kilobytes on the wire and gigabytes on disk, so
// what lands is capped separately.
const (
	// MaxCorpusBytes caps the total decompressed size of an archive.
	MaxCorpusBytes = 32 << 20 // 32 MiB
	// MaxCorpusFileBytes caps one file inside an archive.
	MaxCorpusFileBytes = 4 << 20 // 4 MiB
	// MaxArchiveEntries caps how many members an archive may hold.
	MaxArchiveEntries = 512
)

// ErrTargetNotEmpty reports that the install target already has contents.
// There is no flag that overrides it: deleting the directory is the user's
// call, not a flag's.
var ErrTargetNotEmpty = errors.New("install target already exists and is not empty")

// ErrNoSuchEntry reports that the atlas publishes no corpus for the requested
// slug.
var ErrNoSuchEntry = errors.New("no corpus published for this slug")

// Options configures [Install].
type Options struct {
	// ArchiveURL is the archive URL template, with {slug} substituted. Empty
	// means [DefaultArchiveURL]. Point it at a mirror or a test server to
	// redirect the install.
	ArchiveURL string
	// Target is the directory the corpus is installed into. Required — an
	// empty Target is an error, never "the current directory".
	Target string
	// Client fetches the archive. Nil means [NewClient].
	Client *Client
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
// The order of operations is the contract:
//
//  1. The slug and the target are checked. A non-empty target is refused
//     *before any network I/O*, so a user with an existing corpus and no
//     connectivity gets the actionable refusal instead of a dial timeout.
//  2. One archive is fetched, capped on the wire.
//  3. It is extracted into a temporary directory beside the target, with
//     every member's path validated and the decompressed size capped.
//  4. The staged corpus is loaded. One that does not load is an atlas defect:
//     the staging directory goes away and nothing is installed.
//  5. A rename moves the staged directory onto the target.
//
// The rename is what makes the install atomic. Nothing partial can land: until
// step 5 the target does not exist, and step 5 is one syscall.
func Install(ctx context.Context, slug string, opts Options) (*Result, error) {
	if err := ValidateSlug(slug); err != nil {
		return nil, fmt.Errorf("invalid slug %q: %w", SafeText(slug), err)
	}
	if opts.Target == "" {
		return nil, errors.New("no install target: name the directory to install the corpus into")
	}
	archiveURL, err := ResolveArchiveURL(opts.ArchiveURL, slug)
	if err != nil {
		return nil, err
	}

	absTarget, err := filepath.Abs(opts.Target)
	if err != nil {
		return nil, fmt.Errorf("resolve target %s: %w", opts.Target, err)
	}
	state, err := inspectTarget(absTarget)
	if err != nil {
		return nil, err
	}
	if state == targetNonEmpty {
		return nil, fmt.Errorf("%w: %s — delete it and run this again to install over it", ErrTargetNotEmpty, opts.Target)
	}

	client := opts.Client
	if client == nil {
		client = NewClient()
	}
	archive, err := client.Fetch(ctx, archiveURL, MaxArchiveBytes)
	if err != nil {
		var httpErr *HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%w: %q — browse %s, or search for the site: sightmap atlas find <domain>",
				ErrNoSuchEntry, SafeText(slug), AtlasURL)
		}
		return nil, err
	}

	parent := filepath.Dir(absTarget)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", parent, err)
	}
	// The staging directory is a sibling of the target so the final rename
	// stays within one filesystem.
	staging, err := os.MkdirTemp(parent, ".sightmap-add-*")
	if err != nil {
		return nil, fmt.Errorf("create staging directory in %s: %w", parent, err)
	}
	// A no-op once the rename below has moved the staging directory away.
	defer os.RemoveAll(staging)

	files, err := extractCorpus(archive, staging)
	if err != nil {
		return nil, fmt.Errorf("atlas entry %q: %w", SafeText(slug), err)
	}
	if err := checkStagedCorpus(staging, slug); err != nil {
		return nil, err
	}
	if err := os.Chmod(staging, 0o755); err != nil {
		return nil, fmt.Errorf("set permissions on staging directory: %w", err)
	}
	if err := moveIntoPlace(staging, absTarget); err != nil {
		return nil, err
	}
	return &Result{Slug: slug, Target: opts.Target, Source: archiveURL, Files: files}, nil
}

// ResolveArchiveURL substitutes slug into an archive URL template and applies
// the transport policy to the result, so a refused URL is reported before
// anything is dialled. An empty template means [DefaultArchiveURL].
func ResolveArchiveURL(template, slug string) (string, error) {
	if template == "" {
		template = DefaultArchiveURL
	}
	if err := ValidateSlug(slug); err != nil {
		return "", fmt.Errorf("invalid slug %q: %w", SafeText(slug), err)
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

// checkStagedCorpus loads the corpus that has just been extracted, before it
// is renamed onto the target. An entry that does not load is a *published*
// defect: without this check `add` writes it, prints success, and the user
// discovers the breakage later against their own files with no hint that the
// atlas entry is at fault.
func checkStagedCorpus(staging, slug string) error {
	if _, err := sightmap.Load(staging); err != nil {
		// Rewrite the staging path out of the message: the corpus-relative
		// path is the one the user would quote when reporting the entry to the
		// atlas, and it does not change with --target.
		msg := strings.ReplaceAll(err.Error(), staging+string(filepath.Separator), corpusPrefix)
		return fmt.Errorf("atlas entry %q publishes a corpus that does not load: %s — the atlas entry is broken, nothing was installed", SafeText(slug), msg)
	}
	return nil
}

// extractCorpus unpacks a .tar.gz into dest and returns the corpus-relative
// paths it wrote, sorted.
//
// Everything an archive controls is bounded here: how much it decompresses to,
// how large one member may be, how many members there are, what type they may
// be, and where they may land. A member path is validated with
// [ValidateCorpusPath] *and* re-checked for containment after it is joined
// onto dest, because the second check is the one that states the actual
// guarantee.
func extractCorpus(archive []byte, dest string) ([]string, error) {
	zr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("read archive: %w", err)
	}
	defer zr.Close()

	// The cap covers the whole tar stream, headers included, so a bomb fails
	// on the read that crosses it rather than after it is written.
	tr := tar.NewReader(&cappedReader{r: zr, max: MaxCorpusBytes})

	var files []string
	seen := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read archive: %w", err)
		}
		seen++
		if seen > MaxArchiveEntries {
			return nil, fmt.Errorf("archive holds more than %d files", MaxArchiveEntries)
		}
		isDir := hdr.Typeflag == tar.TypeDir
		rel, err := corpusMemberPath(hdr.Name, isDir)
		if err != nil {
			return nil, err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			// The corpus root itself needs no directory: dest already exists.
			if rel == "" {
				continue
			}
			path, err := safeJoin(dest, rel)
			if err != nil {
				return nil, err
			}
			if err := os.MkdirAll(path, 0o755); err != nil {
				return nil, fmt.Errorf("create %s: %w", SafeText(rel), err)
			}
		case tar.TypeReg:
			if rel == "" {
				return nil, fmt.Errorf("publishes %s as a file", corpusPrefix)
			}
			if hdr.Size > MaxCorpusFileBytes {
				return nil, fmt.Errorf("publishes %q at %d bytes, over the %d-byte file limit", SafeText(rel), hdr.Size, int64(MaxCorpusFileBytes))
			}
			path, err := safeJoin(dest, rel)
			if err != nil {
				return nil, err
			}
			if err := writeMember(path, tr, rel); err != nil {
				return nil, err
			}
			files = append(files, rel)
		default:
			// Symlinks, hardlinks, devices, and fifos are not corpus content.
			// Refusing beats skipping: an install that silently drops members
			// produces something other than what was published.
			return nil, fmt.Errorf("publishes %q, which is not a regular file or a directory", SafeText(hdr.Name))
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("publishes no files under %s", corpusPrefix)
	}
	sort.Strings(files)
	return files, nil
}

// writeMember writes one archive member, capped independently of the header's
// declared size so a lying header cannot outrun the limit.
func writeMember(path string, r io.Reader, rel string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create the directory for %s: %w", SafeText(rel), err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("write %s: %w", SafeText(rel), err)
	}
	n, copyErr := io.Copy(f, io.LimitReader(r, MaxCorpusFileBytes+1))
	closeErr := f.Close()
	switch {
	case copyErr != nil:
		return fmt.Errorf("write %s: %w", SafeText(rel), copyErr)
	case n > MaxCorpusFileBytes:
		return fmt.Errorf("publishes %q over the %d-byte file limit", SafeText(rel), int64(MaxCorpusFileBytes))
	case closeErr != nil:
		return fmt.Errorf("write %s: %w", SafeText(rel), closeErr)
	}
	return nil
}

// corpusMemberPath validates an archive member's name and returns its path
// relative to the corpus root, or "" for the corpus root directory itself.
func corpusMemberPath(name string, isDir bool) (string, error) {
	p := name
	if isDir {
		p = strings.TrimSuffix(p, "/")
	}
	p = strings.TrimPrefix(p, "./")
	if p == strings.TrimSuffix(corpusPrefix, "/") {
		return "", nil
	}
	if err := ValidateCorpusPath(p); err != nil {
		return "", fmt.Errorf("publishes an unsafe path %q: %w", SafeText(name), err)
	}
	return strings.TrimPrefix(p, corpusPrefix), nil
}

// safeJoin resolves rel under dest and proves the result stays there. This is
// the zip-slip guard, restated on the path that is actually written.
func safeJoin(dest, rel string) (string, error) {
	cleanDest := filepath.Clean(dest)
	path := filepath.Clean(filepath.Join(cleanDest, filepath.FromSlash(rel)))
	if path != cleanDest && !strings.HasPrefix(path, cleanDest+string(os.PathSeparator)) {
		return "", fmt.Errorf("publishes %q, which escapes the install directory", SafeText(rel))
	}
	return path, nil
}

// cappedReader fails the read that crosses max, so a compression bomb is
// detected while it decompresses rather than after it lands.
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

// moveIntoPlace renames the staged corpus onto the target. The target's state
// is re-read here, immediately before the rename, so a directory created or
// filled while the archive was in flight is not silently overwritten.
func moveIntoPlace(staging, target string) error {
	state, err := inspectTarget(target)
	if err != nil {
		return err
	}
	switch state {
	case targetAbsent:
	case targetEmpty:
		// os.Remove refuses a directory that gained an entry since the check,
		// so a file that appeared during the fetch turns into an error rather
		// than a silent overwrite.
		if err := os.Remove(target); err != nil {
			return fmt.Errorf("install into %s: %w", target, err)
		}
	default:
		return fmt.Errorf("%w: %s — delete it and run this again to install over it", ErrTargetNotEmpty, target)
	}
	if err := os.Rename(staging, target); err != nil {
		return fmt.Errorf("install into %s: %w", target, err)
	}
	return nil
}

type targetState int

const (
	targetAbsent targetState = iota
	targetEmpty
	targetNonEmpty
)

func inspectTarget(dir string) (targetState, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return targetAbsent, nil
		}
		return 0, fmt.Errorf("read target %s: %w", dir, err)
	}
	if len(entries) == 0 {
		return targetEmpty, nil
	}
	return targetNonEmpty, nil
}
