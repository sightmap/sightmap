package sightmap

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Extract sources (SEP-0017). Each entity accepts a subset: see
// ComponentExtractSources, RequestPropertySources and MessagePropertySources.
const (
	FromDOMText         = "dom.text"
	FromDOMRawText      = "dom.raw_text"
	FromDOMAttr         = "dom.attr"
	FromDOMState        = "dom.state"
	FromComponent       = "component"
	FromComponentExists = "component.exists"
	FromReqBody         = "req.body"
	FromRspBody         = "rsp.body"
	FromReqHeaders      = "req.headers"
	FromRspHeaders      = "rsp.headers"
	FromStack           = "stack"
)

// ComponentExtractSources is the closed set of sources a component property may
// read.
var ComponentExtractSources = []string{
	FromDOMText, FromDOMRawText, FromDOMAttr, FromDOMState, FromComponent, FromComponentExists,
}

// StateNames are the interactive-state names `from: dom.state` accepts (SEP-0013).
var StateNames = []string{"checked", "selected", "disabled", "expanded"}

// Extract is a property's extraction directive (SEP-0017): a source, the value
// within it, and optional refinements. A string form in the corpus is lowered to
// this shape at load time, so consumers see one form whichever the author wrote.
type Extract struct {
	From    string `json:"from"`
	Path    string `json:"path,omitempty"`
	Pattern string `json:"pattern,omitempty"`
	// Join collapses every match of a component path into one value. Only valid
	// with From == FromComponent; empty means first match.
	Join string `json:"join,omitempty"`

	// Legacy is the deprecated string form this was lowered from, empty for the
	// object form. Validation warns on it (extract-legacy-form).
	Legacy string `json:"-"`

	joinSet bool // join was present in the corpus, even as ""
	mixed   bool // the corpus entry used the object and a string form together
}

// IsLegacy reports whether the directive was written in a deprecated string form.
func (e Extract) IsLegacy() bool { return e.Legacy != "" }

// String renders the directive in its object form, for messages and hashing.
func (e Extract) String() string {
	parts := []string{"from: " + e.From}
	if e.Path != "" {
		parts = append(parts, "path: "+e.Path)
	}
	if e.Pattern != "" {
		parts = append(parts, fmt.Sprintf("pattern: %q", e.Pattern))
	}
	if e.Join != "" {
		parts = append(parts, fmt.Sprintf("join: %q", e.Join))
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

// Refine applies Pattern to s: capture group 1 when the pattern has one, else
// the whole match. With no pattern s is returned unchanged. A value the pattern
// does not match, or a pattern that fails to compile, yields false; validation
// reports a malformed pattern separately.
func (e Extract) Refine(s string) (string, bool) {
	if e.Pattern == "" {
		return s, true
	}
	return applyPattern(e.Pattern, s)
}

var patternCache sync.Map // pattern -> *regexp.Regexp, or error

func compilePattern(pattern string) (*regexp.Regexp, error) {
	if v, ok := patternCache.Load(pattern); ok {
		if re, ok := v.(*regexp.Regexp); ok {
			return re, nil
		}
		return nil, v.(error)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		patternCache.Store(pattern, err)
		return nil, err
	}
	patternCache.Store(pattern, re)
	return re, nil
}

// LowerComponentExtract lowers a deprecated component string form to its object
// equivalent. An unrecognized string keeps an empty From, so validation reports
// it with the original text.
func LowerComponentExtract(s string) Extract {
	e := Extract{Legacy: s}
	switch {
	case s == "text":
		e.From = FromDOMText
	case s == "raw_text":
		e.From = FromDOMRawText
	case strings.HasPrefix(s, "attr="):
		e.From, e.Path = FromDOMAttr, strings.TrimPrefix(s, "attr=")
	case strings.HasPrefix(s, "exists:"):
		e.From, e.Path = FromComponentExists, strings.TrimPrefix(s, "exists:")
	default:
		if dot := strings.LastIndex(s, "."); dot > 0 && dot < len(s)-1 {
			e.From, e.Path = FromComponent, s
		}
	}
	return e
}

// lowerSourceField lowers a deprecated request or message `source`/`field`/
// `pattern` triple to its object equivalent.
func lowerSourceField(source, field, pattern string) Extract {
	legacy := "source: " + source
	if field != "" {
		legacy += ", field: " + field
	}
	return Extract{From: source, Path: field, Pattern: pattern, Legacy: legacy}
}

// rawExtract decodes `extract:` as either the object form or a deprecated
// string form.
type rawExtract struct {
	set    bool
	legacy string
	obj    struct {
		From    string  `yaml:"from"`
		Path    string  `yaml:"path"`
		Pattern string  `yaml:"pattern"`
		Join    *string `yaml:"join"`
	}
}

func (r *rawExtract) UnmarshalYAML(n *yaml.Node) error {
	r.set = true
	if n.Kind == yaml.ScalarNode {
		r.legacy = n.Value
		if r.legacy == "" {
			r.legacy = " " // an explicit empty string is still a (broken) string form
		}
		return nil
	}
	return n.Decode(&r.obj)
}

// component lowers a component property's extract value.
func (r rawExtract) component() Extract {
	if r.legacy != "" {
		return LowerComponentExtract(strings.TrimSpace(r.legacy))
	}
	return r.object()
}

func (r rawExtract) object() Extract {
	e := Extract{From: r.obj.From, Path: r.obj.Path, Pattern: r.obj.Pattern}
	if r.obj.Join != nil {
		e.Join, e.joinSet = *r.obj.Join, true
	}
	return e
}

// sourced lowers a request or message property, which may carry the object
// form or the deprecated source/field/pattern keys, but not both.
func (r rawExtract) sourced(source, field, pattern string) Extract {
	hasLegacy := source != "" || field != "" || pattern != ""
	if !r.set {
		if hasLegacy {
			return lowerSourceField(source, field, pattern)
		}
		return Extract{}
	}
	e := r.object()
	if r.legacy != "" {
		e = Extract{Legacy: strings.TrimSpace(r.legacy)}
	}
	e.mixed = hasLegacy
	return e
}

// PathSegment is one component name in a component path. Multi marks a segment
// written Name[], which resolves to every match instead of the first.
type PathSegment struct {
	Name  string
	Multi bool
}

// UnmarshalJSON reads the object form, and also a bare string: the component
// extract shape written before SEP-0017, so a corpus exported by an older SDK
// still decodes. The string is lowered as the loader lowers YAML.
func (e *Extract) UnmarshalJSON(data []byte) error {
	var legacy string
	if err := json.Unmarshal(data, &legacy); err == nil {
		*e = LowerComponentExtract(legacy)
		return nil
	}
	type wire Extract
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*e = Extract(w)
	e.joinSet = e.Join != ""
	return nil
}

// sourcedJSON is a request or message property as either SDK wrote it: the
// object form, or the source/field/pattern keys written before SEP-0017.
type sourcedJSON struct {
	Name    string   `json:"name"`
	Extract *Extract `json:"extract"`
	Source  string   `json:"source"`
	Field   string   `json:"field"`
	Pattern string   `json:"pattern"`
}

func (w sourcedJSON) lower() Extract {
	if w.Extract != nil {
		return *w.Extract
	}
	if w.Source == "" && w.Field == "" && w.Pattern == "" {
		return Extract{}
	}
	return lowerSourceField(w.Source, w.Field, w.Pattern)
}

// UnmarshalJSON also accepts the source/field/pattern shape written before
// SEP-0017.
func (d *RequestPropertyDef) UnmarshalJSON(data []byte) error {
	var w sourcedJSON
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*d = RequestPropertyDef{Name: w.Name, Extract: w.lower()}
	return nil
}

// UnmarshalJSON also accepts the source/field/pattern shape written before
// SEP-0017.
func (d *MessagePropertyDef) UnmarshalJSON(data []byte) error {
	var w sourcedJSON
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*d = MessagePropertyDef{Name: w.Name, Extract: w.lower()}
	return nil
}

// ParseComponentPath splits a component path into its segments. It does not
// validate; see validation for the accepted forms.
func ParseComponentPath(path string) []PathSegment {
	parts := strings.Split(path, ".")
	out := make([]PathSegment, len(parts))
	for i, p := range parts {
		name, multi := strings.CutSuffix(p, "[]")
		out[i] = PathSegment{Name: name, Multi: multi}
	}
	return out
}

func containsString(set []string, s string) bool {
	for _, v := range set {
		if v == s {
			return true
		}
	}
	return false
}
