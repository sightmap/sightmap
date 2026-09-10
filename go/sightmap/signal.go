package sightmap

// SignalDef is a named, reference-based STATE predicate — the Component/View
// subset of SEP-0007 point signals. A signal names an existing corpus entity by
// `ref`; evaluated online it is the boolean "does that entity's state currently
// hold": a component being present, or a view's route being active.
//
// This is deliberately the dependency-free core of SEP-0007 (a Component ref
// leans only on SEP-0003 properties, a View ref on nothing). It excludes
// Request/Message refs — those need SEP-0005/0006 and stream evaluation — and all
// of the ba02 range-signals temporal/window machinery. A consumer turns a
// resolved signal into a live predicate: a component compquery (present?) or a
// route match (active?), which is why the two evaluate identically to the
// existing wait_for {query} / wait_for {route} forms.
type SignalDef struct {
	Name string   `json:"name"`
	Ref  string   `json:"ref"`
	Tags []string `json:"tags,omitempty"`
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
