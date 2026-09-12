package sightmap

import (
	"fmt"
	"strings"
)

// SplitFieldPath splits a RequestPropertyDef.Field body-source path into its
// dot-separated segments, honoring backslash escapes: "\." is a literal dot
// within a segment (for a JSON object key that itself contains a dot), "\\"
// is a literal backslash, and an unescaped "." breaks a new segment. Any
// other escape sequence is an error.
//
// This is purely mechanical — it does not validate segment content (an empty
// segment from "a..b" splits into ["a", "", "b"] rather than erroring) or
// resolve anything against a JSON document; walkJSONPath does that, and a
// caller that needs its own validation (e.g. rejecting an empty segment) does
// so over the returned slice. An empty field splits to nil (no segments), the
// same "no path" case a header-source Field or a pattern-only body extraction
// already treats specially at the call site.
func SplitFieldPath(field string) ([]string, error) {
	if field == "" {
		return nil, nil
	}

	var segs []string
	var cur strings.Builder
	escaped := false

	for i := 0; i < len(field); i++ {
		c := field[i]
		if escaped {
			switch c {
			case '.', '\\':
				cur.WriteByte(c)
			default:
				return nil, fmt.Errorf("sightmap: invalid escape %q in field path %q at position %d", string(c), field, i)
			}
			escaped = false
			continue
		}
		switch c {
		case '\\':
			escaped = true
		case '.':
			segs = append(segs, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	if escaped {
		return nil, fmt.Errorf("sightmap: field path %q ends with an unterminated escape", field)
	}
	segs = append(segs, cur.String())
	return segs, nil
}
