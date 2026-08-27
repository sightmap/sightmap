package webmcp

// Ordered YAML decoding. The compiler must preserve mapping order from the
// manifest (read specs, query/header/body maps) the way js-yaml's plain-object
// load does in the Node generator, so YAML decodes into *OM / []any / scalars
// via yaml.Node rather than into Go maps.

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

func decodeYAMLNode(n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil, nil
		}
		return decodeYAMLNode(n.Content[0])
	case yaml.MappingNode:
		om := NewOM()
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("line %d: non-scalar mapping key", k.Line)
			}
			v, err := decodeYAMLNode(n.Content[i+1])
			if err != nil {
				return nil, err
			}
			om.Set(k.Value, v)
		}
		return om, nil
	case yaml.SequenceNode:
		out := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := decodeYAMLNode(c)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case yaml.ScalarNode:
		return decodeYAMLScalar(n), nil
	case yaml.AliasNode:
		return decodeYAMLNode(n.Alias)
	}
	return nil, fmt.Errorf("line %d: unsupported YAML node kind %d", n.Line, n.Kind)
}

func decodeYAMLScalar(n *yaml.Node) any {
	tag := n.Tag
	if tag == "" || tag == "!" {
		tag = "!!str"
	}
	switch tag {
	case "!!null":
		return nil
	case "!!bool":
		return strings.EqualFold(n.Value, "true")
	case "!!int":
		if i, err := strconv.Atoi(n.Value); err == nil {
			return i
		}
		return n.Value
	case "!!float":
		if f, err := strconv.ParseFloat(n.Value, 64); err == nil {
			return f
		}
		return n.Value
	default:
		return n.Value
	}
}

// parseYAMLOrdered parses one YAML document into *OM / []any / scalars.
func parseYAMLOrdered(data []byte) (any, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if root.Kind == 0 {
		return nil, nil // empty document
	}
	return decodeYAMLNode(&root)
}

// Typed accessors over the decoded ordered values.

func omGet(v any, key string) any {
	om, ok := v.(*OM)
	if !ok {
		return nil
	}
	out, _ := om.Get(key)
	return out
}

func asOM(v any) *OM {
	om, _ := v.(*OM)
	return om
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	case int:
		return strconv.Itoa(t)
	case bool:
		return strconv.FormatBool(t)
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'g', -1, 64)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func asInt(v any) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case float64:
		if t == float64(int(t)) {
			return int(t), true
		}
	}
	return 0, false
}
