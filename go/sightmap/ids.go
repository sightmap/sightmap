package sightmap

import (
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

// Entity ids (SEP-0020).
//
// Every named entity may declare an `id`: its identity across versions of the
// sightmap. Ids are checked on the raw YAML rather than the loaded corpus
// because the loader deep-copies a definition at each $ref site, so one
// declaration appears many times once expanded, and because yaml.v3 decodes an
// unquoted number into a Go string, which would hide a non-string id.

// idPattern is the shape of an id: 8 to 64 letters, digits, '_' or '-',
// starting with a letter or digit.
var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{7,63}$`)

// idIndex collects every entity across all files of a corpus.
type idIndex struct {
	entities []idEntity
}

type idEntity struct {
	kind, name, file string
	line             int
	id               *yaml.Node // nil when the entity declares no id
}

func newIDIndex() *idIndex { return &idIndex{} }

// addFile records the entities declared in one corpus file.
func (x *idIndex) addFile(data []byte, file string) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 {
		return // parse errors are surfaced elsewhere
	}
	root := mapping(doc.Content[0])
	forEachItem(root["environments"], func(n *yaml.Node) { x.add("environment", n, file) })
	forEachItem(root["views"], func(n *yaml.Node) { x.addView(n, file) })
	forEachItem(root["components"], func(n *yaml.Node) { x.addComponent(n, file) })
	forEachItem(root["definitions"], func(n *yaml.Node) { x.addComponent(n, file) })
	forEachItem(root["requests"], func(n *yaml.Node) { x.addRequest(n, file) })
	forEachItem(root["messages"], func(n *yaml.Node) {
		v := x.add("message", n, file)
		forEachItem(v["properties"], func(p *yaml.Node) { x.add("property", p, file) })
	})
	forEachItem(root["signals"], func(n *yaml.Node) { x.add("signal", n, file) })
	forEachItem(root["snapshots"], func(n *yaml.Node) { x.add("snapshot", n, file) })
}

func (x *idIndex) addView(n *yaml.Node, file string) {
	v := x.add("view", n, file)
	forEachItem(v["properties"], func(p *yaml.Node) { x.add("property", p, file) })
	forEachItem(v["components"], func(c *yaml.Node) { x.addComponent(c, file) })
	forEachItem(v["requests"], func(r *yaml.Node) { x.addRequest(r, file) })
}

// addComponent records a component or a $ref entry. A $ref entry is an entity
// of its own: its id names the placement, while the definition's id names the
// declaration.
func (x *idIndex) addComponent(n *yaml.Node, file string) {
	if _, ok := mapping(n)["$ref"]; ok {
		x.add("$ref", n, file)
		return
	}
	v := x.add("component", n, file)
	forEachItem(v["properties"], func(p *yaml.Node) { x.add("property", p, file) })
	forEachItem(v["children"], func(c *yaml.Node) { x.addComponent(c, file) })
}

func (x *idIndex) addRequest(n *yaml.Node, file string) {
	v := x.add("request", n, file)
	forEachItem(v["properties"], func(p *yaml.Node) { x.add("property", p, file) })
	for _, body := range []string{"request", "response"} {
		forEachItem(mapping(v[body])["fields"], func(f *yaml.Node) { x.add("field", f, file) })
	}
}

// add records one entity and returns its keys so callers can descend.
func (x *idIndex) add(kind string, n *yaml.Node, file string) map[string]*yaml.Node {
	v := mapping(n)
	if n == nil || n.Kind != yaml.MappingNode {
		return v
	}
	e := idEntity{kind: kind, file: file, line: n.Line, id: v["id"]}
	if name := v["name"]; name != nil {
		e.name = name.Value
	} else if ref := v["$ref"]; ref != nil {
		e.name = ref.Value
	}
	x.entities = append(x.entities, e)
	return v
}

// diagnostics reports ids of the wrong shape, ids shared by two entities, and,
// once any entity in the corpus declares an id, every entity without one.
func (x *idIndex) diagnostics() []ValidationError {
	var out []ValidationError
	bad := func(e idEntity, code, msg string) {
		out = append(out, ValidationError{File: e.file, Component: e.name, Code: code, Severity: SeverityError, Message: msg})
	}
	first := map[string]idEntity{}
	adopted := false
	for _, e := range x.entities {
		if e.id == nil {
			continue
		}
		adopted = true
		id := e.id
		switch {
		case id.Kind != yaml.ScalarNode || (id.Tag != "" && id.Tag != "!!str"):
			bad(e, "id-invalid", fmt.Sprintf("%s id must be a string (line %d); quote the value: id: %q", e.kind, id.Line, id.Value))
		case !idPattern.MatchString(id.Value):
			bad(e, "id-invalid", fmt.Sprintf("%s id %q must be 8 to 64 letters, digits, '_' or '-', starting with a letter or digit (line %d)", e.kind, id.Value, id.Line))
		default:
			if prev, dup := first[id.Value]; dup {
				bad(e, "id-duplicate", fmt.Sprintf("%s id %q (line %d) is already declared by %s %q in %s (line %d); an id names exactly one entity", e.kind, id.Value, id.Line, prev.kind, prev.name, prev.file, prev.id.Line))
			} else {
				first[id.Value] = e
			}
		}
	}
	if !adopted {
		return out
	}
	for _, e := range x.entities {
		if e.id == nil {
			bad(e, "id-missing", fmt.Sprintf("%s (line %d) has no id; once a sightmap declares any id, every entity needs one", e.kind, e.line))
		}
	}
	return out
}

// mapping returns the value node for each key of a mapping node (empty
// otherwise).
func mapping(n *yaml.Node) map[string]*yaml.Node {
	vals := map[string]*yaml.Node{}
	if n == nil || n.Kind != yaml.MappingNode {
		return vals
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		vals[n.Content[i].Value] = n.Content[i+1]
	}
	return vals
}
