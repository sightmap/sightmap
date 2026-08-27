package webmcp

// Component-query parser — a direct port of webmcp/src/query.js. See that
// file (and the sightmap-browser skill) for the DSL this accepts.

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type queryPred struct {
	Prop  string
	Op    string
	Value string
	CI    bool
}

type queryLink struct {
	Name  string
	Preds []queryPred
	Index *int // nil when absent
}

type parsedQuery struct {
	Kind     string // "query" | "css"
	Selector string // css only
	Links    []queryLink
}

var queryNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*`)
var predBodyRe = regexp.MustCompile(`^([a-z][a-z0-9_]*)\s*(\^=|\*=|=)\s*([\s\S]*)$`)
var indexRe = regexp.MustCompile(`^#(\d+)`)
var trailingFlagRe = regexp.MustCompile(`\s+i$`)

func isQuoted(v string) bool {
	return len(v) >= 2 &&
		((strings.HasPrefix(v, `"`) && strings.HasSuffix(v, `"`)) ||
			(strings.HasPrefix(v, `'`) && strings.HasSuffix(v, `'`)))
}

func parsePred(body string) (queryPred, error) {
	m := predBodyRe.FindStringSubmatch(body)
	if m == nil {
		return queryPred{}, fmt.Errorf("cannot parse predicate \"[%s]\"", body)
	}
	value := strings.TrimSpace(m[3])
	ci := false
	noFlag := trailingFlagRe.ReplaceAllString(value, "")
	if noFlag != value && (isQuoted(noFlag) || !isQuoted(value)) {
		ci = true
		value = noFlag
	}
	if isQuoted(value) {
		value = value[1 : len(value)-1]
	}
	return queryPred{Prop: m[1], Op: m[2], Value: value, CI: ci}, nil
}

func findPredEnd(s string) (int, error) {
	var quote byte
	for i := 1; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
		} else if c == '"' || c == '\'' {
			quote = c
		} else if c == ']' {
			return i, nil
		}
	}
	return 0, fmt.Errorf("unterminated predicate in %q", s)
}

func parseLink(text string) (queryLink, error) {
	name := queryNameRe.FindString(text)
	if name == "" {
		return queryLink{}, fmt.Errorf("expected a component name at %q", text)
	}
	link := queryLink{Name: name}
	rest := text[len(name):]
	for strings.HasPrefix(rest, "[") {
		end, err := findPredEnd(rest)
		if err != nil {
			return queryLink{}, err
		}
		pred, err := parsePred(rest[1:end])
		if err != nil {
			return queryLink{}, err
		}
		link.Preds = append(link.Preds, pred)
		rest = rest[end+1:]
	}
	if strings.HasPrefix(rest, "#") {
		m := indexRe.FindStringSubmatch(rest)
		if m == nil {
			return queryLink{}, fmt.Errorf("expected an index after \"#\" in %q", text)
		}
		n, _ := strconv.Atoi(m[1])
		link.Index = &n
		rest = rest[len(m[0]):]
	}
	if rest != "" {
		return queryLink{}, fmt.Errorf("unexpected trailing %q in query link %q", rest, text)
	}
	return link, nil
}

// splitLinks splits on whitespace outside [...] predicates.
func splitLinks(q string) []string {
	var parts []string
	depth := 0
	var quote byte
	var cur strings.Builder
	for i := 0; i < len(q); i++ {
		c := q[i]
		switch {
		case quote != 0:
			cur.WriteByte(c)
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			cur.WriteByte(c)
			quote = c
		case c == '[':
			depth++
			cur.WriteByte(c)
		case c == ']':
			depth--
			cur.WriteByte(c)
		case (c == ' ' || c == '\t' || c == '\n' || c == '\r') && depth == 0:
			if cur.Len() > 0 {
				parts = append(parts, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}

var predStripRe = regexp.MustCompile(`\[[^\]]*\]`)

func parseQuery(q string) (parsedQuery, error) {
	text := strings.TrimSpace(q)
	if strings.HasPrefix(text, "css:") {
		selector := strings.TrimSpace(text[4:])
		if selector == "" {
			return parsedQuery{}, fmt.Errorf("empty selector in %q", q)
		}
		return parsedQuery{Kind: "css", Selector: selector}, nil
	}
	if text == "" {
		return parsedQuery{}, fmt.Errorf("empty component query")
	}
	if strings.Contains(predStripRe.ReplaceAllString(text, ""), ">") {
		return parsedQuery{}, fmt.Errorf(
			"%q: the child combinator \">\" is not supported — whitespace is a descendant combinator", q)
	}
	var links []queryLink
	for _, part := range splitLinks(text) {
		link, err := parseLink(part)
		if err != nil {
			return parsedQuery{}, err
		}
		links = append(links, link)
	}
	if len(links) == 0 {
		return parsedQuery{}, fmt.Errorf("empty component query")
	}
	return parsedQuery{Kind: "query", Links: links}, nil
}
