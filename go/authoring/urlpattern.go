package authoring

import (
	"regexp"
	"strings"
)

// NormalizePath collapses a concrete URL path into a route pattern by replacing
// dynamic segments with "*". A segment is dynamic if it is all digits, a UUID,
// or a slug-like token (8+ lowercase alphanumeric/dash chars) containing at
// least one digit. Used by `discover` to fold many concrete links into the
// handful of route patterns a corpus would declare.
func NormalizePath(path string) string {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	for i, seg := range segments {
		if isDynamicSegment(seg) {
			segments[i] = "*"
		}
	}
	return "/" + strings.Join(segments, "/")
}

var (
	reAllDigits = regexp.MustCompile(`^\d+$`)
	reSlugDyn   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{7,}$`) // 8+ chars
	reUUID      = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	reHasDigit  = regexp.MustCompile(`\d`)
)

func isDynamicSegment(seg string) bool {
	if seg == "" {
		return false
	}
	lower := strings.ToLower(seg)
	return reAllDigits.MatchString(seg) ||
		reUUID.MatchString(lower) ||
		(reSlugDyn.MatchString(lower) && reHasDigit.MatchString(lower))
}
