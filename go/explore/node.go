// Package explore drives a browser toward a goal one step at a time: observe the
// page as an annotated component tree, turn its interactive nodes into a short
// list of named actions, ask a Picker (a small typed model such as Jev, or a
// big model) which one to take, act, wait for the page to settle, repeat.
//
// The Picker only ever answers typed questions (pick one of these, yes/no). It
// never invents text: every value the loop types comes from the Spec.
package explore

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sightmap/sightmap/go/observe"
	"github.com/sightmap/sightmap/go/sightmap"
)

// Node is one node of the annotated tree, flattened with the ancestry context
// the loop needs to describe and group it.
type Node struct {
	ID          string
	Role        string
	Name        string
	Text        string
	Value       string
	Comp        string            // matched component name, "" when unmatched
	Props       map[string]string // extracted property values (matched nodes only)
	Interactive bool
	Visible     bool
	InViewport  bool
	Tag         string
	Attrs       map[string]string
	Classes     []string
	Depth       int
	ParentComp  *Node   // nearest ancestor that matched a component (nil when none)
	Landmark    *Node   // nearest ancestor with a landmark role (nil when none)
	Parent      *Node   // direct parent
	Ancestors   []*Node // root-first
	Raw         *sightmap.ComponentNode
}

// Page is one observation of the live page.
type Page struct {
	URL    string
	View   string // matched sightmap view name, "" when none
	Route  string
	Nodes  []*Node
	Result *observe.Result
}

var landmarkRoles = map[string]bool{
	"banner": true, "navigation": true, "main": true, "contentinfo": true, "complementary": true,
	"form": true, "region": true, "dialog": true, "search": true, "list": true, "table": true,
	"grid": true, "menu": true, "tablist": true,
}

// Flatten walks an observation into Nodes, joining each tree node with its match.
func Flatten(res *observe.Result) []*Node {
	if res == nil || res.Root == nil {
		return nil
	}
	var out []*Node
	var walk func(n *sightmap.ComponentNode, depth int, parent, parentComp, landmark *Node, anc []*Node)
	walk = func(n *sightmap.ComponentNode, depth int, parent, parentComp, landmark *Node, anc []*Node) {
		node := &Node{
			ID:          n.Id,
			Role:        n.Role,
			Name:        n.Name,
			Text:        n.Text,
			Value:       n.Value,
			Interactive: n.IsInteractive,
			Visible:     n.IsVisible,
			InViewport:  n.InViewport,
			Depth:       depth,
			Parent:      parent,
			ParentComp:  parentComp,
			Landmark:    landmark,
			Ancestors:   anc,
			Raw:         n,
			Props:       map[string]string{},
			Attrs:       map[string]string{},
		}
		if n.Element != nil {
			node.Tag = n.Element.Tag
			node.Classes = n.Element.Classes
			if n.Element.Attrs != nil {
				node.Attrs = n.Element.Attrs
			}
		}
		if m := res.Matches[n]; m != nil {
			node.Comp = m.Name
			for _, p := range m.Properties {
				node.Props[p.Name] = p.Value
			}
		}
		out = append(out, node)
		nextComp := parentComp
		if node.Comp != "" {
			nextComp = node
		}
		nextLandmark := landmark
		if landmarkRoles[node.Role] {
			nextLandmark = node
		}
		childAnc := append(append([]*Node{}, anc...), node)
		for _, c := range n.Children {
			walk(c, depth+1, node, nextComp, nextLandmark, childAnc)
		}
	}
	walk(res.Root, 0, nil, nil, nil, nil)
	return out
}

// NewPage builds a Page from an observation and the page URL.
func NewPage(res *observe.Result, url string) *Page {
	p := &Page{URL: url, Result: res, Nodes: Flatten(res)}
	if res != nil && res.View != nil {
		p.View = res.View.Name
		p.Route = res.View.Route
	}
	return p
}

// CompLabel renders a matched node as [Name prop="value" ...], or "" when unmatched.
func CompLabel(n *Node) string {
	if n == nil || n.Comp == "" {
		return ""
	}
	keys := make([]string, 0, len(n.Props))
	for k, v := range n.Props {
		if v != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("[")
	b.WriteString(n.Comp)
	for _, k := range keys {
		fmt.Fprintf(&b, " %s=%q", k, trunc(n.Props[k], 40))
	}
	b.WriteString("]")
	return b.String()
}

// Describe renders a node the way the picker sees it: component label, role,
// accessible name, value, input type, href for bare links, and the owning component.
func Describe(n *Node) string {
	var parts []string
	if n.Comp != "" {
		parts = append(parts, CompLabel(n))
	}
	role := n.Role
	if role == "" {
		role = n.Tag
	}
	if n.Name != "" {
		parts = append(parts, fmt.Sprintf("%s %q", role, trunc(n.Name, 60)))
	} else {
		parts = append(parts, role)
	}
	if n.Value != "" && !propHasValue(n, n.Value) {
		parts = append(parts, fmt.Sprintf("value=%q", trunc(n.Value, 30)))
	}
	if n.Tag == "input" {
		if t := n.Attrs["type"]; t != "" && t != "text" && t != "submit" && t != "button" {
			parts = append(parts, "type="+t)
		}
	}
	if n.Tag == "a" && n.Comp == "" {
		if h := n.Attrs["href"]; h != "" {
			parts = append(parts, "href="+trunc(h, 50))
		}
	}
	if n.ParentComp != nil && n.ParentComp.Comp != n.Comp {
		parts = append(parts, "in "+CompLabel(n.ParentComp))
	}
	return strings.Join(parts, " ")
}

func propHasValue(n *Node, v string) bool {
	for _, pv := range n.Props {
		if pv == v {
			return true
		}
	}
	return false
}

var textInputRoles = map[string]bool{"textbox": true, "searchbox": true, "spinbutton": true}

// IsTextInput reports whether the node takes typed text.
func IsTextInput(n *Node) bool {
	if n.Tag == "textarea" || textInputRoles[n.Role] {
		return true
	}
	if n.Tag == "input" {
		switch strings.ToLower(n.Attrs["type"]) {
		case "", "text", "password", "email", "number", "search", "tel", "url":
			return true
		}
		return false
	}
	return n.Attrs["contenteditable"] == "true"
}

// IsSelect reports whether the node is a native <select>.
func IsSelect(n *Node) bool { return n.Tag == "select" }

// IsCheckable reports whether the node is a checkbox or radio input.
func IsCheckable(n *Node) bool {
	if n.Tag != "input" {
		return false
	}
	t := strings.ToLower(n.Attrs["type"])
	return t == "checkbox" || t == "radio"
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
