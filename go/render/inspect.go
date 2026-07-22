package render

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/match"
)

// InspectNode is a node in the unfiltered DOM-structural tree produced by Inspect().
// Unlike Comp (the filtered semantic tree from Filter()), InspectNode preserves
// every node from the raw component tree and shows full selector details.
type InspectNode struct {
	Id            string
	Tag           string              // element tag (from Selector.Tag or Role fallback)
	Selector      *comps.SelectorPart // full selector from probe.js
	Role          string              // AX role (informational)
	Name          string              // AX accessible name (shown when non-empty)
	IsVisible     bool
	IsIgnored     bool
	IsInteractive bool
	Match         *match.SightmapMatch // non-nil when a sightmap definition matched
	Children      []*InspectNode
	Original      *comps.ComponentNode
}

// InspectOpts controls the output of FormatInspect.
type InspectOpts struct {
	// Classes, if true, includes CSS class names in the selector display.
	// Default: false (classes are too noisy for routine authoring).
	Classes bool
	// Selectors, if true, shows all attributes rather than only the key
	// identifying ones (data-*, aria-*, role, type, href, name, etc.).
	Selectors bool
	// MaxDepth limits recursion. 0 = unlimited.
	MaxDepth int
	// Interactive, if true, skips nodes that are neither interactive nor
	// have any interactive descendants.
	Interactive bool
}

// Inspect builds an unfiltered InspectNode tree from a raw ComponentNode tree.
// Every node is preserved — hidden, ignored, and structural nodes all appear.
// If matches is non-nil, matched nodes have their Match field populated.
func Inspect(root *comps.ComponentNode, matches map[*comps.ComponentNode]*match.SightmapMatch) *InspectNode {
	if root == nil {
		return nil
	}
	return buildInspectNode(root, matches)
}

func buildInspectNode(node *comps.ComponentNode, matches map[*comps.ComponentNode]*match.SightmapMatch) *InspectNode {
	tag := ""
	if node.Selector != nil && node.Selector.Tag != "" {
		tag = node.Selector.Tag
	} else {
		tag = node.Role
	}

	n := &InspectNode{
		Id:            node.Id,
		Tag:           tag,
		Selector:      node.Selector,
		Role:          node.Role,
		Name:          node.Name,
		IsVisible:     node.IsVisible,
		IsIgnored:     node.IsIgnored,
		IsInteractive: node.IsInteractive,
		Original:      node,
	}
	if matches != nil {
		n.Match = matches[node]
	}
	for _, ch := range node.Children {
		n.Children = append(n.Children, buildInspectNode(ch, matches))
	}
	return n
}

// FormatInspect writes the inspect tree to w.
// Each level is indented by 2 spaces. Call with indent="" for the root.
func FormatInspect(w io.Writer, root *InspectNode, opts InspectOpts) {
	if root == nil {
		return
	}
	var intDesc map[*InspectNode]bool
	if opts.Interactive {
		intDesc = buildInspectInteractiveSet(root)
	}
	root.formatInspectNode(w, "", 0, opts, intDesc)
}

func (n *InspectNode) formatInspectNode(w io.Writer, indent string, depth int, opts InspectOpts, intDesc map[*InspectNode]bool) {
	if opts.MaxDepth > 0 && depth > opts.MaxDepth {
		return
	}
	if opts.Interactive && intDesc != nil && !intDesc[n] {
		childIndent := indent + "  "
		for _, ch := range n.Children {
			ch.formatInspectNode(w, childIndent, depth+1, opts, intDesc)
		}
		return
	}

	var line strings.Builder
	line.WriteString(indent)

	// ID prefix (always, when non-empty — go-0045 convention)
	if n.Id != "" {
		line.WriteString(n.Id)
		line.WriteString(" ")
	}

	// Selector display: tag#id[.classes][attrs]
	line.WriteString(inspectSelectorDisplay(n.Selector, n.Tag, opts))

	// Accessible name (shown when non-empty; helps identify interactive elements)
	if n.Name != "" {
		fmt.Fprintf(&line, " %s", quoteName(n.Name))
	}

	// Visibility state annotations
	if !n.IsVisible {
		line.WriteString(" [hidden]")
	}
	if n.IsIgnored {
		line.WriteString(" [ignored]")
	}

	// Sightmap match annotation (★ style — inspect is DOM-centric, not semantic)
	if n.Match != nil {
		fmt.Fprintf(&line, " \u2605%s", n.Match.Name)
	}

	fmt.Fprintln(w, line.String())

	childIndent := indent + "  "
	for _, ch := range n.Children {
		ch.formatInspectNode(w, childIndent, depth+1, opts, intDesc)
	}
}

// inspectSelectorDisplay builds the CSS-selector-style display string for an
// InspectNode line.
//
// Default (no flags): tag#id [data-*] [aria-*] [role] [type] [href] [name] [for] [action] [src] [placeholder]
// --classes:          also prepends .class1.class2
// --selectors:        shows ALL attributes (complete probe.js output)
func inspectSelectorDisplay(sel *comps.SelectorPart, tag string, opts InspectOpts) string {
	if sel == nil {
		return tag
	}
	var sb strings.Builder

	if sel.Tag != "" {
		sb.WriteString(sel.Tag)
	} else if tag != "" {
		sb.WriteString(tag)
	}

	if sel.Id != "" {
		sb.WriteString("#")
		sb.WriteString(sel.Id)
	}

	if opts.Classes {
		for _, cls := range sel.Classes {
			sb.WriteString(".")
			sb.WriteString(cls)
		}
	}

	// Collect attribute keys according to display mode.
	var attrKeys []string
	if opts.Selectors {
		// All attributes, sorted.
		attrKeys = sortedKeys(sel.Attrs)
	} else {
		// Key identifying attributes only.
		for k := range sel.Attrs {
			if isInspectKeyAttr(k) {
				attrKeys = append(attrKeys, k)
			}
		}
		sort.Strings(attrKeys)
	}

	for _, k := range attrKeys {
		fmt.Fprintf(&sb, "[%s=%q]", k, sel.Attrs[k])
	}

	return sb.String()
}

// isInspectKeyAttr returns true for attributes that are useful for sightmap
// authoring by default. data-* and aria-* are always key; a fixed set of
// standard attributes that affect identity or behaviour are also included.
func isInspectKeyAttr(key string) bool {
	if strings.HasPrefix(key, "data-") || strings.HasPrefix(key, "aria-") {
		return true
	}
	switch key {
	case "role", "type", "href", "name", "for", "action", "src", "placeholder":
		return true
	}
	return false
}

// buildInspectInteractiveSet returns a set of InspectNodes that are themselves
// interactive or have at least one interactive descendant.
func buildInspectInteractiveSet(root *InspectNode) map[*InspectNode]bool {
	set := make(map[*InspectNode]bool)
	markInspectInteractive(root, set)
	return set
}

func markInspectInteractive(n *InspectNode, set map[*InspectNode]bool) bool {
	if n == nil {
		return false
	}
	isInt := n.IsInteractive
	for _, ch := range n.Children {
		if markInspectInteractive(ch, set) {
			isInt = true
		}
	}
	if isInt {
		set[n] = true
	}
	return isInt
}
