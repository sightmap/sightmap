package main

import (
	"encoding/json"
	"os"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/sightmap"
)

// annotatedNode is a strict superset of the ComponentNode tree JSON shape.
// It adds component name, memory notes, and extracted prop values from the
// sightmap corpus, omitting those fields when the node is unmatched.
type annotatedNode struct {
	ID            string         `json:"id"`
	Role          string         `json:"role"`
	Name          string         `json:"name,omitempty"`
	Value         string         `json:"value,omitempty"`
	Element       *comps.Element `json:"element,omitempty"`
	Bounds        *comps.Bounds  `json:"bounds,omitempty"`
	IsVisible     bool           `json:"isVisible"`
	IsInteractive bool           `json:"isInteractive"`
	InViewport    bool           `json:"inViewport"`
	IsIgnored     bool           `json:"isIgnored,omitempty"`
	NthChild      int            `json:"nthChild,omitempty"`
	// sightmap-added fields — omitted when the node is unmatched or has no props
	Component string            `json:"component,omitempty"`
	Memory    []string          `json:"memory,omitempty"`
	Props     map[string]string `json:"props,omitempty"`
	Children  []*annotatedNode  `json:"children,omitempty"`
}

// buildAnnotatedNode recursively constructs an annotatedNode tree by joining
// each ComponentNode pointer with its ComponentMatch and any extracted props.
func buildAnnotatedNode(
	node *comps.ComponentNode,
	matches map[*comps.ComponentNode]*sightmap.ComponentMatch,
	propValues map[string]map[string]string, // nodeID → {propName → value}; may be nil
) *annotatedNode {
	an := &annotatedNode{
		ID:            node.Id,
		Role:          node.Role,
		Name:          node.Name,
		Value:         node.Value,
		Element:       node.Element,
		Bounds:        node.Bounds,
		IsVisible:     node.IsVisible,
		IsInteractive: node.IsInteractive,
		InViewport:    node.InViewport,
		IsIgnored:     node.IsIgnored,
		NthChild:      node.NthChild,
	}

	if m, ok := matches[node]; ok && m != nil {
		an.Component = m.Name
		an.Memory = m.Memory
	}

	if propValues != nil {
		if props, ok := propValues[node.Id]; ok && len(props) > 0 {
			an.Props = props
		}
	}

	for _, child := range node.Children {
		an.Children = append(an.Children, buildAnnotatedNode(child, matches, propValues))
	}

	return an
}

// annotatedOutput is the top-level JSON document written by --json.
// It wraps the tree with the matched view name and route so the harness
// can attribute events without parsing the .snap text header.
type annotatedOutput struct {
	View  string         `json:"view,omitempty"`  // sightmap view name (e.g. "PDPPage")
	Route string         `json:"route,omitempty"` // URL route pattern (e.g. "/p/**")
	Tree  *annotatedNode `json:"tree"`
}

// writeAnnotatedJSON writes a machine-readable annotated tree JSON to path.
// Each node in the tree carries the standard ComponentNode fields plus
// component, memory, and props when the node was matched by the sightmap.
// The top-level object also carries the matched view name and route.
// Writes atomically via a .tmp file (same pattern as writeTreeJSON).
func writeAnnotatedJSON(
	root *comps.ComponentNode,
	path string,
	view *sightmap.ViewDef,
	matches map[*comps.ComponentNode]*sightmap.ComponentMatch,
	propValues map[string]map[string]string,
) error {
	if root == nil {
		return nil
	}
	if matches == nil {
		matches = map[*comps.ComponentNode]*sightmap.ComponentMatch{}
	}

	out := annotatedOutput{
		Tree: buildAnnotatedNode(root, matches, propValues),
	}
	if view != nil {
		out.View = view.Name
		out.Route = view.Route
	}

	data, err := json.Marshal(out)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
