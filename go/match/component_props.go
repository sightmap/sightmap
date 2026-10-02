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
// A property is dropped silently when it does not resolve: empty text, an
// attribute the node does not carry, a PATH that matches no descendant
// component, or a value read from a node whose effective privacy withholds it.
func resolveComponentProperties(
	result map[*sightmap.ComponentNode]*sightmap.ComponentMatch,
	defByNode map[*sightmap.ComponentNode]*sightmap.ComponentDef,
	privacy map[*sightmap.ComponentNode]string,
) {
	for node, cm := range result {
		def := defByNode[node]
		if def == nil || len(def.Properties) == 0 {
			continue
		}
		var props []sightmap.PropertyValue
		for _, p := range def.Properties {
			if v, ok := resolveExtract(node, p.Extract, result, defByNode, privacy); ok {
				props = append(props, sightmap.PropertyValue{Name: p.Name, Value: v})
			}
		}
		cm.Properties = props
	}
}

// effectivePrivacy resolves SEP-0009 privacy for every node under root: a
// matched component's declaration applies to its subtree, and the nearest
// enclosing declaration wins. Nodes with no enclosing declaration are absent.
func effectivePrivacy(
	root *sightmap.ComponentNode,
	defByNode map[*sightmap.ComponentNode]*sightmap.ComponentDef,
) map[*sightmap.ComponentNode]string {
	out := map[*sightmap.ComponentNode]string{}
	var walk func(n *sightmap.ComponentNode, inherited string)
	walk = func(n *sightmap.ComponentNode, inherited string) {
		if def := defByNode[n]; def != nil && def.Privacy != "" {
			inherited = def.Privacy
		}
		if inherited != "" {
			out[n] = inherited
		}
		for _, c := range n.Children {
			walk(c, inherited)
		}
	}
	walk(root, "")
	return out
}

// withholds reports whether a value read locally from a node with effective
// privacy p must not be surfaced (SEP-0009): content is withheld under block and
// mask, except that mask permits the interactive-state attributes. PATH.prop and
// exists: read another node and are judged there instead.
func withholds(p, extract string) bool {
	if p != "block" && p != "mask" {
		return false
	}
	switch {
	case extract == "text", extract == "raw_text":
		return true
	case strings.HasPrefix(extract, "attr="):
		return p == "block" || !sightmap.IsStateAttr(extract[len("attr="):])
	}
	return false
}

// resolveExtract resolves one extract directive against node. References descend
// only, so recursion strictly enters smaller subtrees and always terminates.
//
// Privacy is judged at the node a value is read from: a PATH.prop recurses into
// the target, whose own effective privacy then applies, so an unrestricted
// ancestor cannot surface a value out of a blocked descendant.
func resolveExtract(
	node *sightmap.ComponentNode,
	extract string,
	result map[*sightmap.ComponentNode]*sightmap.ComponentMatch,
	defByNode map[*sightmap.ComponentNode]*sightmap.ComponentDef,
	privacy map[*sightmap.ComponentNode]string,
) (string, bool) {
	if withholds(privacy[node], extract) {
		return "", false
	}
	switch {
	case extract == "text":
		// Prefer the accessible name; fall back to the node's rendered text
		// (populated for role-less nodes that have no accessible name). Both are
		// already whitespace-normalized to the same shape at capture time.
		if node.Name != "" {
			return node.Name, true
		}
		return node.Text, node.Text != ""

	case extract == "raw_text":
		// The node's own literal text (direct text nodes, normalized) — never the
		// accessibility name, and pinned to textContent-style data rather than the
		// layout-dependent innerText (SEP-0013). The deterministic escape when the
		// accessible name welds in extra text (e.g. a heading whose AX name appends
		// a CSS ::after badge: `text` -> "Main Most popular" but `raw_text` -> "Main").
		return node.RawText, node.RawText != ""

	case strings.HasPrefix(extract, "attr="):
		name := extract[len("attr="):]
		if name == "" {
			return "", false
		}
		if sightmap.IsStateAttr(name) {
			if v := node.State[name]; v != "" {
				return v, true
			}
		}
		if node.Element == nil {
			return "", false
		}
		v, ok := node.Element.Attrs[name]
		return v, ok && v != ""

	case strings.HasPrefix(extract, "exists:"):
		target := resolvePath(node, extract[len("exists:"):], result)
		if target == nil || privacy[target] == "block" {
			return "", false
		}
		return "true", true

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
				return resolveExtract(target, tp.Extract, result, defByNode, privacy)
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
