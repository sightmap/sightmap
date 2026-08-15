package render

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/sightmap/sightmap/go/sightmap"
)

// Comp is a filtered, display-ready component node produced by Filter().
type Comp struct {
	Id       string
	Role     string                   // display role: AX role (interactive) or match name (structural)
	Name     string                   // accessible name; may be collapsed from StaticText children
	Value    string                   // form field value
	Match    *sightmap.ComponentMatch // non-nil when matched by a sightmap definition
	Children []*Comp
	Original *sightmap.ComponentNode // original raw node; .Role has the unmodified AX role
}

// FormatOpts controls the output of Format.
type FormatOpts struct {
	// Selectors, if true, appends a compact DOM selector hint after each node:
	// tag #id [data-testid="..."] [data-component="..."] — no CSS classes.
	Selectors bool
	// MaxDepth limits recursion. 0 = unlimited.
	MaxDepth int
	// Interactive, if true, skips nodes (and collapses their indentation) when
	// neither the node nor any of its Comp descendants is interactive.
	Interactive bool
}

// Filter builds a compact, filtered Comp tree from a raw ComponentNode tree.
// It applies the shouldFilter algorithm:
//   - style/script/noscript tags are dropped (with their subtrees)
//   - invisible nodes are dropped (with their subtrees)
//   - role="none" structural wrappers (without a sightmap match) are made
//     transparent (removed; their children are promoted)
//   - AX-ignored nodes (without a sightmap match) are made transparent
//   - role="generic" nodes with no name, no match, and ≤1 non-text child are
//     transparent
//
// After filtering, adjacent StaticText siblings are merged and a solo
// StaticText child is collapsed into the parent's Name.
//
// If matches is nil or empty, no sightmap annotations are applied.
func Filter(root *sightmap.ComponentNode, matches map[*sightmap.ComponentNode]*sightmap.ComponentMatch) *Comp {
	if root == nil {
		return nil
	}
	results := convert(root, matches)
	switch len(results) {
	case 0:
		return nil
	case 1:
		return results[0]
	default:
		// Shouldn't happen with a valid tree, but handle gracefully.
		return &Comp{Role: "document", Children: results, Original: root}
	}
}

// Format writes the filtered Comp tree to w.
// Each level is indented by 2 spaces. Call with indent="" for the root.
func (c *Comp) Format(w io.Writer, indent string, opts FormatOpts) {
	if c == nil {
		return
	}
	var intDesc map[*Comp]bool
	if opts.Interactive {
		intDesc = buildInteractiveDescSet(c)
	}
	c.formatNode(w, indent, 0, opts, intDesc)
}

func (c *Comp) formatNode(w io.Writer, indent string, depth int, opts FormatOpts, intDesc map[*Comp]bool) {
	if opts.MaxDepth > 0 && depth > opts.MaxDepth {
		return
	}

	// --interactive: skip nodes with no interactive descendants.
	// Still recurse so that interactive children appear at the right depth.
	if opts.Interactive && intDesc != nil && !intDesc[c] {
		childIndent := indent + "  "
		for _, ch := range c.Children {
			ch.formatNode(w, childIndent, depth+1, opts, intDesc)
		}
		return
	}

	var line strings.Builder
	line.WriteString(indent)

	// Component ID prefix — always shown when non-empty.
	if c.Id != "" {
		line.WriteString(c.Id)
		line.WriteString(" ")
	}

	if c.Match != nil {
		// ── Matched: {id} [CompName prop="v"... "text"] ──────────────────────
		line.WriteString("[")
		line.WriteString(c.Role) // already set to match name by convert()

		// Merged props: sightmap-extracted + AX value fallback.
		props := mergedProps(c)

		// Props sorted alphabetically, before text.
		for _, k := range sortedKeys(props) {
			fmt.Fprintf(&line, " %s=%q", k, truncate(props[k], 60))
		}

		// Text last — Rule A: suppress when it exactly equals any prop value.
		if c.Name != "" && !anyPropEquals(c.Name, props) {
			fmt.Fprintf(&line, " %s", quoteName(c.Name))
		}

		line.WriteString("]")
	} else {
		// ── Unmatched: {id} role "name" value="v" ────────────────────────────
		line.WriteString(c.Role)
		if c.Name != "" {
			fmt.Fprintf(&line, " %s", quoteName(c.Name))
		}
		if c.Value != "" {
			fmt.Fprintf(&line, " value=%q", truncate(c.Value, 60))
		}
	}

	// Selector hint (opt-in) — appended after closing bracket for matched nodes.
	if opts.Selectors && c.Original != nil && c.Original.Element != nil {
		if hint := selectorHint(c.Original.Element); hint != "" {
			fmt.Fprintf(&line, " <%s>", hint)
		}
	}

	fmt.Fprintln(w, line.String())

	// Compute parent props once for Rule B check below.
	var parentProps map[string]string
	if c.Match != nil {
		parentProps = mergedProps(c)
	}

	childIndent := indent + "  "
	for _, ch := range c.Children {
		// Rule B: suppress StaticText children whose text exactly equals
		// any prop value of the matched parent.
		if ch.Role == "StaticText" && parentProps != nil && anyPropEquals(ch.Name, parentProps) {
			continue
		}
		ch.formatNode(w, childIndent, depth+1, opts, intDesc)
	}
}

// --- Internal filter pipeline ---

type disposition int

const (
	keepNode        disposition = iota
	makeTransparent             // remove node, promote filtered children
	dropNode                    // remove node and entire subtree
)

func decide(node *sightmap.ComponentNode, matched bool) disposition {
	// Drop non-semantic embedded resources.
	if node.Element != nil {
		switch strings.ToLower(node.Element.Tag) {
		case "style", "script", "noscript":
			return dropNode
		}
	}

	// An invisible node doesn't render itself, but must NOT drop its subtree:
	// visibility is overridable (a `visibility:hidden` ancestor can hold
	// `visibility:visible` descendants) and a zero-size wrapper — notably the
	// document <html> root, which probe.js reports as height 0 / ignored — has
	// fully visible content beneath it. `display:none` is the case the old
	// dropNode was really guarding, and probe.js already propagates that to
	// descendants as effective invisibility, so they arrive here invisible too.
	// Making the node transparent (recurse, drop only the node itself) handles
	// both: a genuinely hidden subtree renders nothing (every node transparent),
	// while visible descendants of an invisible ancestor survive — which keeps the
	// tree in lockstep with coverage, which counts those same visible nodes.
	if !node.IsVisible {
		return makeTransparent
	}

	// Transparent: structural "none" role wrapper without sightmap identity.
	if node.Role == "none" && !matched {
		return makeTransparent
	}

	// Transparent: AX-ignored node without sightmap identity.
	if node.IsIgnored && !matched {
		return makeTransparent
	}

	// Transparent: generic container with no accessible name and no sightmap
	// identity. Mirrors the canonical extractor's shouldFilter exactly:
	//   - empty generic (no children) → transparent
	//   - generic with exactly one non-StaticText child → transparent (pass-through)
	//   - generic with a solo StaticText child → keep (text collapses into parent)
	//   - generic with 2+ children → keep
	if node.Role == "generic" && node.Name == "" && !matched {
		switch len(node.Children) {
		case 0:
			return makeTransparent
		case 1:
			if node.Children[0].Role != "StaticText" {
				return makeTransparent
			}
		}
	}

	return keepNode
}

func convert(node *sightmap.ComponentNode, matches map[*sightmap.ComponentNode]*sightmap.ComponentMatch) []*Comp {
	m := matches[node]
	matched := m != nil

	switch decide(node, matched) {
	case dropNode:
		return nil
	case makeTransparent:
		return convertChildren(node, matches)
	}

	// keepNode: recursively build children.
	children := convertChildren(node, matches)
	children = mergeAdjacentStaticText(children)

	// Collapse a solo StaticText child into the parent Name.
	name := node.Name
	if len(children) == 1 && children[0].Role == "StaticText" {
		if name == "" {
			name = children[0].Name
		}
		children = nil
	} else if len(children) > 0 && allStaticText(children) {
		// All remaining children are text nodes — merge them into Name.
		if name == "" {
			texts := make([]string, 0, len(children))
			for _, ch := range children {
				if ch.Name != "" {
					texts = append(texts, ch.Name)
				}
			}
			name = strings.TrimSpace(strings.Join(texts, " "))
		}
		children = nil
	}

	// If the sole surviving child has the same accessible name as this node,
	// suppress the parent's name — the child will display it. This avoids
	// e.g. `link "Foo" → heading "Foo"` where the heading already shows the text.
	if name != "" && len(children) == 1 && children[0].Name == name {
		name = ""
	}

	// Determine display role. Mirrors the canonical extractor's DisplayRole():
	// match name always replaces the AX role when a sightmap definition matched.
	role := node.Role
	if matched {
		role = m.Name
	}

	return []*Comp{{
		Id:       node.Id,
		Role:     role,
		Name:     name,
		Value:    node.Value,
		Match:    m,
		Children: children,
		Original: node,
	}}
}

func convertChildren(node *sightmap.ComponentNode, matches map[*sightmap.ComponentNode]*sightmap.ComponentMatch) []*Comp {
	var out []*Comp
	for _, ch := range node.Children {
		out = append(out, convert(ch, matches)...)
	}
	return out
}

func mergeAdjacentStaticText(children []*Comp) []*Comp {
	if len(children) == 0 {
		return nil
	}
	out := make([]*Comp, 0, len(children))
	for _, ch := range children {
		if ch.Role == "StaticText" && len(out) > 0 && out[len(out)-1].Role == "StaticText" {
			prev := out[len(out)-1]
			prev.Name = strings.TrimSpace(prev.Name + " " + ch.Name)
		} else {
			out = append(out, ch)
		}
	}
	return out
}

func allStaticText(children []*Comp) bool {
	for _, ch := range children {
		if ch.Role != "StaticText" {
			return false
		}
	}
	return true
}

// buildInteractiveDescSet returns a set of Comp nodes that are themselves
// interactive or have at least one interactive descendant. Used by formatNode
// to skip structural-only nodes when --interactive is set.
func buildInteractiveDescSet(root *Comp) map[*Comp]bool {
	set := make(map[*Comp]bool)
	markInteractive(root, set)
	return set
}

func markInteractive(c *Comp, set map[*Comp]bool) bool {
	if c == nil {
		return false
	}
	isInt := c.Original != nil && c.Original.IsInteractive
	for _, ch := range c.Children {
		if markInteractive(ch, set) {
			isInt = true
		}
	}
	if isInt {
		set[c] = true
	}
	return isInt
}

// mergedProps builds the combined property map for a matched Comp node.
// Sightmap-extracted properties (c.Match.Properties) take precedence.
// If no "value" key is present and the AX Comp.Value is non-empty, it is
// added as "value" (reserved-but-overridable built-in).
// Returns nil when there are no props and no AX value.
func mergedProps(c *Comp) map[string]string {
	var match []sightmap.PropertyValue
	if c.Match != nil {
		match = c.Match.Properties
	}
	if len(match) == 0 && c.Value == "" {
		return nil
	}
	props := make(map[string]string, len(match)+1)
	for _, pv := range match {
		props[pv.Name] = pv.Value
	}
	if _, hasValue := props["value"]; !hasValue && c.Value != "" {
		props["value"] = c.Value
	}
	return props
}

// anyPropEquals reports whether text exactly equals any value in props.
// Used for Rule A (name suppression) and Rule B (StaticText child suppression).
func anyPropEquals(text string, props map[string]string) bool {
	for _, v := range props {
		if text == v {
			return true
		}
	}
	return false
}

// sortedKeys returns the keys of m sorted alphabetically for stable output.
func sortedKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// --- Formatting helpers ---

func quoteName(name string) string {
	name = truncate(name, 80)
	name = strings.ReplaceAll(name, `\`, `\\`)
	name = strings.ReplaceAll(name, `"`, `\"`)
	return `"` + name + `"`
}

func truncate(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "…"
}

// selectorHint builds a compact selector string showing only tag, #id,
// [data-testid], and [data-component] — no CSS classes.
func selectorHint(sel *sightmap.Element) string {
	if sel == nil {
		return ""
	}
	var sb strings.Builder
	if sel.Tag != "" {
		sb.WriteString(sel.Tag)
	}
	if sel.Id != "" {
		sb.WriteString("#")
		sb.WriteString(sel.Id)
	}
	for _, key := range []string{"data-testid", "data-component"} {
		if v, ok := sel.Attrs[key]; ok {
			fmt.Fprintf(&sb, ` [%s=%q]`, key, v)
		}
	}
	return sb.String()
}
