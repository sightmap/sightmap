package grow

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/sightmap/sightmap/go/explore"
)

var stopWords = map[string]bool{}

func init() {
	for _, w := range strings.Fields("the a an to of and or for in on view details div ul ol li col row sm md lg xs wrapper container inner fluid block primary default btn") {
		stopWords[w] = true
	}
}

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]+`)
var hasDigit = regexp.MustCompile(`\d`)
var hashLike = regexp.MustCompile(`\d{4,}|[a-f0-9]{8,}|css-|sc-|emotion|__[a-z0-9]{5,}`)

func words(s string) []string {
	var out []string
	for _, w := range strings.Fields(nonAlnum.ReplaceAllString(s, " ")) {
		if stopWords[strings.ToLower(w)] || hasDigit.ReplaceAllString(w, "") == "" {
			continue
		}
		out = append(out, w)
	}
	return out
}

func pascal(ws []string) string {
	var b strings.Builder
	for _, w := range ws {
		if w == "" {
			continue
		}
		b.WriteString(strings.ToUpper(w[:1]))
		b.WriteString(strings.ToLower(w[1:]))
	}
	return b.String()
}

var kindSuffix = map[string]string{"button": "Button", "link": "Link", "input": "Input", "select": "Select", "nav": "Link", "card": "Card"}

// GroupName names a group: a single control from its own text, a repeated
// control from its container hook (or from the shared text when there is no
// hook), then the kind's suffix.
func GroupName(hook, tag, role string, members []*explore.Node, kind string, count int) string {
	suffix := kindSuffix[kind]
	if suffix == "" {
		suffix = "Item"
	}
	base := ""
	if count == 1 && len(members) > 0 {
		base = nameFromNode(members[0])
	}
	if base == "" && hook == "" && len(members) > 0 {
		shared := true
		for _, m := range members[1:] {
			if m.Name != members[0].Name {
				shared = false
				break
			}
		}
		if shared {
			base = nameFromNode(members[0])
		}
	}
	if base == "" {
		h := hook
		if i := strings.IndexAny(h, "["); i >= 0 {
			h = h[:i]
		}
		tagOnly := !strings.ContainsAny(h, ".#")
		var ws []string
		if tagOnly {
			ws = words(h)
		} else {
			ws = words(h[strings.IndexAny(h, ".#"):])
			if len(ws) > 2 {
				ws = ws[len(ws)-2:]
			}
		}
		base = pascal(ws)
	}
	if base == "" {
		ws := words(tag)
		if len(ws) > 0 {
			base = pascal(ws[:1])
		}
	}
	if base == "" {
		base = "Element"
	}
	if strings.HasSuffix(base, suffix) {
		return base
	}
	return base + suffix
}

func nameFromNode(m *explore.Node) string {
	src := m.Name
	for _, alt := range []string{m.Attrs["aria-label"], m.Attrs["placeholder"], m.Attrs["name"], m.Attrs["id"]} {
		if src == "" {
			src = alt
		}
	}
	ws := words(src)
	if len(ws) > 3 {
		ws = ws[:3]
	}
	return pascal(ws)
}

// Stability scores a selector: data attributes and ids first, generated-looking
// tokens last.
func Stability(sel string) int {
	s := 0
	if regexp.MustCompile(`\[data-(test|testid|cy|qa|id|sentry)`).MatchString(sel) {
		s += 10
	}
	if regexp.MustCompile(`^#[a-zA-Z][\w-]*$`).MatchString(sel) && !regexp.MustCompile(`\d{3,}`).MatchString(sel) {
		s += 8
	}
	if strings.Contains(sel, "[aria-label=") || strings.Contains(sel, "[name=") || strings.Contains(sel, "[href=") {
		s += 6
	}
	if regexp.MustCompile(`\[id[\^$*]=`).MatchString(sel) {
		s += 5
	}
	if regexp.MustCompile(`^[a-z]+\.[a-zA-Z][\w-]*$`).MatchString(sel) {
		s += 3
	}
	if hashLike.MatchString(sel) {
		s -= 8
	}
	return s
}

var indexHTML = regexp.MustCompile(`^index\.html?$`)

// RoutePattern generalises a page URL into a view route: every path segment
// that contains a digit becomes "*" (one segment), except index.html.
func RoutePattern(pageURL string) string {
	u, err := url.Parse(pageURL)
	if err != nil {
		return "/"
	}
	var segs []string
	for _, s := range strings.Split(u.Path, "/") {
		if s == "" {
			continue
		}
		if hasDigit.MatchString(s) && !indexHTML.MatchString(s) {
			segs = append(segs, "*")
		} else {
			segs = append(segs, s)
		}
	}
	return "/" + strings.Join(segs, "/")
}

// RouteName derives a view name from a route pattern: the last two literal
// segments in PascalCase, "Detail" when a wildcard precedes more segments,
// "Page" when the wildcard is last, "Home" for the root.
func RouteName(route string) string {
	segs := strings.Split(strings.Trim(route, "/"), "/")
	var literal []string
	star := -1
	for i, s := range segs {
		if s == "*" {
			if star < 0 {
				star = i
			}
			continue
		}
		if s == "" || indexHTML.MatchString(s) {
			continue
		}
		literal = append(literal, s)
	}
	if len(literal) == 0 {
		if star >= 0 {
			return "Page"
		}
		return "Home"
	}
	if len(literal) > 2 {
		literal = literal[len(literal)-2:]
	}
	var ws []string
	for _, l := range literal {
		ws = append(ws, words(strings.TrimSuffix(strings.TrimSuffix(l, ".html"), ".htm"))...)
	}
	if len(ws) > 3 {
		ws = ws[:3]
	}
	name := pascal(ws)
	if name == "" {
		name = "Page"
	}
	last := len(segs) - 1
	switch {
	case star >= 0 && star < last:
		name += "Detail"
	case star == last:
		name += "Page"
	}
	return name
}
