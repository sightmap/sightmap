package sightmap

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// IDSelector returns a selector that matches an element whose id is exactly
// id, written within the portable selector profile (SEP-0021): "#" plus the id
// with each ASCII punctuation character backslash-escaped ("product.1" →
// `#product\.1`). The profile has no hex escapes, so an id that cannot be
// written that way (a leading digit, a space) becomes `[id="…"]`. It returns
// "" for an empty id or one no profile selector can express (control
// characters such as a newline).
func IDSelector(id string) string {
	if id == "" {
		return ""
	}
	if s, ok := hashIDSelector(id); ok {
		return s
	}
	q, ok := QuoteSelectorString(id)
	if !ok {
		return ""
	}
	return "[id=" + q + "]"
}

// hashIDSelector writes id as "#" + an identifier, escaping every printable
// ASCII character that is not an identifier character. It fails on a leading
// digit (only a hex escape can write one) and on whitespace or control
// characters (not escapable in the profile).
func hashIDSelector(id string) (string, bool) {
	var b strings.Builder
	b.Grow(len(id) + 4)
	b.WriteByte('#')
	for i := 0; i < len(id); {
		r, size := utf8.DecodeRuneInString(id[i:])
		switch {
		case r >= utf8.RuneSelf:
			if unicode.IsSpace(r) {
				return "", false
			}
			b.WriteString(id[i : i+size])
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			if i == 0 {
				return "", false
			}
			b.WriteRune(r)
		case r == '-':
			// A bare "-" may lead only when an identifier start follows it,
			// so "-", "--x" and "-1" lead with `\-`.
			if i == 0 && !hyphenMayLead(id[1:]) {
				b.WriteString(`\-`)
			} else {
				b.WriteByte('-')
			}
		case r > ' ' && r < 0x7f:
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			return "", false
		}
		i += size
	}
	return b.String(), true
}

// hyphenMayLead reports whether rest, the id after a leading "-", starts with
// something that can open an identifier: a letter, "_", non-ASCII, or a
// character that will be escaped.
func hyphenMayLead(rest string) bool {
	if rest == "" {
		return false
	}
	c := rest[0]
	return !(c >= '0' && c <= '9') && c != '-' && c > ' ' && c != 0x7f
}

// QuoteSelectorString quotes s as a double-quoted selector string, escaping
// `"` and `\`. It reports false when s holds a character the profile's quoted
// form cannot carry: an ASCII control character, or whitespace other than a
// plain space.
func QuoteSelectorString(s string) (string, bool) {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < ' ' || r == 0x7f:
			return "", false
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String(), true
}
