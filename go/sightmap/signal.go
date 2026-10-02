package sightmap

import "sort"

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

// SignalRefKind is which kind of corpus entity a signal's ref resolves to.
type SignalRefKind int

const (
	// SignalRefUnresolved means the ref matched no component or view, or matched
	// both (ambiguous). Validate reports these as signal-ref-unresolved /
	// signal-ref-ambiguous; a consumer should treat an unresolved target as
	// "do not evaluate".
	SignalRefUnresolved SignalRefKind = iota
	// SignalRefComponent means the ref names a component (present? predicate).
	SignalRefComponent
	// SignalRefView means the ref names a view (route-active? predicate).
	SignalRefView
)

// SignalTarget is the resolved subject of a signal. For SignalRefComponent,
// Component is set; for SignalRefView, View is set; for SignalRefUnresolved, both
// are nil.
type SignalTarget struct {
	Kind      SignalRefKind
	Component *ComponentDef
	View      *ViewDef
}

// SignalByName returns a pointer to the first signal with the given name, or nil.
func (c *Corpus) SignalByName(name string) *SignalDef {
	for i := range c.Signals {
		if c.Signals[i].Name == name {
			return &c.Signals[i]
		}
	}
	return nil
}

// componentByName finds a component by name across the global list and every
// view's components (first-seen wins, matching AllComponents' order), or nil.
func (c *Corpus) componentByName(name string) *ComponentDef {
	for i := range c.GlobalComponents {
		if c.GlobalComponents[i].Name == name {
			return &c.GlobalComponents[i]
		}
	}
	for vi := range c.Views {
		for ci := range c.Views[vi].Components {
			if c.Views[vi].Components[ci].Name == name {
				return &c.Views[vi].Components[ci]
			}
		}
	}
	return nil
}

// ResolveSignalRef resolves a signal's `ref` name to its corpus subject. A ref
// that matches both a component and a view is ambiguous and reported here as
// Unresolved (Validate flags it as a hard error); post-validation a consumer can
// trust any non-Unresolved result.
func (c *Corpus) ResolveSignalRef(ref string) SignalTarget {
	comp := c.componentByName(ref)
	view := c.ViewByName(ref)
	switch {
	case comp != nil && view != nil:
		return SignalTarget{Kind: SignalRefUnresolved}
	case comp != nil:
		return SignalTarget{Kind: SignalRefComponent, Component: comp}
	case view != nil:
		return SignalTarget{Kind: SignalRefView, View: view}
	default:
		return SignalTarget{Kind: SignalRefUnresolved}
	}
}

// ResolveSignal looks up a signal by name and resolves its ref (SignalByName +
// ResolveSignalRef). An unknown name yields an Unresolved target.
func (c *Corpus) ResolveSignal(name string) SignalTarget {
	s := c.SignalByName(name)
	if s == nil {
		return SignalTarget{Kind: SignalRefUnresolved}
	}
	return c.ResolveSignalRef(s.Ref)
}

// TagsForSignal returns a signal's effective tags: its own, unioned with the
// resolved tags of the entity its ref names, deduplicated and lexicographically
// sorted. Nil for an unknown signal, or when neither side carries any.
//
// A signal is a named classification ABOUT an entity, so the entity's own
// classification applies to it (SEP-0016). Requiring an author to restate a
// referenced request's tags on every signal would reintroduce, one level up, the
// shadowing problem SEP-0004 exists to avoid. Resolution is transitive only
// through ref, and ref resolves to exactly one entity, so there is no chain to
// walk and no cycle to detect.
func (c *Corpus) TagsForSignal(name string) []string {
	s := c.SignalByName(name)
	if s == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(tags []string) {
		for _, t := range tags {
			if t == "" || seen[t] {
				continue
			}
			seen[t] = true
			out = append(out, t)
		}
	}
	add(s.Tags)
	switch target := c.ResolveSignalRef(s.Ref); target.Kind {
	case SignalRefComponent:
		add(target.Component.Tags)
	case SignalRefView:
		add(target.View.Tags)
	}
	sort.Strings(out)
	return out
}

// ReservedComponentPropertyNames are component property names a signal filter
// may name without the component declaring them. `value` is always available
// from the accessibility tree, so a signal filtering on it is valid even when
// the component declares no properties of its own. (The request-side analogue,
// ReservedRequestPropertyNames, lives in request.go.)
var ReservedComponentPropertyNames = []string{"value"}

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
