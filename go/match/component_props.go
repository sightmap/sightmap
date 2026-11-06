package match

import (
	"slices"
	"strings"

	"github.com/sightmap/sightmap/go/sightmap"
)

// propResolver resolves component properties over one matched tree.
type propResolver struct {
	root      *sightmap.ComponentNode
	result    map[*sightmap.ComponentNode]*sightmap.ComponentMatch
	defByNode map[*sightmap.ComponentNode]*sightmap.ComponentDef
	// privacy is the effective privacy of any node in the tree, named or not.
	privacy func(*sightmap.ComponentNode) string
	index   *treeIndex // built on first use
}

func (r *propResolver) treeIndex() *treeIndex {
	if r.index == nil {
		r.index = buildTreeIndex(r.root)
	}
	return r.index
}

// resolveComponentProperties fills each match's Properties by resolving its
// component definition's extract directives over the matched component tree,
// per SEP-0010. Resolution is tree-closed and offline: it never touches a live
// DOM. It runs after all matches are collected, so component and component.exists reads can
// see sibling and descendant matches.
//
// A property is dropped silently when it does not resolve: empty text, an
// attribute the node does not carry, a PATH that matches no descendant
// component, or a value read from a node whose effective privacy withholds it.
// The last case is also recorded in the match's Withheld.
func resolveComponentProperties(r *propResolver) {
	for node, cm := range r.result {
		def := r.defByNode[node]
		if def == nil || len(def.Properties) == 0 {
			continue
		}
		var props []sightmap.PropertyValue
		var withheld []string
		for _, p := range def.Properties {
			v, ok, held := r.resolveExtract(node, p.Extract)
			switch {
			case ok:
				props = append(props, sightmap.PropertyValue{Name: p.Name, Value: v})
			case held:
				withheld = append(withheld, p.Name)
			}
		}
		cm.Properties = props
		cm.Withheld = withheld
	}
}

// Withholds reports whether the directive alone withholds a value read by e from
// a node whose effective privacy is p (SEP-0009, per SEP-0017). Content is
// withheld under block and mask; mask still permits state, including a dom.attr
// read of an interactive-state attribute. component and component.exists read
// another node and are judged there instead, so they never withhold here. p is
// a resolved directive, as Matcher.Privacy or ComponentMatch.Privacy report it.
//
// It does not see the node, so it cannot apply the one exception that depends on
// it: under mask, a form control's accessible name may still resolve through
// dom.text (see control_name.go). ComponentMatch.Withheld records what Match
// actually withheld.
func Withholds(p string, e sightmap.Extract) bool {
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

// resolveExtract resolves one SEP-0017 directive against node, reporting the
// value, whether it resolved, and, when it did not, whether privacy withheld it.
// References descend only, so recursion strictly enters smaller subtrees and
// always terminates.
//
// Privacy is judged at the node a value is read from: a component read recurses
// into the target, whose own effective privacy then applies, so an unrestricted
// ancestor cannot surface a value out of a blocked descendant.
func (r *propResolver) resolveExtract(node *sightmap.ComponentNode, e sightmap.Extract) (string, bool, bool) {
	if p := r.privacy(node); Withholds(p, e) {
		// Under mask a form control's accessible name labels the field and never
		// carries what was entered in it. Only the name: the rendered-text
		// fallback below is content.
		if p == "mask" && e.From == sightmap.FromDOMText && controlNameSurvives(node, r.treeIndex, r.privacy) {
			v, ok := refine(e, node.Name)
			return v, ok, false
		}
		return "", false, true
	}
	switch e.From {
	case sightmap.FromDOMText:
		// Prefer the accessible name; fall back to the node's rendered text
		// (populated for role-less nodes that have no accessible name). Both are
		// already whitespace-normalized to the same shape at capture time.
		if node.Name != "" {
			return resolved(refine(e, node.Name))
		}
		return resolved(refine(e, node.Text))

	case sightmap.FromDOMRawText:
		// The node's own literal text (direct text nodes, normalized), never the
		// accessibility name (SEP-0013): the deterministic escape when the
		// accessible name welds in extra text, e.g. a CSS ::after badge.
		return resolved(refine(e, node.RawText))

	case sightmap.FromDOMAttr:
		if node.Element == nil {
			return "", false, false
		}
		return resolved(refine(e, node.Element.Attrs[e.Path]))

	case sightmap.FromDOMState:
		// Current interactive state, never the markup attribute of the same
		// name, which records only initial state. A tree captured before State
		// existed still carries the accessibility layer's value in Properties.
		if v := node.State[e.Path]; v != "" {
			return resolved(refine(e, v))
		}
		return resolved(refine(e, node.Properties[e.Path]))

	case sightmap.FromComponentExists:
		target := resolvePath(node, e.Path, r.result)
		if target == nil {
			return "", false, false
		}
		if r.privacy(target) == "block" {
			return "", false, true
		}
		return "true", true, false

	case sightmap.FromComponent:
		dot := strings.LastIndex(e.Path, ".")
		if dot <= 0 || dot == len(e.Path)-1 {
			return "", false, false
		}
		path, prop := e.Path[:dot], e.Path[dot+1:]
		if !strings.Contains(path, "[]") {
			target := resolvePath(node, path, r.result)
			if target == nil {
				return "", false, false
			}
			v, ok, held := r.readProperty(target, prop)
			if !ok {
				return "", false, held
			}
			return resolved(refine(e, v))
		}
		if e.Join == "" {
			return "", false, false // an array-valued result is reserved; validation rejects it
		}
		var vals []string
		anyHeld := false
		for _, target := range resolvePathAll(node, path, r.result) {
			v, ok, held := r.readProperty(target, prop)
			if !ok {
				anyHeld = anyHeld || held
				continue
			}
			if v, ok = refine(e, v); ok {
				vals = append(vals, v)
			}
		}
		if len(vals) == 0 {
			return "", false, anyHeld
		}
		return strings.Join(vals, e.Join), true, false
	}
	return "", false, false
}

// resolved adapts refine's result to resolveExtract's: a value that did not
// resolve here was not withheld.
func resolved(v string, ok bool) (string, bool, bool) { return v, ok, false }

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
func (r *propResolver) readProperty(target *sightmap.ComponentNode, prop string) (string, bool, bool) {
	tdef := r.defByNode[target]
	if tdef == nil {
		return "", false, false
	}
	for _, tp := range tdef.Properties {
		if tp.Name == prop {
			return r.resolveExtract(target, tp.Extract)
		}
	}
	return "", false, false
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
