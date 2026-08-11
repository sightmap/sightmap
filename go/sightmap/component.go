package sightmap

import "github.com/sightmap/sightmap/go/comps"

// ComponentDef is a single flattened sightmap component definition.
// Hierarchical YAML selectors should be pre-flattened by the caller into
// compound descendant selectors before compiling into match queries.
type ComponentDef struct {
	Name        string     `json:"name"`
	Selectors   []string   `json:"selectors"`
	Source      string     `json:"source,omitempty"`
	Memory      []string   `json:"memory,omitempty"`
	Tags        []string   `json:"tags,omitempty"`
	Properties  []Property `json:"properties,omitempty"`
	ParentChain []string   `json:"parentChain,omitempty"` // ancestor component names, root-first
	Stability   string     `json:"stability,omitempty"`   // "" (default), "uncertain", or "unstable"
}

// Property describes a value to extract from a matched DOM element.
type Property struct {
	Name      string `json:"name"`
	Extract   string `json:"extract"`   // see extract modes: text, inner_text, text_only, attr=NAME, exists:SEL, CSS selector
	Transform string `json:"transform"` // optional post-processing
}

// ComponentMatch records which component definition matched a node.
type ComponentMatch struct {
	Name   string
	Memory []string
	Tags   []string
}

// Conflict records a DOM node directly matched by more than one DISTINCT
// component name. Component matching is first-match-wins, so only Names[0]
// actually claims the node; the rest are silently dropped. Names are in
// first-seen (definition) order.
type Conflict struct {
	Node  *comps.ComponentNode
	Names []string
}
