package sightmap

// ComponentDef is a single flattened sightmap component definition.
// Hierarchical YAML selectors should be pre-flattened by the caller into
// compound descendant selectors before compiling into match queries.
type ComponentDef struct {
	Name        string                 `json:"name"`
	Selectors   []string               `json:"selectors"`
	Source      string                 `json:"source,omitempty"`
	Memory      []string               `json:"memory,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	Properties  []ComponentPropertyDef `json:"properties,omitempty"`
	ParentChain []string               `json:"parentChain,omitempty"` // ancestor component names, root-first
	Stability   string                 `json:"stability,omitempty"`   // "" (default), "uncertain", or "unstable"
}

// ComponentPropertyDef describes a value extracted from a matched component,
// resolved over the component tree (SEP-0010).
type ComponentPropertyDef struct {
	Name    string `json:"name"`
	Extract string `json:"extract"` // SEP-0010: text | attr=NAME | PATH.prop | exists:PATH
}

// ComponentMatch records which component definition matched a node, and carries
// its resolved property values (Properties, in the definition's order; nil
// unless the component declares properties that resolved).
type ComponentMatch struct {
	Name       string
	Memory     []string
	Tags       []string
	Properties []PropertyValue
}

// Property returns the extracted value named name, if present.
func (m *ComponentMatch) Property(name string) (PropertyValue, bool) {
	for _, p := range m.Properties {
		if p.Name == name {
			return p, true
		}
	}
	return PropertyValue{}, false
}

// Conflict records a DOM node directly matched by more than one DISTINCT
// component name. Component matching is first-match-wins, so only Names[0]
// actually claims the node; the rest are silently dropped. Names are in
// first-seen (definition) order.
type Conflict struct {
	Node  *ComponentNode
	Names []string
}
