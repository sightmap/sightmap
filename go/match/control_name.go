package match

import (
	"slices"
	"strings"

	"github.com/sightmap/sightmap/go/sightmap"
)

// This file decides the one exception to SEP-0009's text withholding: under
// mask, a form control's accessible name may surface when, by construction, it
// cannot carry what the user entered. The name of an <input>, <textarea> or
// <select> comes from aria-labelledby, aria-label, an associated <label>,
// title or placeholder (HTML-AAM), never from the field's value, so it labels
// the field rather than reporting its contents. Every condition below is a
// way the name could still pick up content, and each one withholds:
//
//   - the element is not one of those controls, or is one whose name is its
//     own rendered content (input type button/submit/reset/image), names a
//     chosen file (file), or is never rendered (hidden);
//   - its role is not a form-control role (an ARIA role override such as
//     role=button lets name-from-content reach the control);
//   - aria-labelledby names an element that cannot be vetted: missing from the
//     tree, masked or blocked anywhere in its subtree, or holding a control
//     whose value the name computation would embed (the labelled control
//     itself included, as in aria-labelledby="self");
//   - an associated <label> (an ancestor label, or label[for] naming the
//     control's id) is masked or blocked anywhere in its subtree, or holds a
//     control other than this one;
//   - no source on the control explains a non-empty name;
//   - the name contains the control's own content: its current value, a text
//     input's value attribute, a textarea's text, or a select's option text.
//
// block still withholds everything: a blocked element is not captured at all.

// controlRoles are the computed roles of a form control whose name is never
// taken from its contents. The capitalized ones are Chromium's internal roles
// for date, time and color inputs.
var controlRoles = []string{
	"textbox", "searchbox", "combobox", "listbox", "checkbox", "radio", "switch",
	"spinbutton", "slider", "Date", "DateTime", "InputTime", "ColorWell",
}

// labelledInputTypes are the <input> types whose accessible name is a label for
// the control rather than its own content. A missing or unknown type is text.
var labelledInputTypes = []string{
	"text", "search", "email", "tel", "url", "password", "number",
	"date", "datetime-local", "month", "week", "time",
	"checkbox", "radio", "range", "color",
}

// unlabelledInputTypes are the input types whose name is not a label: a
// button's is its own text, an image's its alt, a file input's the chosen file,
// and a hidden input has none.
var unlabelledInputTypes = []string{"button", "submit", "reset", "image", "file", "hidden"}

// valueRoles are the roles of an element whose value the accessible name
// computation embeds when it meets one inside a label (accname 1.2, step 2C).
var valueRoles = []string{
	"textbox", "searchbox", "combobox", "listbox", "slider", "spinbutton",
	"scrollbar", "progressbar", "meter",
}

// treeIndex is what the name checks need to look outside a node: parents for
// ancestor labels, ids for aria-labelledby, and label[for]. Built once per
// Match, and only when a masked control's name is read.
type treeIndex struct {
	parent    map[*sightmap.ComponentNode]*sightmap.ComponentNode
	byID      map[string][]*sightmap.ComponentNode
	labelsFor map[string][]*sightmap.ComponentNode
}

func buildTreeIndex(root *sightmap.ComponentNode) *treeIndex {
	ix := &treeIndex{
		parent:    map[*sightmap.ComponentNode]*sightmap.ComponentNode{},
		byID:      map[string][]*sightmap.ComponentNode{},
		labelsFor: map[string][]*sightmap.ComponentNode{},
	}
	var walk func(n *sightmap.ComponentNode)
	walk = func(n *sightmap.ComponentNode) {
		if id := elementID(n); id != "" {
			ix.byID[id] = append(ix.byID[id], n)
		}
		if tag(n) == "label" {
			if f := strings.TrimSpace(n.Element.Attrs["for"]); f != "" {
				ix.labelsFor[f] = append(ix.labelsFor[f], n)
			}
		}
		for _, c := range n.Children {
			if c.IsFrameRoot() {
				continue // an id or label in another document is not this one's
			}
			ix.parent[c] = n
			walk(c)
		}
	}
	if root != nil {
		walk(root)
	}
	return ix
}

func elementID(n *sightmap.ComponentNode) string {
	if n.Element == nil {
		return ""
	}
	if n.Element.Id != "" {
		return n.Element.Id
	}
	return n.Element.Attrs["id"]
}

func tag(n *sightmap.ComponentNode) string {
	if n == nil || n.Element == nil {
		return ""
	}
	return strings.ToLower(n.Element.Tag)
}

func attr(n *sightmap.ComponentNode, name string) string {
	if n == nil || n.Element == nil {
		return ""
	}
	return n.Element.Attrs[name]
}

func inputType(n *sightmap.ComponentNode) string {
	t := strings.ToLower(strings.TrimSpace(attr(n, "type")))
	switch {
	case slices.Contains(labelledInputTypes, t):
		return t
	case slices.Contains(unlabelledInputTypes, t):
		return t
	default:
		return "text" // HTML: a missing or unknown type is the text state
	}
}

// isLabelledControl reports whether n is a form control whose accessible name
// labels it rather than repeating its contents.
func isLabelledControl(n *sightmap.ComponentNode) bool {
	switch tag(n) {
	case "input":
		if !slices.Contains(labelledInputTypes, inputType(n)) {
			return false
		}
	case "textarea", "select":
	default:
		return false
	}
	if r := strings.TrimSpace(attr(n, "role")); r != "" && !slices.Contains(controlRoles, r) {
		return false
	}
	return slices.Contains(controlRoles, n.Role)
}

// embedsValue reports whether the accessible name computation would take a
// value out of n when n sits inside a label or an aria-labelledby target.
func embedsValue(n *sightmap.ComponentNode) bool {
	switch tag(n) {
	case "input":
		return inputType(n) != "hidden"
	case "textarea", "select":
		return true
	}
	if ce, ok := n.Element.Attrs["contenteditable"]; ok && !strings.EqualFold(ce, "false") {
		return true
	}
	return slices.Contains(valueRoles, strings.TrimSpace(attr(n, "role"))) || slices.Contains(valueRoles, n.Role)
}

// controlNameSurvives reports whether node's accessible name may surface under
// mask (see the file comment). privacy is the effective privacy of any node.
func controlNameSurvives(
	node *sightmap.ComponentNode,
	index func() *treeIndex,
	privacy func(*sightmap.ComponentNode) string,
) bool {
	if node.Element == nil || node.Name == "" || !isLabelledControl(node) {
		return false
	}
	if containsOwnContent(node) {
		return false
	}
	ix := index()
	// vetted: no masked or blocked node and no value-bearing control in r's
	// subtree, except self's own subtree when self is allowed (a label wrapping
	// the control it labels).
	var vetted func(r, self *sightmap.ComponentNode) bool
	vetted = func(r, self *sightmap.ComponentNode) bool {
		if r == self {
			return true
		}
		if p := privacy(r); p == "mask" || p == "block" {
			return false
		}
		if r.Element != nil && embedsValue(r) {
			return false
		}
		for _, c := range r.Children {
			if !vetted(c, self) {
				return false
			}
		}
		return true
	}

	// 1. aria-labelledby wins whenever it is present; vet every reference, and
	// the labelled control itself is never allowed inside one.
	if refs := strings.Fields(attr(node, "aria-labelledby")); len(refs) > 0 {
		for _, id := range refs {
			targets := ix.byID[id]
			if len(targets) == 0 {
				return false
			}
			for _, t := range targets {
				if !vetted(t, nil) {
					return false
				}
			}
		}
		return true
	}
	// 2. aria-label is the name as written.
	if al := strings.TrimSpace(attr(node, "aria-label")); al != "" {
		return node.Name == normalize(al)
	}
	// 3. Associated labels, then title and placeholder.
	var labels []*sightmap.ComponentNode
	for p := ix.parent[node]; p != nil; p = ix.parent[p] {
		if tag(p) == "label" {
			labels = append(labels, p)
		}
	}
	if id := elementID(node); id != "" {
		labels = append(labels, ix.labelsFor[id]...)
	}
	for _, l := range labels {
		if !vetted(l, node) {
			return false
		}
	}
	if len(labels) > 0 {
		return true
	}
	for _, a := range []string{"title", "placeholder"} {
		if v := normalize(attr(node, a)); v != "" && node.Name == v {
			return true
		}
	}
	// Nothing on the control explains the name.
	return false
}

// containsOwnContent reports whether the name repeats anything the control
// holds: its current value, a text input's value attribute (a checkbox's or
// radio's is the author's submission value, not content), a textarea's text,
// or a select's option text.
func containsOwnContent(n *sightmap.ComponentNode) bool {
	contents := []string{n.Value, n.Properties["value"]}
	switch tag(n) {
	case "input":
		if t := inputType(n); t != "checkbox" && t != "radio" {
			contents = append(contents, attr(n, "value"))
		}
	case "textarea":
		contents = append(contents, n.Text, n.RawText)
	case "select":
		var walk func(c *sightmap.ComponentNode)
		walk = func(c *sightmap.ComponentNode) {
			if tag(c) == "option" {
				contents = append(contents, c.Name, c.Text, c.RawText, attr(c, "label"))
			}
			for _, k := range c.Children {
				walk(k)
			}
		}
		walk(n)
	}
	for _, c := range contents {
		if c = normalize(c); c != "" && strings.Contains(n.Name, c) {
			return true
		}
	}
	return false
}

func normalize(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
