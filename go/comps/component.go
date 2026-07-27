package comps

import "encoding/json"

// SelectorPart is the structured CSS identity of a DOM element, or a
// synthetic identity for native mobile elements. It carries no proto
// dependency and mirrors the fs/selectors.SelectorPart field-for-field.
//
// For CSS selectors, Tag is the lowercase element name ("div", "button").
// For native mobile, Tag is the platform type name ("UIButton", "android.widget.Button").
type SelectorPart struct {
	Tag     string            `json:"tag,omitempty"`
	Id      string            `json:"id,omitempty"`
	Classes []string          `json:"classes,omitempty"`
	Attrs   map[string]string `json:"attrs,omitempty"`
	// AttrOps holds the matching operator for each entry in Attrs where the
	// operator is not exact equality. Omitted entries default to "=".
	// Valid operators: "=" (default), "^=", "$=", "*=", "~=", "|=", "[]" (presence-only).
	AttrOps map[string]string `json:"attrOps,omitempty"`
	// Not, if non-nil, is a selector that the element must NOT match.
	// Represents the :not() pseudo-class.
	Not *SelectorPart `json:"not,omitempty"`
	// Is, if non-empty, contains the alternatives from a :is() or :where()
	// pseudo-class. The element must match at least one alternative.
	Is []*SelectorPart `json:"is,omitempty"`
	// Has, if non-empty, contains one entry per :has() pseudo-class on this
	// compound selector. The element matches only if its subtree satisfies
	// EVERY entry (multiple :has() are AND-ed). Evaluating :has() requires tree
	// context, so it is handled by sel.MatchesNode (not the flat sel.Matches).
	Has []*HasSelector `json:"has,omitempty"`
}

// HasSelector represents a single :has() relational pseudo-class. The anchor
// element matches if its subtree satisfies ANY of the comma-separated
// Alternatives (a relative selector list).
type HasSelector struct {
	Alternatives []HasRelative `json:"alternatives"`
}

// HasRelative is one relative selector inside :has(). Parts holds the compound
// selectors in anchor→leaf order; Combinators[i] is the combinator linking the
// previous node to Parts[i]. Combinators[0] links the :has() anchor element to
// Parts[0] (" " = any descendant, ">" = direct child).
type HasRelative struct {
	Parts       []*SelectorPart `json:"parts"`
	Combinators []string        `json:"combinators"`
}

// Bounds holds the bounding box of a component in viewport coordinates.
type Bounds struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// ComponentNode is the shared component tree node type. It is produced by the
// extraction pipeline (probe.js + A11Y merge) and consumed by the sightmap
// rule matcher and display layer.
//
// Fields marked "pre-merge" are populated by probe.js before A11Y data is
// available. Fields marked "post-merge" are populated by extract.BuildTree
// after merging the A11Y tree. See conformance/component-schema.md for the
// full field contract.
type ComponentNode struct {
	// Id is a globally unique identifier assigned during extraction.
	// For multi-frame pages, frame-prefixed IDs are used (e.g. "1_5").
	Id string `json:"id"`

	// Role is the WAI-ARIA role from the accessibility tree. (post-merge)
	// Special values: "StaticText" for text nodes, "none" for ignored elements.
	Role string `json:"role"`

	// Name is the computed accessible name from Accessibility.getFullAXTree.
	// This is NOT raw textContent. (post-merge)
	Name string `json:"name"`

	// Value is the current value for form controls (inputs, selects, etc.). (post-merge)
	Value string `json:"value"`

	// Properties holds additional A11Y properties (aria-* and others). (post-merge)
	Properties map[string]string `json:"properties,omitempty"`

	// Selector is the structured CSS identity of the underlying DOM element. (pre-merge)
	// Nil for nodes that have no corresponding DOM element (e.g. virtual nodes).
	Selector *SelectorPart `json:"selector,omitempty"`

	// Bounds is the viewport bounding box. (pre-merge)
	Bounds *Bounds `json:"bounds,omitempty"`

	// IsVisible reports whether the element is EFFECTIVELY visible. probe.js
	// computes it in-browser via Element.checkVisibility, so it already accounts
	// for ancestor-driven hiding (display:none, visibility:hidden, opacity:0,
	// content-visibility): a node inside a hidden container is false too.
	// Coverage and the tree renderer both read this one signal. (pre-merge)
	IsVisible bool `json:"isVisible"`

	// IsInteractive reports whether the element is actionable (clickable,
	// focusable, etc.) according to probe heuristics. (pre-merge)
	IsInteractive bool `json:"isInteractive"`

	// InViewport reports whether the element's bounds intersect the viewport. (pre-merge)
	InViewport bool `json:"inViewport"`

	// IsIgnored reports whether the A11Y tree marks this node as ignored. (post-merge)
	IsIgnored bool `json:"isIgnored"`

	// NthChild is the 1-based position of this node among its parent's children. (pre-merge)
	NthChild int `json:"nthChild"`

	// Children holds the direct children of this node in document order.
	Children []*ComponentNode `json:"children,omitempty"`
}

// UnmarshalJSON implements lenient unmarshaling with sensible defaults for test fixtures.
// This allows minimal JSON fixtures to omit common default values while maintaining
// backward compatibility with fully-populated snapshot files.
func (n *ComponentNode) UnmarshalJSON(data []byte) error {
	// Define an alias type to avoid recursion when calling json.Unmarshal
	type Alias ComponentNode

	// Set defaults before unmarshaling
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(n),
	}

	// Apply defaults for commonly omitted fields
	n.IsVisible = true // default: visible (most common case)
	// IsInteractive defaults to false (already zero value)
	// InViewport defaults to false (already zero value)
	// IsIgnored defaults to false (already zero value)
	// NthChild defaults to 0 (already zero value)

	// Unmarshal the JSON, which will overwrite defaults with actual values
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Ensure slices and maps are never nil (empty instead)
	if n.Properties == nil {
		n.Properties = make(map[string]string)
	}
	if n.Children == nil {
		n.Children = []*ComponentNode{}
	}

	return nil
}
