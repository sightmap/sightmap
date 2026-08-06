package atlas

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sightmap/sightmap/go/sightmap"
)

// ErrTargetNotEmpty reports that the install target already has contents and
// [Options.Replace] was not set. Callers match it with [errors.Is] to add
// their own "how to force this" wording.
var ErrTargetNotEmpty = errors.New("install target already exists and is not empty")

// Options configures [Install].
type Options struct {
	// IndexURL is the atlas index to install from. Empty means
	// [DefaultIndexURL]. Every other URL is derived from it, so pointing it at
	// a mirror or a test server redirects the whole install.
	IndexURL string
	// Target is the directory the corpus is installed into. Required — an
	// empty Target is an error, never "the current directory".
	Target string
	// Replace permits installing over a non-empty target. The target's
	// previous contents are *replaced*, not merged: files the entry no longer
	// publishes do not survive, so an install never yields a hybrid corpus.
	Replace bool
	// Client fetches the index and the files. Nil means [NewClient].
	Client *Client
}

// Result describes a completed install.
type Result struct {
	Slug     string   // the installed entry's slug
	Name     string   // its display name, if the atlas publishes one
	Ref      string   // the ref its files were fetched at
	Pinned   bool     // whether Ref is the entry's published commit
	Target   string   // the target directory, as the caller named it
	Files    []string // installed paths relative to Target, slash-separated, in index order
	Replaced bool     // whether an existing non-empty target was replaced
	Warnings []string // advisory problems with the atlas entry; already terminal-safe
}

// Label renders the entry for display: its slug, plus its published name when
// that adds anything. Terminal-safe.
func (r *Result) Label() string {
	if r.Name != "" && r.Name != r.Slug {
		return fmt.Sprintf("%s (%s)", SafeText(r.Slug), SafeText(r.Name))
	}
	return SafeText(r.Slug)
}

// Install installs one published atlas corpus into opts.Target.
//
// The order of operations is the contract:
//
//  1. The requested slug and the target are checked — a slug the user typed is
//     blamed on the user, before any atlas entry is in hand.
//  2. Local preconditions are checked *before any network I/O*, so a user with
//     an existing corpus and no connectivity gets the actionable refusal
//     instead of a dial timeout.
//  3. The index is fetched, gated on its schema version, and the entry is
//     validated fail-closed.
//  4. Every file is fetched (concurrently) before any is written.
//  5. The files are staged in a temporary directory and the staged corpus is
//     loaded, so a broken atlas entry is caught before it touches the target.
//  6. The staging directory is swapped into place with a rename.
//
// An install therefore either lands whole or leaves the target exactly as it
// was: no partial corpus, no half-overwritten one, and nothing installed that
// sightmap cannot read.
func Install(ctx context.Context, slug string, opts Options) (*Result, error) {
	if err := ValidateSlug(slug); err != nil {
		return nil, fmt.Errorf("invalid slug %q: %w", SafeText(slug), err)
	}
	if opts.Target == "" {
		return nil, errors.New("no install target: name the directory to install the corpus into")
	}
	indexURL := opts.IndexURL
	if indexURL == "" {
		indexURL = DefaultIndexURL
	}
	src, err := ParseSource(indexURL)
	if err != nil {
		return nil, err
	}
	client := opts.Client
	if client == nil {
		client = NewClient()
	}

	absTarget, err := filepath.Abs(opts.Target)
	if err != nil {
		return nil, fmt.Errorf("resolve target %s: %w", opts.Target, err)
	}
	state, err := inspectTarget(absTarget)
	if err != nil {
		return nil, err
	}
	if state == targetNonEmpty && !opts.Replace {
		return nil, fmt.Errorf("%w: %s", ErrTargetNotEmpty, opts.Target)
	}

	indexData, err := client.Fetch(ctx, indexURL, MaxIndexBytes)
	if err != nil {
		return nil, fmt.Errorf("fetch atlas index: %w", err)
	}
	idx, err := ParseIndex(indexData)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", SafeText(indexURL), err)
	}

	entry := idx.Entry(slug)
	if entry == nil {
		// Suggestions are built from slugs that have not been validated yet, so
		// SuggestSlugs drops any that would be refused at install time and the
		// message is rendered terminal-safe regardless.
		if suggestions := idx.SuggestSlugs(slug); len(suggestions) > 0 {
			for i, s := range suggestions {
				suggestions[i] = SafeText(s)
			}
			return nil, fmt.Errorf("no atlas entry with slug %q — closest matches:\n  %s",
				SafeText(slug), strings.Join(suggestions, "\n  "))
		}
		return nil, fmt.Errorf("no atlas entry with slug %q in the atlas index", SafeText(slug))
	}
	// Everything below splices index-supplied strings into URLs and local
	// paths, so validate all of it up front and fail closed.
	if err := entry.Validate(); err != nil {
		return nil, fmt.Errorf("atlas entry %q %w — refusing to install", SafeText(entry.Slug), err)
	}

	res := &Result{
		Slug:   entry.Slug,
		Name:   entry.Name,
		Ref:    src.RefFor(entry),
		Pinned: entry.Commit != "",
		Target: opts.Target,
	}
	if !res.Pinned {
		// Resolving the ref to a sha ourselves would mean talking to a git
		// hosting API — a second host, with its own auth and rate limits —
		// which would break the rule that --index alone redirects every fetch.
		// So an unpinned entry stays on the floating ref and says so.
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"atlas entry %q publishes no commit, so its files were fetched from the floating ref %q — the index and the files can straddle an atlas push",
			SafeText(entry.Slug), SafeText(src.Ref)))
	}

	bodies, err := fetchEntryFiles(ctx, client, src, res.Ref, entry)
	if err != nil {
		return nil, err
	}

	res.Files = make([]string, len(entry.Files))
	for i, p := range entry.Files {
		res.Files[i] = strings.TrimPrefix(p, corpusPrefix)
	}

	warnings, replaced, err := writeAtomically(absTarget, entry, res.Files, bodies, opts.Replace)
	if err != nil {
		return nil, err
	}
	res.Warnings = append(res.Warnings, warnings...)
	res.Replaced = replaced
	return res, nil
}

// fetchEntryFiles fetches every file of an entry before any of them is
// written, with bounded concurrency — one round trip per file, serialized, is
// the dominant cost of installing a corpus. Bodies land in an index-positioned
// slice so write order stays the published order, and the reported error is
// always the first file's, not whichever goroutine lost the race.
func fetchEntryFiles(ctx context.Context, client *Client, src Source, ref string, entry *Entry) ([][]byte, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	bodies := make([][]byte, len(entry.Files))
	errs := make([]error, len(entry.Files))

	var (
		mu    sync.Mutex
		total int64
	)
	sem := make(chan struct{}, FetchConcurrency)
	var wg sync.WaitGroup
	for i, p := range entry.Files {
		wg.Add(1)
		go func(i int, p string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			data, err := client.Fetch(ctx, src.FileURL(ref, entry.Slug, p), MaxFileBytes)
			if err != nil {
				errs[i] = err
				cancel()
				return
			}
			mu.Lock()
			total += int64(len(data))
			over := total > MaxEntryBytes
			mu.Unlock()
			if over {
				errs[i] = fmt.Errorf("atlas entry %q exceeds the %d-byte total size limit — refusing to install", SafeText(entry.Slug), int64(MaxEntryBytes))
				cancel()
				return
			}
			bodies[i] = data
		}(i, p)
	}
	wg.Wait()

	// Report the first *real* failure in published order. The in-flight
	// fetches this cancelled fail too, and their cancellation must not
	// out-shout the 404 that caused it.
	var canceled error
	for _, err := range errs {
		switch {
		case err == nil:
		case errors.Is(err, context.Canceled):
			if canceled == nil {
				canceled = err
			}
		default:
			return nil, err
		}
	}
	if canceled != nil {
		return nil, canceled
	}
	// A cancelled context can leave a hole with no recorded error.
	if err := ctx.Err(); err != nil && errors.Is(err, context.Canceled) {
		for i := range bodies {
			if bodies[i] == nil {
				return nil, fmt.Errorf("fetch %s: %w", entry.Files[i], err)
			}
		}
	}
	return bodies, nil
}

// writeAtomically stages the fetched corpus in a temporary directory beside
// the target, proves it loads, and renames it into place. Staging is what
// makes the install atomic: a mid-loop MkdirAll or WriteFile failure (an entry
// whose files collide, a full disk) leaves the target untouched instead of
// half-overwritten, replacing a target is a swap rather than a merge, and
// rolling back a broken atlas entry costs one RemoveAll.
func writeAtomically(target string, entry *Entry, rels []string, bodies [][]byte, replace bool) (warnings []string, replaced bool, err error) {
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, false, fmt.Errorf("create %s: %w", parent, err)
	}
	// The staging directory is a sibling of the target so the final rename
	// stays within one filesystem.
	staging, err := os.MkdirTemp(parent, ".sightmap-add-*")
	if err != nil {
		return nil, false, fmt.Errorf("create staging directory in %s: %w", parent, err)
	}
	defer os.RemoveAll(staging) // a no-op once the rename below has moved it

	for i, rel := range rels {
		dest := filepath.Join(staging, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return nil, false, fmt.Errorf("atlas entry %q: create directory for %s: %w", SafeText(entry.Slug), SafeText(entry.Files[i]), err)
		}
		if err := os.WriteFile(dest, bodies[i], 0o644); err != nil {
			return nil, false, fmt.Errorf("atlas entry %q: write %s: %w", SafeText(entry.Slug), SafeText(entry.Files[i]), err)
		}
	}
	if err := os.Chmod(staging, 0o755); err != nil {
		return nil, false, fmt.Errorf("set permissions on staging directory: %w", err)
	}

	warnings, err = checkStagedCorpus(staging, entry)
	if err != nil {
		return nil, false, err
	}

	replaced, err = swapIntoPlace(staging, target, replace)
	if err != nil {
		return nil, false, err
	}
	return warnings, replaced, nil
}

// checkStagedCorpus loads the staged corpus before it is installed. An entry
// that does not load is a *published* defect: without this check `add` writes
// it, prints success, and the user discovers the breakage later against their
// own files with no hint that the atlas entry is at fault.
//
// A load failure is fatal and the install is abandoned — the corpus is
// unusable, and while it is still staged rolling back costs nothing. Validation
// findings are only warnings: a corpus that loads is installable, the findings
// may be deliberate, and refusing them here would reject entries the atlas
// itself accepts. Either way the message names the atlas entry, not the user's
// project.
func checkStagedCorpus(staging string, entry *Entry) ([]string, error) {
	corpus, err := sightmap.Load(staging)
	if err != nil {
		// Rewrite the staging path out of the message — the user has no
		// business seeing a temp directory, and the corpus-relative path is
		// what they would report to the atlas.
		msg := strings.ReplaceAll(err.Error(), staging+string(filepath.Separator), corpusPrefix)
		return nil, fmt.Errorf("atlas entry %q publishes a corpus that does not load: %s — the atlas entry is broken; nothing was installed", SafeText(entry.Slug), msg)
	}
	var problems []string
	for _, f := range sightmap.Validate(corpus) {
		if f.IsError() {
			problems = append(problems, f.Error())
		}
	}
	if len(problems) == 0 {
		return nil, nil
	}
	return []string{fmt.Sprintf(
		"the corpus published by atlas entry %q has %d validation error(s) — this is a defect in the atlas entry, not in your project. First: %s",
		SafeText(entry.Slug), len(problems), SafeText(problems[0]))}, nil
}

// swapIntoPlace moves the staged corpus onto the target. The target's state is
// re-read here, immediately before the write, so a directory that was created
// or filled during the (multi-second) fetch is not silently overwritten.
func swapIntoPlace(staging, target string, replace bool) (replaced bool, err error) {
	state, err := inspectTarget(target)
	if err != nil {
		return false, err
	}
	switch state {
	case targetAbsent:
		if err := os.Rename(staging, target); err != nil {
			return false, fmt.Errorf("install into %s: %w", target, err)
		}
		return false, nil
	case targetEmpty:
		// os.Remove refuses a directory that gained an entry since the check,
		// so a file that appeared during the fetch turns into an error rather
		// than a silent overwrite.
		if err := os.Remove(target); err != nil {
			return false, fmt.Errorf("install into %s: %w", target, err)
		}
		if err := os.Rename(staging, target); err != nil {
			return false, fmt.Errorf("install into %s: %w", target, err)
		}
		return false, nil
	default:
		if !replace {
			return false, fmt.Errorf("%w: %s", ErrTargetNotEmpty, target)
		}
		// Replace, don't merge: the old tree moves aside whole, so files the
		// entry no longer publishes cannot survive into a hybrid corpus.
		backup := staging + ".replaced"
		if err := os.Rename(target, backup); err != nil {
			return false, fmt.Errorf("move existing %s aside: %w", target, err)
		}
		if err := os.Rename(staging, target); err != nil {
			// Put the user's corpus back before reporting the failure.
			if restoreErr := os.Rename(backup, target); restoreErr != nil {
				return false, fmt.Errorf("install into %s: %w (the previous contents are in %s)", target, err, backup)
			}
			return false, fmt.Errorf("install into %s: %w", target, err)
		}
		os.RemoveAll(backup)
		return true, nil
	}
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
