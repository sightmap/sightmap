package webmcp

// Route-glob helpers implementing the spec's "Route matching" rules. The
// produced regex SOURCE strings are embedded into generated bundles
// (require_view guards) and evaluated as JavaScript RegExp, so they must
// match what `new RegExp(source)` would compile in the page.

import (
	"regexp"
	"strings"
)

var regexEscaper = strings.NewReplacer(
	`.`, `\.`, `*`, `\*`, `+`, `\+`, `?`, `\?`, `^`, `\^`, `$`, `\$`,
	`{`, `\{`, `}`, `\}`, `(`, `\(`, `)`, `\)`, `|`, `\|`, `[`, `\[`,
	`]`, `\]`, `\`, `\\`,
)

func escapeRegex(s string) string { return regexEscaper.Replace(s) }

func normalizePath(p string) string {
	out := p
	if out == "" {
		out = "/"
	}
	out = strings.SplitN(out, "?", 2)[0]
	out = strings.SplitN(out, "#", 2)[0]
	if len(out) > 1 {
		out = strings.TrimRight(out, "/")
		if out == "" {
			out = "/"
		}
	}
	if !strings.HasPrefix(out, "/") {
		out = "/" + out
	}
	return out
}

var multiStar = regexp.MustCompile(`\*{2,}`)

func segmentToRegex(seg string) string {
	if seg == "*" || strings.HasPrefix(seg, ":") {
		return "[^/]+"
	}
	collapsed := multiStar.ReplaceAllString(seg, "*")
	parts := strings.Split(collapsed, "*")
	for i, p := range parts {
		parts[i] = escapeRegex(p)
	}
	return strings.Join(parts, "[^/]*")
}

// routeGlobRegexSource returns a regex source string for a sightmap route
// glob. JavaScript's RegExp.source escapes literal "/" outside character
// classes (so the source is valid in a /.../ literal); structural slashes
// below are written pre-escaped to match, while the "/" inside [^/] classes
// stays bare, exactly as JS reports it.
func routeGlobRegexSource(route string) string {
	norm := normalizePath(route)
	if norm == "/" {
		return `^\/$`
	}
	var parts []string
	for _, seg := range strings.Split(norm, "/") {
		if seg == "" {
			continue
		}
		if seg == "**" {
			parts = append(parts, `(?:\/[^/]+)*`)
		} else {
			parts = append(parts, `\/`+segmentToRegex(seg))
		}
	}
	src := "^" + strings.Join(parts, "") + "$"
	// A glob whose segments can all match zero segments must also match "/".
	if re, err := regexp.Compile(src); err == nil && re.MatchString("") {
		src = "^(?:" + strings.Join(parts, "") + `|\/)$`
	}
	return src
}

func pathMatchesRoute(pathname, route string) bool {
	re, err := regexp.Compile(routeGlobRegexSource(route))
	if err != nil {
		return false
	}
	return re.MatchString(normalizePath(pathname))
}
