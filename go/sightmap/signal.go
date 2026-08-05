package sightmap

// SignalDef composes a named, tagged classification from an entity the corpus
// already defines (SEP-0007). A rule references that entity by name and
// optionally filters on its declared properties; it never redeclares a
// selector, route, or body pattern of its own, so a classification cannot drift
// away from the thing it is about.
//
// Signals are file-root only. There is no view-scoped form.
type SignalDef struct {
	// Name is the semantic identity of the generated classification, e.g.
	// "checkout.payment.declined".
	Name string `json:"name"`
	// Ref names an existing components:/requests:/messages:/views: entry. It
	// must resolve, and must not be ambiguous across entity kinds.
	Ref string `json:"ref"`
	// Tags are carried onto the generated classification (SEP-0004).
	Tags []string `json:"tags,omitempty"`
	// Filter constrains the referenced entity's declared properties and
	// already-structured identity fields. Each key holds one or more accepted
	// values: a single value is equality, several are membership. Keys are
	// ANDed. An absent filter fires on every match of Ref.
	//
	// Values are canonical text. An unquoted YAML integer or boolean is
	// accepted and normalized (200, true), so `status: 200` reads naturally
	// while still comparing as a string.
	Filter map[string][]string `json:"filter,omitempty"`
}

// FilterKeyKind describes how a signal's filter key resolved, for diagnostics.
type FilterKeyKind int

const (
	// FilterKeyUnknown means the key is neither a declared property nor a
	// reserved identity for the referenced entity's kind.
	FilterKeyUnknown FilterKeyKind = iota
	// FilterKeyDeclared means the key names a property the entity declares.
	FilterKeyDeclared
	// FilterKeyReserved means the key names an always-available field that
	// needs no declaration: status/method/duration on a request, value on a
	// component.
	FilterKeyReserved
)
