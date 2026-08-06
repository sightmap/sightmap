package atlas

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrUnsafeReplace reports that [Options.Replace] was set but the target is not
// a directory this package will replace. Replacing is destructive by design —
// the previous contents do not survive — so it is confined to directories that
// are visibly a corpus and nothing else. Callers match it with [errors.Is]; the
// wrapped message names the target, the reason, and what to do instead.
var ErrUnsafeReplace = errors.New("refusing to replace the install target")

// corpusDirs are the subdirectory names a corpus is made of: views/ is where
// the scaffold and every published entry put the view files, and snapshots/ and
// review/ are the two directories the corpus loader itself knows by name.
// Anything else at the top level of a directory is somebody's project, not a
// corpus.
var corpusDirs = map[string]bool{"views": true, "snapshots": true, "review": true}

// checkReplaceable decides whether a non-empty target may be replaced
// wholesale. Two rules, both fail-closed:
//
//  1. Location. A target that is the working directory, a directory containing
//     it, the user's home directory, a directory containing that, or a
//     filesystem root is refused whatever it holds. `--target .` in a project
//     root is a typo away from `--target ..`, and no amount of content
//     inspection makes deleting either of them the user's intent.
//  2. Contents. Every top-level entry must be corpus content: a .yaml/.yml
//     file, or one of [corpusDirs]. A .git directory, a .env file, a go.mod, a
//     src/ — any of them means the target is a project directory that happens
//     to hold YAML, and replacing it would delete work no atlas entry can put
//     back.
//
// The contents rule is an allowlist on purpose. A blocklist of alarming names
// only refuses the destruction someone thought of in advance; the allowlist
// refuses everything except the shape `add` itself installs, so the only
// directories `--force` can destroy are ones it could have written.
//
// It reads one level deep. That is enough to establish that the target *is* a
// corpus directory, which is the question; proving that every byte beneath it
// is corpus content would mean walking a snapshot set on every install.
func checkReplaceable(target string) error {
	if reason := unsafeReplaceLocation(target); reason != "" {
		return fmt.Errorf("%w %s: %s — replacing it deletes everything in it. Install into a corpus directory instead (for example %s)",
			ErrUnsafeReplace, target, reason, filepath.Join(target, ".sightmap"))
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return fmt.Errorf("read target %s: %w", target, err)
	}
	var foreign []string
	for _, e := range entries {
		if !isCorpusEntry(e) {
			foreign = append(foreign, e.Name())
		}
	}
	if len(foreign) == 0 {
		return nil
	}
	return fmt.Errorf("%w %s: it holds %s, which is not sightmap corpus content — replacing it deletes everything in it, so this looks like a project directory, not a corpus. Install into a corpus directory instead (for example %s), or delete %s yourself if you really did mean to replace it",
		ErrUnsafeReplace, target, nameList(foreign), filepath.Join(target, ".sightmap"), target)
}

// isCorpusEntry reports whether one top-level directory entry is something a
// sightmap corpus is made of. Only regular .yaml/.yml files and the corpus's
// own subdirectories qualify; a symlink never does, whatever it points at,
// because the rename that replaces the target would take the link with it.
//
// .DS_Store is the single exception: macOS writes it into any directory a user
// opens in Finder, it holds nothing of theirs, and refusing every corpus that
// has been looked at would make the guardrail read as a bug.
func isCorpusEntry(e os.DirEntry) bool {
	name := e.Name()
	if e.IsDir() {
		return corpusDirs[name]
	}
	if !e.Type().IsRegular() {
		return false
	}
	if name == ".DS_Store" {
		return true
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".yaml", ".yml":
		return true
	}
	return false
}

// unsafeReplaceLocation names why target is a directory no install may replace,
// or "" when the location itself is fine.
func unsafeReplaceLocation(target string) string {
	abs := resolveDir(target)
	if filepath.Dir(abs) == abs {
		return "it is a filesystem root"
	}
	if cwd, err := os.Getwd(); err == nil {
		switch c := resolveDir(cwd); {
		case c == abs:
			return "it is the current working directory"
		case isUnder(c, abs):
			return fmt.Sprintf("it contains the current working directory (%s)", cwd)
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		switch h := resolveDir(home); {
		case h == abs:
			return "it is your home directory"
		case isUnder(h, abs):
			return fmt.Sprintf("it contains your home directory (%s)", home)
		}
	}
	return ""
}

// resolveDir returns dir with symlinks resolved, falling back to a lexical
// clean when it cannot be resolved. Which spelling of a path the caller typed
// must not decide whether the guardrail fires: on macOS the working directory
// reads back as /private/var/… while a caller says /var/….
func resolveDir(dir string) string {
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		return filepath.Clean(r)
	}
	return filepath.Clean(dir)
}

// isUnder reports whether child is dir or lives inside it.
func isUnder(child, dir string) bool {
	rel, err := filepath.Rel(dir, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// nameList renders offending entry names for a message: up to three, quoted so
// a control character in a filename cannot reach the terminal raw, and a count
// for the rest.
func nameList(names []string) string {
	shown := names
	if len(shown) > 3 {
		shown = shown[:3]
	}
	quoted := make([]string, len(shown))
	for i, n := range shown {
		quoted[i] = fmt.Sprintf("%q", SafeText(n))
	}
	list := strings.Join(quoted, ", ")
	if len(names) > len(shown) {
		list += fmt.Sprintf(" and %d more", len(names)-len(shown))
	}
	return list
}
