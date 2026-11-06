package sightmap

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// A route may end in query conditions (SEP-0025): PATH?KEY=VALUE&KEY=:name.
// Each condition reads the first value of its key in the URL's query string,
// decoded as url.query reads it (SEP-0008). Its value is matched and scored as
// a path segment is:
//
//	key=gift-cards  literal: the first value must equal it            (3)
//	key=sparkline*  a "*" run inside text is a wildcard within it     (3)
//	key=:name       present, with any value                           (2)
//	key=*           present, with any value                           (1)
//
// Keys the route does not name are ignored, so a route with no conditions
// matches whatever the query string holds.

type queryCondKind int

const (
	condLiteral queryCondKind = iota
	condGlob
	condParam
	condAny
)

type queryCond struct {
	key   string
	kind  queryCondKind
	value string         // decoded literal, or the :name for condParam
	re    *regexp.Regexp // condGlob
	raw   string         // the condition as written, for canonical spelling
}

// splitRoute separates a route's path from its query conditions. The path ends
// at the first "?"; hasQuery reports whether there was one.
func splitRoute(route string) (path, query string, hasQuery bool) {
	return strings.Cut(route, "?")
}

// routePath is a route's path part, without its query conditions.
func routePath(route string) string {
	p, _, _ := splitRoute(route)
	return p
}

// parseRouteQuery reads a route's query part. A malformed part is an error;
// validation reports it (route-query-invalid) and matching treats the route as
// matching nothing.
func parseRouteQuery(raw string) ([]queryCond, error) {
	if raw == "" {
		return nil, fmt.Errorf("the query part after \"?\" is empty")
	}
	var out []queryCond
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, "&") {
		rk, rv, ok := strings.Cut(part, "=")
		if !ok || rk == "" || rv == "" {
			return nil, fmt.Errorf("condition %q is not KEY=VALUE, KEY=:name or KEY=*", part)
		}
		key, err := url.QueryUnescape(rk)
		if err != nil {
			return nil, fmt.Errorf("condition %q: key: %v", part, err)
		}
		if seen[key] {
			return nil, fmt.Errorf("key %q is named more than once", key)
		}
		seen[key] = true
		c := queryCond{key: key, raw: part}
		switch {
		case strings.HasPrefix(rv, ":"):
			if len(rv) == 1 {
				return nil, fmt.Errorf("condition %q: a :name needs a name", part)
			}
			c.kind, c.value = condParam, rv[1:]
		case strings.Trim(rv, "*") == "":
			c.kind = condAny
		case strings.Contains(rv, "*"):
			re, err := queryGlob(rv)
			if err != nil {
				return nil, fmt.Errorf("condition %q: value: %v", part, err)
			}
			c.kind, c.re = condGlob, re
		default:
			v, err := url.QueryUnescape(rv)
			if err != nil {
				return nil, fmt.Errorf("condition %q: value: %v", part, err)
			}
			c.kind, c.value = condLiteral, v
		}
		out = append(out, c)
	}
	return out, nil
}

// queryGlob compiles a value holding "*" runs into an anchored regexp over the
// decoded value. The stars are read before decoding, so "%2A" is a literal star.
func queryGlob(raw string) (*regexp.Regexp, error) {
	var sb strings.Builder
	sb.WriteString("(?s)^")
	star := false
	for i, piece := range strings.Split(raw, "*") {
		if i > 0 && !star {
			sb.WriteString(".*")
			star = true
		}
		if piece == "" {
			continue
		}
		star = false
		v, err := url.QueryUnescape(piece)
		if err != nil {
			return nil, err
		}
		sb.WriteString(regexp.QuoteMeta(v))
	}
	sb.WriteByte('$')
	return regexp.Compile(sb.String())
}

// holds reports whether the condition is met by a URL's query values.
func (c queryCond) holds(q url.Values) bool {
	vs, ok := q[c.key]
	if !ok || len(vs) == 0 {
		return false
	}
	switch c.kind {
	case condLiteral:
		return vs[0] == c.value
	case condGlob:
		return c.re.MatchString(vs[0])
	default:
		return true
	}
}

// score is the condition's specificity, the score of the path segment it
// mirrors.
func (c queryCond) score() int {
	switch c.kind {
	case condParam:
		return 2
	case condAny:
		return 1
	default:
		return 3
	}
}

// matchRouteURL reports whether route matches a URL's path and query values.
// A route whose query part is malformed matches nothing.
func matchRouteURL(route, path string, q url.Values) bool {
	p, rawQuery, hasQuery := splitRoute(route)
	if !matchRoutePath(p, path) {
		return false
	}
	if !hasQuery {
		return true
	}
	conds, err := parseRouteQuery(rawQuery)
	if err != nil {
		return false
	}
	for _, c := range conds {
		if !c.holds(q) {
			return false
		}
	}
	return true
}

// canonicalRoute spells a route with its path normalized and its conditions
// sorted by key, so two spellings of one route compare equal.
func canonicalRoute(route string) string {
	p, rawQuery, hasQuery := splitRoute(route)
	p = normalizeRoutePath(p)
	if !hasQuery {
		return p
	}
	conds, err := parseRouteQuery(rawQuery)
	if err != nil {
		return p + "?" + rawQuery
	}
	sort.Slice(conds, func(i, j int) bool { return conds[i].key < conds[j].key })
	parts := make([]string, len(conds))
	for i, c := range conds {
		parts[i] = c.raw
	}
	return p + "?" + strings.Join(parts, "&")
}

// checkRouteQuery reports a malformed query part of a view or request route.
func checkRouteQuery(owner, route string) []ValidationError {
	_, rawQuery, hasQuery := splitRoute(route)
	if !hasQuery {
		return nil
	}
	if _, err := parseRouteQuery(rawQuery); err != nil {
		return []ValidationError{{
			Component: owner,
			Code:      "route-query-invalid",
			Severity:  SeverityError,
			Message:   fmt.Sprintf("%q route %q: %v; query conditions are KEY=VALUE, KEY=:name or KEY=* joined by &", owner, route, err),
		}}
	}
	return nil
}
