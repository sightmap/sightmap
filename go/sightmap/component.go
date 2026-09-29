package sightmap

// ComponentDef is a single flattened sightmap component definition.
// Hierarchical YAML selectors should be pre-flattened by the caller into
// compound descendant selectors before compiling into match queries.
type ComponentDef struct {
	Name      string   `json:"name"`
	Selectors []string `json:"selectors"`
	Source    string   `json:"source,omitempty"`
	Memory    []string `json:"memory,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	// Watch asks a capture consumer to report this component's visibility lifecycle even
	// when it is never interacted with. Applies to this component only, never its
	// children. See SEP-0015.
	Watch       bool                   `json:"watch,omitempty"`
	Properties  []ComponentPropertyDef `json:"properties,omitempty"`
	ParentChain []string               `json:"parentChain,omitempty"` // ancestor component names, root-first
	Stability   string                 `json:"stability,omitempty"`   // "" (default), "uncertain", or "unstable"
	// Origin identifies the global definition this def was instantiated from: its
	// address within that global, NUL-joined. The root global and every $ref
	// expansion of it are separate ComponentDefs with the same Origin, because
	// flattening deep-copies the global at each reference site. Empty for
	// components authored inline in a view.
	Origin string `json:"-"`
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
// component DEFINITION. Component matching is first-match-wins, so only the
// first actually claims the node; the rest are silently dropped. Entries are in
// first-seen (definition) order.
type Conflict struct {
	Node  *ComponentNode
	Names []string
	// Defs are the definitions that claimed the node, index-aligned with Names.
	//
	// Names alone cannot identify a claimant: a component name is unique only
	// WITHIN ITS PARENT, so a genuine conflict may legitimately list the same name
	// twice — two definitions at different points in the tree whose selectors both
	// reach one node. Consumers that need to tell claimants apart (to report them,
	// or to decide which is over-broad) must use Defs.
	Defs []*ComponentDef
}
