package atlas

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// corpusPrefix is the only directory an archive may publish files under. It is
// the corpus directory's canonical name, not the install target's name — the
// prefix is stripped when a file lands, so --target may be anything.
const corpusPrefix = ".sightmap/"

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

// ValidateCorpusPath reports whether p is installable as a corpus file path.
// Only slash-separated paths under .sightmap/ are installable, and nothing
// that could escape the install target or carry an escape sequence into a
// filename is accepted. Every member of a fetched archive is checked here
// before it reaches the filesystem.
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
	if strings.HasPrefix(p, "/") {
		return fmt.Errorf("is an absolute path")
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

// Validate reports whether an entry is usable. An entry that fails here is
// dropped from search results rather than shown, because every field it
// publishes is either printed to a terminal or spliced into a URL. It is also
// the check the atlas publisher CI runs, so a merged entry is one every shipped
// CLI can present and install.
func (e *Entry) Validate() error {
	if err := ValidateSlug(e.Slug); err != nil {
		return fmt.Errorf("publishes an unsafe slug %q: %w", SafeText(e.Slug), err)
	}
	fields := []struct {
		label  string
		values []string
	}{
		{"name", []string{e.Name}},
		{"description", []string{e.Description}},
		{"last_verified", []string{e.LastVerified}},
		{"domains", e.Domains},
		{"categories", e.Categories},
	}
	for _, f := range fields {
		for _, v := range f.values {
			if v == "" {
				continue
			}
			if err := checkPrintable(v); err != nil {
				return fmt.Errorf("publishes an unsafe %s %q: %w", f.label, SafeText(v), err)
			}
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

// maxSafeTextRunes bounds how much index-supplied text one line may carry.
const maxSafeTextRunes = 200

// SafeText renders an index-supplied string for a terminal. Control characters
// (including ESC, which drives cursor movement, colour, and title changes) are
// replaced by their escaped form, invalid UTF-8 becomes U+FFFD, and the result
// is truncated. Every untrusted string that reaches stdout, stderr, or an error
// message passes through here. `find` and `list` print names, descriptions,
// domains, and categories straight off the index, which is far more untrusted
// text than an install ever prints.
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

// safeList renders a list of index-supplied strings as one comma-separated,
// terminal-safe line.
func safeList(values []string) string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		out = append(out, SafeText(v))
	}
	return strings.Join(out, ", ")
}
