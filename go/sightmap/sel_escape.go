package sightmap

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// IDSelector returns a "#" selector that matches an element whose id is
// exactly id, escaping what a CSS identifier can't hold literally: ASCII
// punctuation and spaces get a backslash ("product.1" → `#product\.1`), and a
// leading digit or non-ASCII whitespace a hex escape ("1.circle" →
// `#\31 \.circle`). It returns "" for an empty id or one with an ASCII control
// character, which is never a usable id.
func IDSelector(id string) string {
	if id == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(id) + 4)
	b.WriteByte('#')
	for i := 0; i < len(id); {
		r, size := utf8.DecodeRuneInString(id[i:])
		last := i+size == len(id)
		switch {
		case r < ' ' || r == 0x7f:
			return ""
		case r >= utf8.RuneSelf:
			if unicode.IsSpace(r) {
				writeHexEscape(&b, r, last)
			} else {
				b.WriteString(id[i : i+size])
			}
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			if i == 0 {
				writeHexEscape(&b, r, last)
			} else {
				b.WriteRune(r)
			}
		case r == '-':
			// A bare "-" may lead only when an identifier start follows it,
			// so "-", "--x" and "-1" lead with `\-`.
			if i == 0 && !hyphenMayLead(id[1:]) {
				b.WriteString(`\-`)
			} else {
				b.WriteByte('-')
			}
		default:
			b.WriteByte('\\')
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}

// writeHexEscape writes r as a CSS hex escape. The space after the hex digits
// ends the escape, so a following hex digit isn't read into it; at the end of
// the selector nothing follows and the space is left off.
func writeHexEscape(b *strings.Builder, r rune, last bool) {
	b.WriteByte('\\')
	b.WriteString(strconv.FormatInt(int64(r), 16))
	if !last {
		b.WriteByte(' ')
	}
}

// hyphenMayLead reports whether rest, the id after a leading "-", starts with
// something that can open an identifier: a letter, "_", non-ASCII, or a
// character that will be escaped.
func hyphenMayLead(rest string) bool {
	if rest == "" {
		return false
	}
	c := rest[0]
	return !(c >= '0' && c <= '9') && c != '-' && c >= ' ' && c != 0x7f
}
