package sightmap

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// RequestMatch records which RequestDef classified an observed request, plus any
// values its properties[] extracted from that request's live traffic. It is the
// request-side analogue of ComponentMatch: a projection of the matched
// definition carrying the fields a consumer needs to link a classification back
// to what produced it (Name), the corpus context worth surfacing (Memory, Tags),
// and the extracted content (Properties). Requests, like components, carry
// live-extracted properties — unlike MessageMatch, which has none.
type RequestMatch struct {
	Name       string
	Memory     []string
	Tags       []string
	Properties []PropertyValue
}

// RequestsForRecord returns every RequestDef that classifies the observed
// request rec, as RequestMatch projections. Identity matching is delegated to
// RequestsForURL (route glob + optional method) so there is one identity path;
// each matched def then has its properties[] resolved against rec's live payload
// (headers/bodies), and the resolved values are folded into the match.
//
// All matches apply, in the same order RequestsForURL returns them (global
// requests first, then view-scoped). A def with no properties[] yields a match
// with nil Properties; a property that doesn't resolve is silently omitted, per
// SEP-0005 — so an incomplete record (bodies not captured) degrades to fewer
// properties, never an error.
func (c *Corpus) RequestsForRecord(rec Request) []RequestMatch {
	defs := c.RequestsForURL(rec.URL, rec.Method)
	if len(defs) == 0 {
		return nil
	}
	out := make([]RequestMatch, 0, len(defs))
	for i := range defs {
		out = append(out, RequestMatch{
			Name:       defs[i].Name,
			Memory:     defs[i].Memory,
			Tags:       defs[i].Tags,
			Properties: defs[i].ExtractProperties(rec),
		})
	}
	return out
}

// ExtractProperties resolves d's declared properties[] against the observed
// request rec, returning the values that resolved, in declaration order. A
// property is silently omitted (never an error) when its source is unpopulated,
// its field doesn't resolve, or its pattern finds no match — this is the SEP-0005
// live-traffic contract, where whether a body or header is even available depends
// on the capture layer.
//
// Resolution is source → field → pattern → transform: the source selects a body
// or header block on rec; field selects within it (a JSON dot-path for a body, a
// case-insensitive header name for a header block); an optional RE2 pattern
// refines what field resolved (capture group 1 when present, else the whole
// match) or scans the raw source text when field is absent; an optional transform
// post-processes the string. The reserved identity names (status/method/duration)
// are not handled here — they are a signal-layer concern and need no properties[]
// declaration; a property that happens to be named after one still extracts from
// its own source (the shadowing validation warns about).
func (d *RequestDef) ExtractProperties(rec Request) []PropertyValue {
	if len(d.Properties) == 0 {
		return nil
	}
	var out []PropertyValue
	for _, p := range d.Properties {
		if v, ok := resolveRequestProperty(p, rec); ok {
			out = append(out, PropertyValue{Name: p.Name, Value: v})
		}
	}
	return out
}

// resolveRequestProperty applies one property definition to rec, returning the
// extracted value and whether it resolved.
func resolveRequestProperty(p RequestPropertyDef, rec Request) (string, bool) {
	raw, ok := resolveSourceValue(p, rec)
	if !ok {
		return "", false
	}
	if p.Pattern != "" {
		matched, ok := applyPattern(p.Pattern, raw)
		if !ok {
			return "", false
		}
		raw = matched
	}
	if raw == "" {
		return "", false
	}
	return ApplyTransform(raw, p.Transform), true
}

// resolveSourceValue resolves the source+field half of a property to a raw
// string (before pattern/transform). For a body source with a field it walks the
// JSON dot-path; with no field it returns the whole raw body for a pattern to
// scan. For a header source it looks up the (required) named header.
func resolveSourceValue(p RequestPropertyDef, rec Request) (string, bool) {
	switch p.Source {
	case "req.body", "rsp.body":
		body := rec.RspBody
		if p.Source == "req.body" {
			body = rec.ReqBody
		}
		if body == nil {
			return "", false
		}
		if p.Field == "" {
			// No field: the pattern (which validation requires when field is
			// absent) scans the raw body text.
			return body.Content, true
		}
		return walkJSONPath(body.Content, p.Field)
	case "req.headers", "rsp.headers":
		headers := rec.RspHeaders
		if p.Source == "req.headers" {
			headers = rec.ReqHeaders
		}
		return lookupHeader(headers, p.Field)
	default:
		return "", false
	}
}

// lookupHeader returns the value of the header named name, matched
// case-insensitively. Duplicate headers are joined with ", " (the RFC 7230 way
// of combining repeated field values), so a pattern sees them all. Returns false
// when no header matches or name is empty.
func lookupHeader(headers []Header, name string) (string, bool) {
	if name == "" {
		return "", false
	}
	var vals []string
	for _, h := range headers {
		if strings.EqualFold(h.Name, name) {
			vals = append(vals, h.Value)
		}
	}
	if len(vals) == 0 {
		return "", false
	}
	return strings.Join(vals, ", "), true
}

// walkJSONPath parses content as JSON and walks a dot-separated path, returning
// the addressed value as a string. An object key indexes a map; a numeric
// segment indexes an array when the value at that level is one. A scalar leaf is
// stringified plainly (numbers without a trailing ".0", booleans as true/false);
// a non-scalar leaf (object/array) is re-encoded as JSON so a pattern can still
// scan it. Returns false when the content isn't JSON, a segment doesn't resolve,
// or the leaf is JSON null.
func walkJSONPath(content, path string) (string, bool) {
	var root any
	if err := json.Unmarshal([]byte(content), &root); err != nil {
		return "", false
	}
	cur := root
	for _, seg := range strings.Split(path, ".") {
		switch node := cur.(type) {
		case map[string]any:
			v, ok := node[seg]
			if !ok {
				return "", false
			}
			cur = v
		case []any:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(node) {
				return "", false
			}
			cur = node[idx]
		default:
			return "", false
		}
	}
	return stringifyJSON(cur)
}

// stringifyJSON renders a JSON leaf value as a property string. Scalars stringify
// plainly; a composite (object/array) is re-encoded so a pattern can scan it.
// JSON null resolves to absent.
func stringifyJSON(v any) (string, bool) {
	switch val := v.(type) {
	case nil:
		return "", false
	case string:
		return val, true
	case bool:
		return strconv.FormatBool(val), true
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64), true
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return "", false
		}
		return string(b), true
	}
}

// applyPattern applies an RE2 regex to s, returning capture group 1 when the
// pattern has one, else the whole match. A pattern that fails to compile (a
// record built from an unvalidated corpus) or finds no match returns false —
// matching stays silent the way value omission does; validation reports a
// malformed pattern separately (request-property-pattern-invalid).
func applyPattern(pattern, s string) (string, bool) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", false
	}
	m := re.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	if len(m) > 1 {
		return m[1], true
	}
	return m[0], true
}
