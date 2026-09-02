package extract

import "github.com/sightmap/sightmap/go/sightmap"

// ProbeResult is the top-level response from running probe.js in the page.
type ProbeResult struct {
	Success bool             `json:"success"`
	Results []ProbeComponent `json:"results"`
	Error   string           `json:"error,omitempty"`
	Debug   string           `json:"debug,omitempty"`
}

// ProbeComponent is the JSON output of probe.js for one element.
// The Selector field is a CSS selector string.
// probe.js will output a structured object; for now, parse
// the string using sightmap.ParseSightmapSelector.
type ProbeComponent struct {
	Id            string            `json:"id"`
	Role          string            `json:"role"`  // empty pre-merge
	Text          string            `json:"text"`  // raw textContent
	Value         string            `json:"value"` // empty pre-merge
	Properties    map[string]string `json:"properties"`
	Bounds        *sightmap.Bounds  `json:"bounds"`
	Selector      string            `json:"selector"`   // CSS selector string
	Attributes    map[string]string `json:"attributes"` // full attribute set (minus data-sightmap-*)
	OnTop         bool              `json:"onTop"`
	IsVisible     bool              `json:"isVisible"`
	InViewport    bool              `json:"inViewport"`
	IsInteractive bool              `json:"isInteractive"`
	IsIgnored     bool              `json:"isIgnored"`
	Metadata      map[string]string `json:"metadata"`
	NthChild      int               `json:"nthChild"`
	TagName       string            `json:"tagName"`
}

// A11YNode is one node from Chrome's Accessibility.getFullAXTree response.
// We mirror the shape without importing chromedp.
type A11YNode struct {
	NodeID           string     `json:"nodeId"` // CDP AXNodeId is a string
	BackendDOMNodeID int        `json:"backendDOMNodeId"`
	Role             *A11YValue `json:"role"`
	Name             *A11YValue `json:"name"`
	Value            *A11YValue `json:"value"`
	Description      *A11YValue `json:"description"`
	Ignored          bool       `json:"ignored"`
	Properties       []A11YProp `json:"properties"`
}

// A11YValue is a CDP AXValue — role/name/value are wrapped objects.
type A11YValue struct {
	Value interface{} `json:"value"`
}

// A11YProp is a name/value pair in the accessibility properties list.
type A11YProp struct {
	Name  string     `json:"name"`
	Value *A11YValue `json:"value"`
}

// DOMNode is a stripped-down CDP DOM tree node used to reconstruct
// parent-child relationships and locate BackendNodeIDs.
type DOMNode struct {
	NodeID        int        `json:"nodeId"`
	BackendNodeID int        `json:"backendNodeId"`
	NodeName      string     `json:"nodeName"`   // "#text", "BUTTON", etc.
	Attributes    []string   `json:"attributes"` // flat [name, val, name, val, ...]
	Children      []*DOMNode `json:"children"`
	ShadowRoots   []*DOMNode `json:"shadowRoots,omitempty"`
}
