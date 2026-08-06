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
	//
	// Because that is destructive, it is confined to directories that are
	// visibly a corpus: a target that holds anything which is not corpus
	// content, or that is the working directory, the home directory, or a
	// filesystem root, is refused with [ErrUnsafeReplace]. The full rule is in
	// the package doc.
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
//     instead of a dial timeout. Replacing a target is checked here too: a
//     directory that is not visibly a corpus is refused outright.
//  3. The index is fetched, gated on its schema version, and the entry is
//     validated fail-closed.
//  4. Every file is fetched (concurrently) before any is written.
//  5. The files are staged in a temporary directory beside the target, and the
//     staging directory is swapped into place with a rename. Whatever the
//     target held is moved aside, not deleted.
//  6. The *installed* corpus is loaded. Only once it does load are the
//     previous contents discarded; a corpus that does not load undoes the swap
//     and puts them back.
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
	if state == targetNonEmpty {
		if !opts.Replace {
			return nil, fmt.Errorf("%w: %s", ErrTargetNotEmpty, opts.Target)
		}
		// Replacing deletes whatever is there. Decide that against the
		// directory itself, before the fetch, so the refusal is the same with
		// or without connectivity.
		if err := checkReplaceable(absTarget); err != nil {
			return nil, err
		}
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
// the target, renames it into place, and proves the installed corpus loads.
// Staging is what makes the install atomic: a mid-loop MkdirAll or WriteFile
// failure (an entry whose files collide, a full disk) leaves the target
// untouched instead of half-overwritten, and replacing a target is a swap
// rather than a merge.
//
// Nothing the target held is deleted until the corpus that replaced it has
// loaded. The load check is therefore a real gate rather than a report: a
// broken atlas entry is undone, and the user gets their previous corpus back
// instead of the message that theirs is gone and the new one does not work.
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
	// A no-op once the rename below has moved the staging directory — and the
	// cleanup for the failed install a rollback moves back onto this path.
	defer os.RemoveAll(staging)

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

	sw, err := swapIntoPlace(staging, target, replace)
	if err != nil {
		return nil, false, err
	}
	warnings, err = checkInstalledCorpus(target, entry)
	if err != nil {
		if rollbackErr := sw.rollback(staging, target); rollbackErr != nil {
			return nil, false, fmt.Errorf("%w; and undoing the install failed: %v%s", err, rollbackErr, sw.strandedNote())
		}
		return nil, false, fmt.Errorf("%w%s", err, sw.undoneNote(target))
	}
	sw.commit()
	return warnings, sw.replaced, nil
}

// checkInstalledCorpus loads the corpus that has just landed on the target. An
// entry that does not load is a *published* defect: without this check `add`
// writes it, prints success, and the user discovers the breakage later against
// their own files with no hint that the atlas entry is at fault.
//
// A load failure is fatal and the install is undone — the corpus is unusable,
// and whatever the target held is still sitting beside it. Validation findings
// are only warnings: a corpus that loads is installable, the findings may be
// deliberate, and refusing them here would reject entries the atlas itself
// accepts. Either way the message names the atlas entry, not the user's
// project.
func checkInstalledCorpus(target string, entry *Entry) ([]string, error) {
	corpus, err := sightmap.Load(target)
	if err != nil {
		// Rewrite the target path out of the message: the corpus-relative path
		// is the one the user would quote when reporting the entry to the
		// atlas, and it does not change with --target.
		msg := strings.ReplaceAll(err.Error(), target+string(filepath.Separator), corpusPrefix)
		return nil, fmt.Errorf("atlas entry %q publishes a corpus that does not load: %s — the atlas entry is broken", SafeText(entry.Slug), msg)
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

// swap is a completed rename of the staged corpus onto the target, plus what
// it takes to undo it. It exists so the post-install load check can be a gate:
// the previous contents are kept, untouched, until the corpus that replaced
// them has proved it loads.
type swap struct {
	replaced   bool        // whether a non-empty target was replaced
	backup     string      // where its contents were moved; "" when there were none
	priorEmpty bool        // whether the target existed and was empty
	priorMode  os.FileMode // that empty directory's permissions, to recreate it
}

// commit discards what the target held. Called only after the installed corpus
// has loaded.
func (s *swap) commit() {
	if s.backup != "" {
		os.RemoveAll(s.backup)
	}
}

// rollback undoes the swap, leaving the target as it was found. The failed
// install is moved back to the staging path first rather than deleted, so the
// previous contents are never removed to make room for a restore that could
// itself fail; writeAtomically's deferred RemoveAll takes it from there.
func (s *swap) rollback(staging, target string) error {
	if err := os.Rename(target, staging); err != nil {
		return err
	}
	if s.backup != "" {
		return os.Rename(s.backup, target)
	}
	if s.priorEmpty {
		return os.Mkdir(target, s.priorMode)
	}
	return nil
}

// undoneNote says what a rolled-back install left behind, in the user's terms.
func (s *swap) undoneNote(target string) string {
	if s.backup != "" {
		return fmt.Sprintf("; the previous contents of %s were restored", target)
	}
	return "; nothing was installed"
}

// strandedNote points at the backup when a rollback could not put it back.
func (s *swap) strandedNote() string {
	if s.backup == "" {
		return ""
	}
	return fmt.Sprintf(" (the previous contents are in %s)", s.backup)
}

// swapIntoPlace moves the staged corpus onto the target. The target's state is
// re-read here, immediately before the write, so a directory that was created
// or filled during the (multi-second) fetch is not silently overwritten — and
// what --force would destroy is judged on what the target holds now, not on
// what it held before the fetch.
func swapIntoPlace(staging, target string, replace bool) (*swap, error) {
	state, err := inspectTarget(target)
	if err != nil {
		return nil, err
	}
	switch state {
	case targetAbsent:
		if err := os.Rename(staging, target); err != nil {
			return nil, fmt.Errorf("install into %s: %w", target, err)
		}
		return &swap{}, nil
	case targetEmpty:
		info, err := os.Stat(target)
		if err != nil {
			return nil, fmt.Errorf("install into %s: %w", target, err)
		}
		// os.Remove refuses a directory that gained an entry since the check,
		// so a file that appeared during the fetch turns into an error rather
		// than a silent overwrite.
		if err := os.Remove(target); err != nil {
			return nil, fmt.Errorf("install into %s: %w", target, err)
		}
		if err := os.Rename(staging, target); err != nil {
			return nil, fmt.Errorf("install into %s: %w", target, err)
		}
		return &swap{priorEmpty: true, priorMode: info.Mode().Perm()}, nil
	default:
		if !replace {
			return nil, fmt.Errorf("%w: %s", ErrTargetNotEmpty, target)
		}
		if err := checkReplaceable(target); err != nil {
			return nil, err
		}
		// Replace, don't merge: the old tree moves aside whole, so files the
		// entry no longer publishes cannot survive into a hybrid corpus.
		backup := staging + ".replaced"
		if err := os.Rename(target, backup); err != nil {
			return nil, fmt.Errorf("move existing %s aside: %w", target, err)
		}
		if err := os.Rename(staging, target); err != nil {
			// Put the user's corpus back before reporting the failure.
			if restoreErr := os.Rename(backup, target); restoreErr != nil {
				return nil, fmt.Errorf("install into %s: %w (the previous contents are in %s)", target, err, backup)
			}
			return nil, fmt.Errorf("install into %s: %w", target, err)
		}
		return &swap{replaced: true, backup: backup}, nil
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
