package atlas

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// corpusPrefix is the only directory an entry may publish files under. It is
// the corpus directory's canonical name, not the install target's name — the
// prefix is stripped when the file lands, so --target may be anything.
const corpusPrefix = ".sightmap/"

// commitRe is what an entry's optional pinning commit must look like before it
// is spliced into a fetch URL.
var commitRe = regexp.MustCompile(`^[0-9a-f]{40}$`)

// ValidateSlug reports whether s is installable as an atlas slug. A slug is
// spliced into a fetch URL and used verbatim in terminal output, so it must be
// valid UTF-8, non-empty, free of path separators and traversal, and free of
// control characters — an ESC byte in a slug is a terminal-escape injection,
// the vector git and npm both patched.
func ValidateSlug(s string) error {
	if s == "" {
		return fmt.Errorf("is empty")
	}
	if err := checkPrintable(s); err != nil {
		return err
	}
	if strings.ContainsAny(s, `/\`) {
		return fmt.Errorf("contains a path separator")
	}
	if strings.Contains(s, "..") {
		return fmt.Errorf("contains %q", "..")
	}
	return nil
}

// ValidateCommit reports whether s is usable as an entry's pinning commit: a
// 40-character lowercase sha, or empty for an unpinned entry.
func ValidateCommit(s string) error {
	if s == "" {
		return nil
	}
	if !commitRe.MatchString(s) {
		return fmt.Errorf("is not a 40-char lowercase sha")
	}
	return nil
}

// ValidateCorpusPath reports whether p is installable as an entry file path.
// Only slash-separated paths under .sightmap/ are installable, and nothing
// that could escape the install target or carry an escape sequence into a
// filename is accepted.
func ValidateCorpusPath(p string) error {
	if p == "" {
		return fmt.Errorf("is empty")
	}
	if err := checkPrintable(p); err != nil {
		return err
	}
	if strings.Contains(p, `\`) {
		return fmt.Errorf("contains a backslash")
	}
	if !strings.HasPrefix(p, corpusPrefix) {
		return fmt.Errorf("is not under %s", corpusPrefix)
	}
	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case "":
			return fmt.Errorf("has an empty path segment")
		case ".", "..":
			return fmt.Errorf("has a relative path segment %q", seg)
		}
	}
	return nil
}

// Validate reports whether an entry is installable. Everything an entry
// contributes to a URL or a filesystem path is checked here, fail-closed, so
// that neither the CLI nor the atlas publisher CI has to know the rules
// piecemeal.
func (e *Entry) Validate() error {
	if err := ValidateSlug(e.Slug); err != nil {
		return fmt.Errorf("publishes an unsafe slug %q: %w", SafeText(e.Slug), err)
	}
	if err := ValidateCommit(e.Commit); err != nil {
		return fmt.Errorf("has a malformed commit %q: %w", SafeText(e.Commit), err)
	}
	if len(e.Files) == 0 {
		return fmt.Errorf("lists no files")
	}
	if len(e.Files) > MaxEntryFiles {
		return fmt.Errorf("lists %d files, more than the %d-file limit", len(e.Files), MaxEntryFiles)
	}
	for _, p := range e.Files {
		if err := ValidateCorpusPath(p); err != nil {
			return fmt.Errorf("publishes an unsafe file path %q: %w", SafeText(p), err)
		}
	}
	return nil
}

// checkPrintable rejects invalid UTF-8 and any C0/C1 control character,
// including ESC and DEL.
func checkPrintable(s string) error {
	if !utf8.ValidString(s) {
		return fmt.Errorf("is not valid UTF-8")
	}
	for _, r := range s {
		if isControl(r) {
			return fmt.Errorf("contains a control character (%s)", escapeRune(r))
		}
	}
	return nil
}

func isControl(r rune) bool {
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}

func escapeRune(r rune) string {
	if r <= 0xff {
		return fmt.Sprintf(`\x%02x`, r)
	}
	return fmt.Sprintf(`\u%04x`, r)
}

// maxSafeTextRunes bounds how much index-supplied text one message may carry.
const maxSafeTextRunes = 120

// SafeText renders an index-supplied string for a terminal. Control characters
// (including ESC, which drives cursor movement, colour, and title changes) are
// replaced by their escaped form, invalid UTF-8 becomes U+FFFD, and the result
// is truncated. Every untrusted string that reaches stdout, stderr, or an error
// message passes through here — the validators fail an install closed, but the
// "did you mean" line and the entry's display name are printed before or
// without validation.
func SafeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	n := 0
	for _, r := range s {
		if n == maxSafeTextRunes {
			b.WriteString("…")
			break
		}
		n++
		switch {
		case r == utf8.RuneError:
			b.WriteRune('�')
		case isControl(r):
			b.WriteString(escapeRune(r))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
