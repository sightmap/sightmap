// add installs a published sightmap corpus from the community atlas
// (github.com/sightmap/atlas) into the current project, so a visitor can run
// one command from a gallery page and get a working .sightmap/ directory.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// defaultAtlasIndexURL is the published index of the community atlas. The
// raw-content base for per-file fetches is derived from the index URL, so
// --index alone redirects all fetching to a mirror or a test server.
const defaultAtlasIndexURL = "https://raw.githubusercontent.com/sightmap/atlas/main/index.json"

// atlasIndex is the subset of index.json that add consumes. Unknown fields
// are deliberately ignored so the atlas can grow metadata without breaking
// already-shipped CLIs.
type atlasIndex struct {
	SchemaVersion int          `json:"schema_version"`
	Entries       []atlasEntry `json:"entries"`
}

type atlasEntry struct {
	Slug   string   `json:"slug"`
	Name   string   `json:"name"`
	Commit string   `json:"commit"` // 40-char sha pinning the entry's content; empty = main
	Files  []string `json:"files"`  // corpus-relative paths, all under .sightmap/
}

// atlasCommitRe is what an entry's optional pinning commit must look like
// before it is spliced into a fetch URL.
var atlasCommitRe = regexp.MustCompile(`^[0-9a-f]{40}$`)

// runAdd is the entry point registered in main.go.
func runAdd(args []string) error {
	return runAddOut(args, os.Stdout)
}

// runAddOut is the testable core: it writes all output to out.
func runAddOut(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	indexURL := fs.String("index", defaultAtlasIndexURL, "Atlas index.json URL (for mirrors and tests)")
	target := fs.String("target", ".sightmap", "Directory to install the corpus into")
	force := fs.Bool("force", false, "Install into a non-empty target directory, overwriting existing files")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: sightmap add SLUG [--index URL] [--target DIR] [--force]\n\n")
		fmt.Fprintf(os.Stderr, "Installs a published sightmap corpus from the community atlas\n")
		fmt.Fprintf(os.Stderr, "(github.com/sightmap/atlas) into TARGET (default ./.sightmap).\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fs.Usage()
		return fmt.Errorf("expected a SLUG argument")
	}
	slug := rest[0]
	// Re-parse what follows the slug so `sightmap add SLUG --force` works —
	// the flag package stops at the first positional argument.
	if err := fs.Parse(rest[1:]); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q after SLUG", fs.Arg(0))
	}

	base, err := atlasRawBase(*indexURL)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 30 * time.Second}

	indexData, err := atlasGet(client, *indexURL)
	if err != nil {
		return fmt.Errorf("fetch atlas index: %w", err)
	}
	var idx atlasIndex
	if err := json.Unmarshal(indexData, &idx); err != nil {
		return fmt.Errorf("parse atlas index %s: %w", *indexURL, err)
	}
	if idx.SchemaVersion > 1 {
		return fmt.Errorf("atlas index %s has schema_version %d, but this sightmap only understands 1 — upgrade sightmap", *indexURL, idx.SchemaVersion)
	}

	var entry *atlasEntry
	for i := range idx.Entries {
		if idx.Entries[i].Slug == slug {
			entry = &idx.Entries[i]
			break
		}
	}
	if entry == nil {
		if suggestions := closestAtlasSlugs(slug, idx.Entries); len(suggestions) > 0 {
			return fmt.Errorf("no atlas entry with slug %q — closest matches:\n  %s",
				slug, strings.Join(suggestions, "\n  "))
		}
		return fmt.Errorf("no atlas entry with slug %q in the atlas index", slug)
	}

	// Everything below splices index-supplied strings into URLs and local
	// paths, so validate all of it up front and fail closed.
	if strings.ContainsAny(entry.Slug, "/\\") || strings.Contains(entry.Slug, "..") {
		return fmt.Errorf("atlas entry %q has an unsafe slug — refusing to install", entry.Slug)
	}
	ref := "main"
	if entry.Commit != "" {
		if !atlasCommitRe.MatchString(entry.Commit) {
			return fmt.Errorf("atlas entry %q has a malformed commit %q (want a 40-char lowercase sha) — refusing to install", entry.Slug, entry.Commit)
		}
		ref = entry.Commit
	}
	if len(entry.Files) == 0 {
		return fmt.Errorf("atlas entry %q lists no files", entry.Slug)
	}
	for _, p := range entry.Files {
		if err := atlasValidateFilePath(p); err != nil {
			return fmt.Errorf("atlas entry %q has an unsafe file path %q: %w — refusing to install", entry.Slug, p, err)
		}
	}

	// Refuse to install over an existing corpus unless asked.
	if dirEntries, readErr := os.ReadDir(*target); readErr == nil {
		if len(dirEntries) > 0 && !*force {
			return fmt.Errorf("target %s already exists and is not empty — pass --force to overwrite", *target)
		}
	} else if !os.IsNotExist(readErr) {
		return fmt.Errorf("read target %s: %w", *target, readErr)
	}

	// Fetch every file before writing any, so a network failure partway
	// through never leaves a half-installed corpus behind.
	type fetchedFile struct {
		rel  string // path under target (the .sightmap/ prefix stripped), slash-separated
		data []byte
	}
	fetched := make([]fetchedFile, 0, len(entry.Files))
	for _, p := range entry.Files {
		fileURL := base + "/" + ref + "/entries/" + url.PathEscape(entry.Slug) + "/" + escapeURLPathSegments(p)
		data, err := atlasGet(client, fileURL)
		if err != nil {
			return err
		}
		fetched = append(fetched, fetchedFile{rel: strings.TrimPrefix(p, ".sightmap/"), data: data})
	}

	for _, f := range fetched {
		dest := filepath.Join(*target, filepath.FromSlash(f.rel))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("create dir for %s: %w", f.rel, err)
		}
		if err := os.WriteFile(dest, f.data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", dest, err)
		}
		fmt.Fprintf(out, "  wrote  %s\n", dest)
	}

	label := entry.Slug
	if entry.Name != "" && entry.Name != entry.Slug {
		label = fmt.Sprintf("%s (%s)", entry.Slug, entry.Name)
	}
	files := fmt.Sprintf("%d files", len(fetched))
	if len(fetched) == 1 {
		files = "1 file"
	}
	fmt.Fprintf(out, "\nInstalled %s: %s → %s. Next:\n  sightmap validate\n", label, files, *target)
	return nil
}

// atlasRawBase derives the raw-content base URL — the prefix of
// <base>/<ref>/entries/<slug>/<path> — from the index URL. On
// raw.githubusercontent.com the index lives at <base>/<ref>/index.json, so
// both the filename and the ref segment are stripped; anywhere else (a mirror
// or an httptest server) the index lives at <base>/index.json.
func atlasRawBase(indexURL string) (string, error) {
	u, err := url.Parse(indexURL)
	if err != nil {
		return "", fmt.Errorf("invalid --index URL %q: %w", indexURL, err)
	}
	if err := atlasCheckScheme(u); err != nil {
		return "", err
	}
	dir := path.Dir(u.Path)
	if u.Host == "raw.githubusercontent.com" {
		dir = path.Dir(dir)
	}
	return u.Scheme + "://" + u.Host + strings.TrimSuffix(dir, "/"), nil
}

// atlasCheckScheme enforces HTTPS, with a plain-HTTP exception for loopback
// hosts so tests can point --index at an httptest server.
func atlasCheckScheme(u *url.URL) error {
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" {
		switch u.Hostname() {
		case "localhost", "127.0.0.1", "::1":
			return nil
		}
	}
	return fmt.Errorf("refusing non-HTTPS URL %s (plain http is allowed only for localhost)", u)
}

// atlasValidateFilePath rejects any entry file path that could escape the
// target directory or reach outside the corpus. Only slash-separated paths
// under .sightmap/ are installable.
func atlasValidateFilePath(p string) error {
	if strings.Contains(p, "\\") {
		return fmt.Errorf("contains a backslash")
	}
	if !strings.HasPrefix(p, ".sightmap/") {
		return fmt.Errorf("not under .sightmap/")
	}
	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case "":
			return fmt.Errorf("empty path segment")
		case ".", "..":
			return fmt.Errorf("relative path segment %q", seg)
		}
	}
	return nil
}

// closestAtlasSlugs returns up to 5 slugs resembling want, best first.
// Substring containment in either direction beats a shared prefix; ties
// break alphabetically. Good enough for "did you mean" on a typo'd slug.
func closestAtlasSlugs(want string, entries []atlasEntry) []string {
	type candidate struct {
		slug  string
		score int
	}
	w := strings.ToLower(want)
	var candidates []candidate
	for _, e := range entries {
		s := strings.ToLower(e.Slug)
		if s == "" {
			continue
		}
		score := 0
		if strings.Contains(s, w) || strings.Contains(w, s) {
			score = 100
		}
		prefix := 0
		for prefix < len(s) && prefix < len(w) && s[prefix] == w[prefix] {
			prefix++
		}
		score += prefix
		if score == 0 {
			continue
		}
		candidates = append(candidates, candidate{slug: e.Slug, score: score})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].slug < candidates[j].slug
	})
	if len(candidates) > 5 {
		candidates = candidates[:5]
	}
	slugs := make([]string, len(candidates))
	for i, c := range candidates {
		slugs[i] = c.slug
	}
	return slugs
}

// escapeURLPathSegments percent-escapes each segment of a slash-separated
// path while keeping the separators.
func escapeURLPathSegments(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

// atlasGet fetches a URL and returns its body, folding non-200 statuses into
// errors that name both the URL and the status.
func atlasGet(client *http.Client, rawURL string) ([]byte, error) {
	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %s", rawURL, resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GET %s: read body: %w", rawURL, err)
	}
	return data, nil
}
