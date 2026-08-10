package sightmap

import (
	"fmt"
	"sort"
	"strings"

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
// NOT the Go structs — the structs are deliberately incomplete (e.g. rawFile
// doesn't read requests), which would produce false positives.
// stability/access/snapshots/url/properties are all recognized here.

var (
	fileRootFields  = set("version", "url", "memory", "views", "components", "requests", "snapshots")
	viewFields      = set("name", "route", "url", "stability", "access", "description", "source", "dependencies", "memory", "components", "requests")
	componentFields = set("name", "selector", "source", "dependencies", "description", "stability", "memory", "tags", "properties", "children")
	refFields       = set("$ref")
	requestFields   = set("name", "route", "method", "description", "source", "request", "response", "headers", "memory", "tags", "properties")
	payloadFields   = set("fields")
	fieldFields     = set("name", "type", "description")
	propertyFields  = set("name", "extract", "transform")
	accessFields    = set("status", "reason")
	snapshotFields  = set("name", "notes", "url")

	requestPropertyFields = set("name", "source", "field", "pattern", "transform")
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

// checkStringScalars reports a scalar value whose resolved YAML tag is not
// !!str for a field the schema types as a string.
//
// This exists because yaml.v3 decodes any scalar into a Go string field by
// taking the raw lexeme, without consulting the resolved tag. So `level: 500`
// and `message: 404` load cleanly as "500" and "404", while ajv rejects both —
// the Go SDK would accept corpora the reference JSON Schema refuses. A bare
// `key:` is worse than a type mismatch: it resolves to !!null and lands as the
// empty string, so a filter or pattern silently matches nothing.
//
// Only scalars need checking. A mapping or sequence where the schema wants a
// string already fails in yaml.Unmarshal before this walker runs.
func checkStringScalars(vals map[string]*yaml.Node, fields []string, file string, out *[]ValidationError) {
	for _, name := range fields {
		n := vals[name]
		if n == nil || n.Kind != yaml.ScalarNode {
			continue
		}
		if n.Tag == "" || n.Tag == "!!str" {
			continue
		}
		*out = append(*out, ValidationError{
			File:     file,
			Code:     "field-type-invalid",
			Severity: SeverityError,
			Message: fmt.Sprintf("field %q must be a string (line %d); quote the value: %s: %q",
				name, n.Line, name, n.Value),
		})
	}
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
	known := fileRootFields
	if node != nil && node.Kind == yaml.MappingNode {
		known = fileRootDiagnostics(node, file, out)
	}
	v := checkKeys(node, known, file, out)
	forEachItem(v["views"], func(n *yaml.Node) { walkView(n, file, out) })
	forEachItem(v["components"], func(n *yaml.Node) { walkComponentOrRef(n, file, out) })
	forEachItem(v["requests"], func(n *yaml.Node) { walkRequest(n, file, out) })
	forEachItem(v["snapshots"], func(n *yaml.Node) { checkKeys(n, snapshotFields, file, out) })
}

// fileRootDiagnostics emits targeted diagnostics for the common file-root
// mistakes an author makes before they've discovered the schema — a misplaced
// view field (the views: wrapper was forgotten), a missing version:, and a
// view-shaped file (url: + components:, no views:) that silently becomes a
// globals file. It returns the set of keys checkKeys should treat as known, so a
// misplaced view field gets the teaching message here instead of a bare
// "unknown field".
func fileRootDiagnostics(node *yaml.Node, file string, out *[]ValidationError) map[string]bool {
	seen := map[string]bool{}
	var misplaced []string
	versionVal := ""
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i].Value
		seen[k] = true
		if k == "version" {
			versionVal = node.Content[i+1].Value
		}
		if !fileRootFields[k] && viewFields[k] {
			misplaced = append(misplaced, k)
		}
	}

	if len(misplaced) > 0 {
		sort.Strings(misplaced)
		quoted := make([]string, len(misplaced))
		for i, k := range misplaced {
			quoted[i] = fmt.Sprintf("%q", k)
		}
		*out = append(*out, ValidationError{
			File:     file,
			Code:     "view-field-at-file-root",
			Severity: SeverityWarning,
			Message: fmt.Sprintf("field(s) %s at the file root look like view fields — view fields belong under a top-level \"views:\" list, e.g.:\n  views:\n    - name: Home\n      route: /",
				strings.Join(quoted, ", ")),
		})
	}
	if !seen["version"] {
		*out = append(*out, ValidationError{
			File:     file,
			Code:     "missing-version",
			Severity: SeverityWarning,
			Message:  `missing "version:" — every corpus file should begin with "version: 1"`,
		})
	} else if versionVal != "1" {
		*out = append(*out, ValidationError{
			File:     file,
			Code:     "unsupported-version",
			Severity: SeverityError,
			Message:  fmt.Sprintf("unsupported version %q — this tooling supports version: 1", versionVal),
		})
	}
	if seen["url"] && seen["components"] && !seen["views"] {
		*out = append(*out, ValidationError{
			File:     file,
			Code:     "no-views-in-file",
			Severity: SeverityWarning,
			Message:  `this file sets "url:" and "components:" but defines no "views:" — its components are treated as global. Did you mean to nest them under a "views:" list?`,
		})
	}

	if len(misplaced) == 0 {
		return fileRootFields
	}
	known := make(map[string]bool, len(fileRootFields)+len(misplaced))
	for k := range fileRootFields {
		known[k] = true
	}
	for _, k := range misplaced {
		known[k] = true
	}
	return known
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
	forEachItem(v["properties"], func(n *yaml.Node) {
		pv := checkKeys(n, requestPropertyFields, file, out)
		checkStringScalars(pv, []string{"name", "source", "field", "pattern", "transform"}, file, out)
	})
}

func walkPayload(node *yaml.Node, file string, out *[]ValidationError) {
	v := checkKeys(node, payloadFields, file, out)
	forEachItem(v["fields"], func(n *yaml.Node) { checkKeys(n, fieldFields, file, out) })
}
