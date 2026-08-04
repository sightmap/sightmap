package sightmap

import (
	"net/url"
	"strings"
)

// RequestDef is a named API endpoint from the sightmap corpus. It classifies an
// observed network request by matching that request's route (a glob pattern)
// and optional method. Requests may be declared globally (a file-root
// `requests:` list) or scoped to a view; either way they are matched by their
// own route against the observed request URL, not by the page's view.
//
// Per the spec, request matching is "all matches apply": every RequestDef whose
// route (and optional method) matches contributes to the enriched output —
// there is no single winner, unlike view matching (most-specific-wins).
//
// It is named on the rule side of the corpus (a RequestDef classifies an
// observed request) so the Request/Response payload fields read naturally; the
// sibling View / component definition types are regularized onto the same
// convention separately.
type RequestDef struct {
	Name        string            `json:"name"`
	Route       string            `json:"route,omitempty"`  // glob pattern; express-style ":param" segments match one segment
	Method      string            `json:"method,omitempty"` // optional HTTP method filter; "" matches any method
	Description string            `json:"description,omitempty"`
	Source      string            `json:"source,omitempty"`
	Request     *Payload          `json:"request,omitempty"`  // expected request payload shape
	Response    *Payload          `json:"response,omitempty"` // expected response payload shape
	Headers     []string          `json:"headers,omitempty"`  // notable header names to highlight
	Memory      []string          `json:"memory,omitempty"`   // request-level memory entries
	Tags        []string          `json:"tags,omitempty"`     // open-vocabulary labels (SEP-0004)
	Properties  []RequestProperty `json:"properties,omitempty"`
}

// RequestProperty declares a named value to extract from a live request/response
// pair (SEP-0005). Exactly one of Field or Pattern is set.
//
// Extraction is a live-traffic concern: these declarations name where a value
// lives, and a consumer observing real traffic resolves them. A tool working
// from static corpus definitions alone treats every property as
// declared-but-unavailable rather than an error.
type RequestProperty struct {
	Name string `json:"name"`
	// Field is a rooted path: "req" or "rsp", then either ".body.<path>"
	// (object-key traversal into the parsed JSON body) or ".headers.<name>"
	// (one header's value, name matched case-insensitively). The reserved
	// identity names in ReservedRequestPropertyNames are also accepted.
	Field string `json:"field,omitempty"`
	// Pattern is a regex matched against the raw text of the response body,
	// for content Field's object-key traversal cannot reach. It carries no
	// root of its own.
	Pattern string `json:"pattern,omitempty"`
	// Transform is optional post-processing, sharing componentProperty's
	// vocabulary (SEP-0003).
	Transform string `json:"transform,omitempty"`
}

// ReservedRequestPropertyNames are the already-structured identity fields of a
// request. A consumer may reference these wherever a property name is expected
// without any properties: declaration, so declaring a property under one of
// these names shadows the HTTP identity and makes it unreachable. Validation
// warns on that shadowing; see checkRequestProperties.
var ReservedRequestPropertyNames = []string{"status", "method", "duration"}

// ReservedComponentPropertyNames are component property names that resolve
// without a properties: declaration. `value` is always available from the
// accessibility tree, so a signal filtering on it is valid even when the
// component declares no properties of its own.
var ReservedComponentPropertyNames = []string{"value"}

// Payload is the expected shape of a request or response body. The field list
// is advisory and not exhaustive — extra fields on the wire are not rejected.
type Payload struct {
	Fields []Field `json:"fields,omitempty"`
}

// Field is one expected field in a Payload. Type and Description are free-text.
type Field struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
}

// RequestsForURL returns every RequestDef whose route matches requestURL and
// whose method matches. An empty RequestDef.Method matches any method; an empty
// method argument matches any RequestDef. requestURL is the observed network
// request's URL (the API endpoint being called), NOT the page URL; its query
// string and fragment are ignored and trailing slashes are normalized away.
//
// Global requests are returned first (corpus-file order), then view-scoped
// requests in view-declaration order. All matches apply — there is no winner.
func (c *Corpus) RequestsForURL(requestURL, method string) []RequestDef {
	u, err := url.Parse(requestURL)
	if err != nil {
		return nil
	}
	path := normalizeRoutePath(u.Path)

	var out []RequestDef
	appendMatches := func(defs []RequestDef) {
		for _, rd := range defs {
			if !MatchRoute(rd.Route, path) {
				continue
			}
			if rd.Method != "" && method != "" && !strings.EqualFold(rd.Method, method) {
				continue
			}
			out = append(out, rd)
		}
	}

	appendMatches(c.Requests)
	for i := range c.Views {
		appendMatches(c.Views[i].Requests)
	}
	return out
}
