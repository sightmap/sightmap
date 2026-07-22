package sightmap

import (
	"regexp"
	"strings"
)

// ApplyTransform applies a transform string to a property value.
// This mirrors the JS applyTransform function in the snapshot extraction script
// (cmd/sightmap/cmd_snapshot.go) and the extension extractors
// (cmd/sightmap/extension/{content,resolver}.js). Keep all four in sync.
// If transform is empty or val is empty, val is returned unchanged.
func ApplyTransform(val, transform string) string {
	if transform == "" || val == "" {
		return val
	}
	// match:REGEX — capture an arbitrary substring/enum. Returns capture group 1
	// if the pattern has one, else the full match. On no match or invalid
	// pattern, the value is passed through unchanged (like first_number).
	if p, ok := strings.CutPrefix(transform, "match:"); ok {
		re, err := regexp.Compile(p)
		if err != nil {
			return val
		}
		m := re.FindStringSubmatch(val)
		if m == nil {
			return val
		}
		if len(m) > 1 {
			return m[1]
		}
		return m[0]
	}
	switch transform {
	case "first_word":
		words := strings.Fields(val)
		if len(words) > 0 {
			return words[0]
		}
		return val
	case "last_word":
		words := strings.Fields(val)
		if len(words) > 0 {
			return words[len(words)-1]
		}
		return val
	case "first_number":
		re := regexp.MustCompile(`\d[\d,.]*`)
		if m := re.FindString(val); m != "" {
			return m
		}
		return val
	case "first_dollar":
		re := regexp.MustCompile(`\$[\d,.]+`)
		if m := re.FindString(val); m != "" {
			return m
		}
		return val
	case "number":
		re := regexp.MustCompile(`[^\d.]`)
		return re.ReplaceAllString(val, "")
	case "slug":
		lower := strings.ToLower(val)
		reSpaces := regexp.MustCompile(`\s+`)
		slug := reSpaces.ReplaceAllString(lower, "-")
		reNonSlug := regexp.MustCompile(`[^a-z0-9-]`)
		return reNonSlug.ReplaceAllString(slug, "")
	default:
		return val
	}
}
