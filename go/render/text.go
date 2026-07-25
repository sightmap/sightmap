package render

import "unicode/utf8"

// TruncateRunes truncates s to at most maxRunes Unicode code points,
// appending "…" when truncation occurs.
func TruncateRunes(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "…"
}
