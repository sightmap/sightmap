package match

import (
	"strings"

	"github.com/sightmap/sightmap/go/sightmap"
)

// resolveComponentProperties fills each match's Properties by resolving its
// component definition's extract directives over the matched component tree,
// per SEP-0010. Resolution is tree-closed and offline: it never touches a live
// DOM. It runs after all matches are collected, so PATH.prop / exists:PATH can
// see sibling and descendant matches.
//
// A property is dropped silently when it does not resolve (empty text, an
// attribute the node does not carry, or a PATH that matches no descendant
// component), matching the spec's silent-omission rule.
func resolveComponentProperties(
	result map[*sightmap.ComponentNode]*sightmap.ComponentMatch,
	defByNode map[*sightmap.ComponentNode]*sightmap.ComponentDef,
) {
	for node, cm := range result {
		def := defByNode[node]
		if def == nil || len(def.Properties) == 0 {
			continue
		}
		var props []sightmap.PropertyValue
		for _, p := range def.Properties {
			if v, ok := resolveExtract(node, p.Extract, result, defByNode); ok {
				props = append(props, sightmap.PropertyValue{Name: p.Name, Value: v})
			}
		}
		cm.Properties = props
	}
}

// resolveExtract resolves one extract directive against node. References descend
// only, so recursion strictly enters smaller subtrees and always terminates.
func resolveExtract(
	node *sightmap.ComponentNode,
	extract string,
	result map[*sightmap.ComponentNode]*sightmap.ComponentMatch,
	defByNode map[*sightmap.ComponentNode]*sightmap.ComponentDef,
) (string, bool) {
	switch {
	case extract == "text":
		return node.Name, node.Name != ""

	case strings.HasPrefix(extract, "attr="):
		name := extract[len("attr="):]
		if name == "" || node.Element == nil {
			return "", false
		}
		v, ok := node.Element.Attrs[name]
		return v, ok && v != ""

	case strings.HasPrefix(extract, "exists:"):
		if resolvePath(node, extract[len("exists:"):], result) != nil {
			return "true", true
		}
		return "", false

	default: // PATH.prop
		dot := strings.LastIndex(extract, ".")
		if dot <= 0 || dot == len(extract)-1 {
			return "", false
		}
		target := resolvePath(node, extract[:dot], result)
		if target == nil {
			return "", false
		}
		tdef := defByNode[target]
		if tdef == nil {
			return "", false
		}
		prop := extract[dot+1:]
		for _, tp := range tdef.Properties {
			if tp.Name == prop {
				return resolveExtract(target, tp.Extract, result, defByNode)
			}
		}
		return "", false
	}
}

// resolvePath walks a dotted component-name path into node's subtree, returning
// the deepest matched node (first in document order at each segment) or nil.
func resolvePath(
	node *sightmap.ComponentNode,
	path string,
	result map[*sightmap.ComponentNode]*sightmap.ComponentMatch,
) *sightmap.ComponentNode {
	cur := node
	for _, seg := range strings.Split(path, ".") {
		if seg == "" {
			return nil
		}
		next := firstDescendantNamed(cur, seg, result)
		if next == nil {
			return nil
		}
		cur = next
	}
	if cur == node {
		return nil // empty path resolves to self, which is not a descendant
	}
	return cur
}

// firstDescendantNamed returns the first node in root's subtree (pre-order,
// excluding root) whose winning component match name equals name.
func firstDescendantNamed(
	root *sightmap.ComponentNode,
	name string,
	result map[*sightmap.ComponentNode]*sightmap.ComponentMatch,
) *sightmap.ComponentNode {
	for _, child := range root.Children {
		if found := searchNamed(child, name, result); found != nil {
			return found
		}
	}
	return nil
}

func searchNamed(
	node *sightmap.ComponentNode,
	name string,
	result map[*sightmap.ComponentNode]*sightmap.ComponentMatch,
) *sightmap.ComponentNode {
	if cm := result[node]; cm != nil && cm.Name == name {
		return node
	}
	for _, child := range node.Children {
		if found := searchNamed(child, name, result); found != nil {
			return found
		}
	}
	return nil
}
