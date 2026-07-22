package extract

import (
	"fmt"
	"strings"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/sel"
)

// BuildTree runs the full post-probe pipeline:
//  1. Build BackendNodeID → A11Y node map
//  2. For each DOM node that has a data-sightmap-id attribute, look up the
//     ProbeComponent and merge in A11Y role/name/value/ignored/properties
//  3. Walk domRoot to reconstruct parent-child links, set NthChild
//  4. Return the root ComponentNode
//
// The string selector from ProbeComponent is parsed into a SelectorPart using
// sel.ParseSightmapSelector. On parse error, a SelectorPart with just Tag set
// (from TagName) is used as a fallback.
func BuildTree(
	probe *ProbeResult,
	a11y []A11YNode,
	domRoot *DOMNode,
) (*comps.ComponentNode, error) {
	// 1. Index probe results by ID.
	probeByID := make(map[string]*ProbeComponent, len(probe.Results))
	for i := range probe.Results {
		p := &probe.Results[i]
		probeByID[p.Id] = p
	}

	// 2. Build A11Y lookup map keyed by BackendDOMNodeID.
	a11yByID := make(map[int]*A11YNode, len(a11y))
	for i := range a11y {
		n := &a11y[i]
		a11yByID[n.BackendDOMNodeID] = n
	}

	// 3. Walk the DOM tree.
	root, children := walkDOMNode(domRoot, probeByID, a11yByID)
	if root != nil {
		return root, nil
	}

	// domRoot had no sightmap ID; return the first top-level component child.
	if len(children) > 0 {
		return children[0], nil
	}

	return nil, fmt.Errorf("extract: no component nodes found in DOM tree")
}

// walkDOMNode recursively walks the DOM subtree rooted at node.
//
// It returns (thisNode, compChildren):
//   - thisNode is the ComponentNode for this DOM node if it carries a
//     data-sightmap-id attribute; otherwise nil.
//   - compChildren is the ordered list of ComponentNodes found among this
//     node's descendants (used when thisNode is nil so callers can attach them
//     to the nearest ancestor that does have a sightmap ID).
func walkDOMNode(
	node *DOMNode,
	probeByID map[string]*ProbeComponent,
	a11yByID map[int]*A11YNode,
) (*comps.ComponentNode, []*comps.ComponentNode) {
	// Treat shadow roots as ordinary children (appended after regular children
	// to preserve document order of the light tree first).
	allDOM := make([]*DOMNode, 0, len(node.Children)+len(node.ShadowRoots))
	allDOM = append(allDOM, node.Children...)
	allDOM = append(allDOM, node.ShadowRoots...)

	// Recursively collect component children.
	var compChildren []*comps.ComponentNode
	for _, child := range allDOM {
		childNode, grandchildren := walkDOMNode(child, probeByID, a11yByID)
		if childNode != nil {
			compChildren = append(compChildren, childNode)
		} else {
			// No component node here — promote descendants to this level.
			compChildren = append(compChildren, grandchildren...)
		}
	}

	// Assign 1-based NthChild positions among the collected component children
	// before we attach them to any parent.
	for i, c := range compChildren {
		c.NthChild = i + 1
	}

	// Check whether this DOM node is tagged with a sightmap ID.
	sightmapID := domAttr(node.Attributes, "data-sightmap-id")
	if sightmapID == "" {
		return nil, compChildren
	}

	pc := probeByID[sightmapID]
	if pc == nil {
		// Sightmap ID present but no matching probe component — skip gracefully.
		return nil, compChildren
	}

	// Parse the CSS selector string; fall back to TagName on error.
	sp := selectorPartFor(pc)

	// Merge A11Y data.
	a11yNode := a11yByID[node.BackendNodeID]
	role, name, value, ignored, props := mergeA11Y(pc, a11yNode)

	compNode := &comps.ComponentNode{
		Id:            pc.Id,
		Role:          role,
		Name:          name,
		Value:         value,
		Properties:    props,
		Selector:      sp,
		Bounds:        pc.Bounds,
		IsVisible:     pc.IsVisible,
		IsInteractive: pc.IsInteractive,
		InViewport:    pc.InViewport,
		IsIgnored:     ignored,
		Children:      compChildren,
		// NthChild is set by the parent's walkDOMNode call.
	}

	return compNode, nil
}

// selectorPartFor parses ProbeComponent.Selector with sel.ParseSightmapSelector.
// If parsing succeeds it returns the last (most-specific) part. On any error it
// falls back to a SelectorPart with Tag = strings.ToLower(pc.TagName).
//
// The returned SelectorPart has its Classes duplicated into Attrs["class"] to
// support attribute selectors like [class*="partial"], [class^="prefix-"], etc.
func selectorPartFor(pc *ProbeComponent) *comps.SelectorPart {
	var sp *comps.SelectorPart
	if pc.Selector != "" {
		parsed, err := sel.ParseSightmapSelector(pc.Selector)
		if err == nil && len(parsed.Parts) > 0 {
			sp = parsed.Parts[len(parsed.Parts)-1]
		}
	}
	if sp == nil {
		sp = &comps.SelectorPart{Tag: strings.ToLower(pc.TagName)}
	}

	// Duplicate classes into Attrs["class"] for attribute selector support.
	if len(sp.Classes) > 0 {
		if sp.Attrs == nil {
			sp.Attrs = make(map[string]string)
		}
		sp.Attrs["class"] = strings.Join(sp.Classes, " ")
	}

	return sp
}

// mergeA11Y derives role/name/value/ignored/properties from the A11Y tree.
//
// Rules:
//   - A11Y node found → populate from a11y fields; "WebArea" role → "document".
//   - A11Y node absent, TagName == "#text" → Role = "StaticText".
//   - A11Y node absent, any other tag     → Role = "none", IsIgnored = true.
func mergeA11Y(
	pc *ProbeComponent,
	a11y *A11YNode,
) (role, name, value string, ignored bool, props map[string]string) {
	if a11y != nil {
		role = a11yString(a11y.Role)
		if role == "WebArea" {
			role = "document"
		}
		name = a11yString(a11y.Name)
		value = a11yString(a11y.Value)
		ignored = a11y.Ignored

		if len(a11y.Properties) > 0 {
			props = make(map[string]string, len(a11y.Properties))
			for _, p := range a11y.Properties {
				props[p.Name] = a11yString(p.Value)
			}
		}
		return
	}

	// No A11Y node.
	if pc.TagName == "#text" {
		role = "StaticText"
	} else {
		role = "none"
		ignored = true
	}
	return
}

// a11yString converts an *A11YValue to a plain string.
// The Value field is an interface{} decoded from JSON and may be a string,
// bool, float64, or nil. Returns "" for nil receiver or nil value.
func a11yString(v *A11YValue) string {
	if v == nil || v.Value == nil {
		return ""
	}
	return fmt.Sprintf("%v", v.Value)
}

// domAttr reads a named attribute from a flat CDP attributes slice whose
// layout is [name₀, val₀, name₁, val₁, …]. Returns "" if not found.
func domAttr(attrs []string, name string) string {
	for i := 0; i+1 < len(attrs); i += 2 {
		if attrs[i] == name {
			return attrs[i+1]
		}
	}
	return ""
}
