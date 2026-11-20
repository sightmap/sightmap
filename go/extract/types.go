package extract

import "github.com/sightmap/sightmap/go/sightmap"

// ProbeResult is the top-level response from running probe.js in the page.
type ProbeResult struct {
	Success bool             `json:"success"`
	Results []ProbeComponent `json:"results"`
	// Truncated is set when the run stopped at its node cap (probe.js's maxNodes
	// argument), leaving part of the document unmapped.
	Truncated bool `json:"truncated,omitempty"`
	// Viewport is the document's own viewport: the iframe's content box for a
	// child frame.
	Viewport struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"viewport"`
	Error string `json:"error,omitempty"`
	Debug string `json:"debug,omitempty"`
}

// ProbeComponent is the JSON output of probe.js for one element.
// The Selector field is a CSS selector string.
// probe.js will output a structured object; for now, parse
// the string using sightmap.ParseSightmapSelector.
type ProbeComponent struct {
	Id            string            `json:"id"`
	Role          string            `json:"role"`    // empty pre-merge
	Text          string            `json:"text"`    // rendered innerText
	RawText       string            `json:"rawText"` // own direct-text-node content (SEP-0013 raw_text)
	Value         string            `json:"value"`   // empty pre-merge
	Properties    map[string]string `json:"properties"`
	Bounds        *sightmap.Bounds  `json:"bounds"`
	Selector      string            `json:"selector"`   // CSS selector string
	Attributes    map[string]string `json:"attributes"` // full attribute set (minus data-sightmap-*)
	State         map[string]string `json:"state"`      // current interactive state (SEP-0013), from native properties
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
	// FrameID is, on an <iframe>/<frame> element, the id of the frame it hosts.
	FrameID string `json:"frameId,omitempty"`
	// ContentDocument is the hosted document of a same-process frame (a frame in
	// another process has none: its DOM comes from its own CDP session).
	ContentDocument *DOMNode `json:"contentDocument,omitempty"`
}
