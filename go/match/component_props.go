package match

import (
	"slices"
	"strings"

	"github.com/sightmap/sightmap/go/sightmap"
)

// resolveComponentProperties fills each match's Properties by resolving its
// component definition's extract directives over the matched component tree,
// per SEP-0010. Resolution is tree-closed and offline: it never touches a live
// DOM. It runs after all matches are collected, so component and component.exists reads can
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

// withholds reports whether a read from source on a node with effective privacy
// p must not be surfaced (SEP-0009, per SEP-0017). Content is withheld under
// block and mask; mask still permits state. component and component.exists read
// another node and are judged there instead.
func withholds(p string, e sightmap.Extract) bool {
	switch e.From {
	case sightmap.FromDOMText, sightmap.FromDOMRawText:
		return p == "block" || p == "mask"
	case sightmap.FromDOMAttr:
		// SEP-0009 retains the interactive-state attributes under mask.
		return p == "block" || (p == "mask" && !slices.Contains(sightmap.StateNames, e.Path))
	case sightmap.FromDOMState:
		return p == "block"
	}
	return false
}

// resolveExtract resolves one SEP-0017 directive against node. References descend
// only, so recursion strictly enters smaller subtrees and always terminates.
//
// Privacy is judged at the node a value is read from: a component read recurses
// into the target, whose own effective privacy then applies, so an unrestricted
// ancestor cannot surface a value out of a blocked descendant.
func resolveExtract(
	node *sightmap.ComponentNode,
	e sightmap.Extract,
	result map[*sightmap.ComponentNode]*sightmap.ComponentMatch,
	defByNode map[*sightmap.ComponentNode]*sightmap.ComponentDef,
	privacy map[*sightmap.ComponentNode]string,
) (string, bool) {
	if withholds(privacy[node], e) {
		return "", false
	}
	switch e.From {
	case sightmap.FromDOMText:
		// Prefer the accessible name; fall back to the node's rendered text
		// (populated for role-less nodes that have no accessible name). Both are
		// already whitespace-normalized to the same shape at capture time.
		if node.Name != "" {
			return refine(e, node.Name)
		}
		return refine(e, node.Text)

	case sightmap.FromDOMRawText:
		// The node's own literal text (direct text nodes, normalized), never the
		// accessibility name (SEP-0013): the deterministic escape when the
		// accessible name welds in extra text, e.g. a CSS ::after badge.
		return refine(e, node.RawText)

	case sightmap.FromDOMAttr:
		if node.Element == nil {
			return "", false
		}
		return refine(e, node.Element.Attrs[e.Path])

	case sightmap.FromDOMState:
		// Current interactive state, never the markup attribute of the same
		// name, which records only initial state. A tree captured before State
		// existed still carries the accessibility layer's value in Properties.
		if v := node.State[e.Path]; v != "" {
			return refine(e, v)
		}
		return refine(e, node.Properties[e.Path])

	case sightmap.FromComponentExists:
		target := resolvePath(node, e.Path, result)
		if target == nil || privacy[target] == "block" {
			return "", false
		}
		return "true", true

	case sightmap.FromComponent:
		dot := strings.LastIndex(e.Path, ".")
		if dot <= 0 || dot == len(e.Path)-1 {
			return "", false
		}
		path, prop := e.Path[:dot], e.Path[dot+1:]
		if !strings.Contains(path, "[]") {
			target := resolvePath(node, path, result)
			if target == nil {
				return "", false
			}
			v, ok := readProperty(target, prop, result, defByNode, privacy)
			if !ok {
				return "", false
			}
			return refine(e, v)
		}
		if e.Join == "" {
			return "", false // an array-valued result is reserved; validation rejects it
		}
		var vals []string
		for _, target := range resolvePathAll(node, path, result) {
			v, ok := readProperty(target, prop, result, defByNode, privacy)
			if !ok {
				continue
			}
			if v, ok = refine(e, v); ok {
				vals = append(vals, v)
			}
		}
		if len(vals) == 0 {
			return "", false
		}
		return strings.Join(vals, e.Join), true
	}
	return "", false
}

// refine applies e's pattern to a resolved value; an empty value is omitted.
func refine(e sightmap.Extract, v string) (string, bool) {
	if v == "" {
		return "", false
	}
	v, ok := e.Refine(v)
	return v, ok && v != ""
}

// readProperty resolves the property named prop on target's own component
// definition.
func readProperty(
	target *sightmap.ComponentNode,
	prop string,
	result map[*sightmap.ComponentNode]*sightmap.ComponentMatch,
	defByNode map[*sightmap.ComponentNode]*sightmap.ComponentDef,
	privacy map[*sightmap.ComponentNode]string,
) (string, bool) {
	tdef := defByNode[target]
	if tdef == nil {
		return "", false
	}
	for _, tp := range tdef.Properties {
		if tp.Name == prop {
			return resolveExtract(target, tp.Extract, result, defByNode, privacy)
		}
	}
	return "", false
}

// resolvePathAll resolves a multi-valued component path, backing `join`. A segment
// written Name[] collects every match within each node the previous segment
// produced; any other segment takes the first match within each. Results are in
// document order without duplicates.
func resolvePathAll(
	node *sightmap.ComponentNode,
	path string,
	result map[*sightmap.ComponentNode]*sightmap.ComponentMatch,
) []*sightmap.ComponentNode {
	cur := []*sightmap.ComponentNode{node}
	for _, seg := range sightmap.ParseComponentPath(path) {
		if seg.Name == "" {
			return nil
		}
		seen := map[*sightmap.ComponentNode]bool{}
		var next []*sightmap.ComponentNode
		add := func(m *sightmap.ComponentNode) {
			if !seen[m] {
				seen[m] = true
				next = append(next, m)
			}
		}
		for _, n := range cur {
			if !seg.Multi {
				if m := firstDescendantNamed(n, seg.Name, result); m != nil {
					add(m)
				}
				continue
			}
			for _, child := range n.Children {
				collectNamed(child, seg.Name, result, add)
			}
		}
		if len(next) == 0 {
			return nil
		}
		cur = next
	}
	return cur
}

func collectNamed(
	node *sightmap.ComponentNode,
	name string,
	result map[*sightmap.ComponentNode]*sightmap.ComponentMatch,
	emit func(*sightmap.ComponentNode),
) {
	if cm := result[node]; cm != nil && cm.Name == name {
		emit(node)
	}
	for _, child := range node.Children {
		collectNamed(child, name, result, emit)
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
