package sightmap

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Unknown-field detection.
//
// The typed loader silently ignores YAML keys it doesn't recognize, so a typo
// like `memroy:` or a half-baked field like `macros:` disappears without a
// trace. This walks the raw YAML and warns (not errors) on any key that isn't
// part of the spec at its position — loud enough to catch typos, lenient enough
// that authors can stash experimental fields during development.
//
// The known-key sets below mirror sightmap.schema.json (the source of truth),
// NOT the Go structs — the structs may still lag a schema addition, which
// would otherwise produce false positives.
// stability/access/snapshots/url/properties are all recognized here.

var (
	fileRootFields        = set("version", "url", "memory", "views", "components", "requests", "snapshots")
	viewFields            = set("name", "route", "url", "stability", "access", "description", "source", "dependencies", "memory", "components", "requests")
	componentFields       = set("name", "selector", "source", "dependencies", "description", "stability", "memory", "tags", "properties", "children")
	refFields             = set("$ref")
	requestFields         = set("name", "route", "method", "description", "source", "request", "response", "headers", "memory", "tags", "properties")
	payloadFields         = set("fields")
	fieldFields           = set("name", "type", "description")
	propertyFields        = set("name", "extract", "transform")
	requestPropertyFields = set("name", "field", "pattern", "transform")
	accessFields          = set("status", "reason")
	snapshotFields        = set("name", "notes", "url")
)

func set(keys ...string) map[string]bool {
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m
}

// unknownFieldWarnings walks the raw YAML in data and returns a warning for each
// key that is not part of the spec at its location. file labels the warnings.
func unknownFieldWarnings(data []byte, file string) []ValidationError {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil // parse errors are surfaced elsewhere
	}
	if len(doc.Content) == 0 {
		return nil
	}
	var out []ValidationError
	walkFile(doc.Content[0], file, &out)
	return out
}

// checkKeys warns on keys of a mapping node that are not in known, and returns
// the value node for each present key so callers can recurse.
func checkKeys(node *yaml.Node, known map[string]bool, file string, out *[]ValidationError) map[string]*yaml.Node {
	vals := map[string]*yaml.Node{}
	if node == nil || node.Kind != yaml.MappingNode {
		return vals
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		vals[key.Value] = node.Content[i+1]
		if !known[key.Value] {
			*out = append(*out, ValidationError{
				File:     file,
				Code:     "unknown-field",
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("unknown field %q (line %d)", key.Value, key.Line),
			})
		}
	}
	return vals
}

// forEachItem calls fn on each element of a sequence node (no-op otherwise).
func forEachItem(seq *yaml.Node, fn func(*yaml.Node)) {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return
	}
	for _, item := range seq.Content {
		fn(item)
	}
}

func walkFile(node *yaml.Node, file string, out *[]ValidationError) {
	v := checkKeys(node, fileRootFields, file, out)
	forEachItem(v["views"], func(n *yaml.Node) { walkView(n, file, out) })
	forEachItem(v["components"], func(n *yaml.Node) { walkComponentOrRef(n, file, out) })
	forEachItem(v["requests"], func(n *yaml.Node) { walkRequest(n, file, out) })
	forEachItem(v["snapshots"], func(n *yaml.Node) { checkKeys(n, snapshotFields, file, out) })
}

func walkView(node *yaml.Node, file string, out *[]ValidationError) {
	v := checkKeys(node, viewFields, file, out)
	if a := v["access"]; a != nil {
		checkKeys(a, accessFields, file, out)
	}
	forEachItem(v["components"], func(n *yaml.Node) { walkComponentOrRef(n, file, out) })
	forEachItem(v["requests"], func(n *yaml.Node) { walkRequest(n, file, out) })
}

// walkComponentOrRef dispatches on whether the entry is a $ref object or an
// inline component definition.
func walkComponentOrRef(node *yaml.Node, file string, out *[]ValidationError) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == "$ref" {
			checkKeys(node, refFields, file, out)
			return
		}
	}
	walkComponent(node, file, out)
}

func walkComponent(node *yaml.Node, file string, out *[]ValidationError) {
	v := checkKeys(node, componentFields, file, out)
	forEachItem(v["properties"], func(n *yaml.Node) { checkKeys(n, propertyFields, file, out) })
	forEachItem(v["children"], func(n *yaml.Node) { walkComponentOrRef(n, file, out) })
}

func walkRequest(node *yaml.Node, file string, out *[]ValidationError) {
	v := checkKeys(node, requestFields, file, out)
	if p := v["request"]; p != nil {
		walkPayload(p, file, out)
	}
	if p := v["response"]; p != nil {
		walkPayload(p, file, out)
	}
	forEachItem(v["properties"], func(n *yaml.Node) { checkKeys(n, requestPropertyFields, file, out) })
}

func walkPayload(node *yaml.Node, file string, out *[]ValidationError) {
	v := checkKeys(node, payloadFields, file, out)
	forEachItem(v["fields"], func(n *yaml.Node) { checkKeys(n, fieldFields, file, out) })
}
