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

	// Authoring/tooling fields — kept out of the serialized wire form, matching
	// ViewDef's convention. Description is parsed from `description:` but was
	// previously dropped during flattening; SourceFile is the basename (sans
	// extension) of the .sightmap/*.yaml file this definition's `name:` was
	// declared in — a $ref'd component keeps its origin file, not the
	// referencing view's file.
	Description string `json:"-"`
	SourceFile  string `json:"-"`
}

// ComponentPropertyDef describes a value to extract from a matched DOM element.
type ComponentPropertyDef struct {
	Name      string `json:"name"`
	Extract   string `json:"extract"`   // see extract modes: text, inner_text, text_only, attr=NAME, exists:SEL, CSS selector
	Transform string `json:"transform"` // optional post-processing
}

// ComponentMatch records which component definition matched a node, and carries
// any live-extracted property values for it (Properties, in the definition's
// order; nil unless properties were extracted).
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
