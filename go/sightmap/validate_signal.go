package sightmap

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// entityIndex maps an entity name to the kinds that claim it and the property
// names a signal may filter on. Ref resolution and filter-key checking share one
// index because they answer the same question about the same name.
type entityIndex map[string]*entityEntry

type entityEntry struct {
	kinds map[string]bool
	// props holds every filter key that resolves for this entity: the
	// properties it declares, the :name bindings its route still produces, and
	// the reserved names available for its kind. A name reused by several
	// entities of one kind (a component declared in two views) is one entity
	// for a ref, so its props are the union across them.
	props map[string]bool
}

func (e *entityEntry) kindList() []string {
	out := make([]string, 0, len(e.kinds))
	for k := range e.kinds {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// producedBindings returns the :name bindings a route still produces after any
// `from: url.path` entry renames its segment (SEP-0008).
func producedBindings(route string, props []namedExtract) []string {
	renamed := map[string]bool{}
	for _, p := range props {
		if p.extract.From == FromURLPath {
			renamed[p.extract.Path] = true
		}
	}
	var out []string
	for _, name := range routeParams(route) {
		if !renamed[name] {
			out = append(out, name)
		}
	}
	return out
}

func buildEntityIndex(c *Corpus) entityIndex {
	idx := entityIndex{}
	entry := func(name, kind string, props ...string) {
		if name == "" {
			return
		}
		e := idx[name]
		if e == nil {
			e = &entityEntry{kinds: map[string]bool{}, props: map[string]bool{}}
			idx[name] = e
		}
		e.kinds[kind] = true
		for _, p := range props {
			e.props[p] = true
		}
	}

	// Every declaration rather than AllComponents, which keeps only the first
	// of each name and would hide a property declared on a later one.
	addComponents := func(comps []ComponentDef) {
		for _, comp := range comps {
			names := slices.Clone(ReservedComponentPropertyNames)
			for _, p := range comp.Properties {
				names = append(names, p.Name)
			}
			entry(comp.Name, "component", names...)
		}
	}
	addComponents(c.GlobalComponents)
	addComponents(c.Definitions)
	for i := range c.Views {
		addComponents(c.Views[i].Components)
	}

	addRequests := func(reqs []RequestDef) {
		for _, req := range reqs {
			// status/method/duration resolve from the request's own HTTP
			// identity with no declaration. A declared property of the same
			// name shadows the identity; either way the key resolves.
			names := slices.Clone(ReservedRequestPropertyNames)
			urlProps := make([]namedExtract, len(req.Properties))
			for i, p := range req.Properties {
				names = append(names, p.Name)
				urlProps[i] = namedExtract{p.Name, p.Extract}
			}
			names = append(names, producedBindings(req.Route, urlProps)...)
			entry(req.Name, "request", names...)
		}
	}
	addRequests(c.Requests)
	for i := range c.Views {
		addRequests(c.Views[i].Requests)
	}

	for _, msg := range c.Messages {
		var names []string
		for _, p := range msg.Properties {
			names = append(names, p.Name)
		}
		entry(msg.Name, "message", names...)
	}

	for _, v := range c.Views {
		var names []string
		urlProps := make([]namedExtract, len(v.Properties))
		for i, p := range v.Properties {
			names = append(names, p.Name)
			urlProps[i] = namedExtract{p.Name, p.Extract}
		}
		names = append(names, producedBindings(v.Route, urlProps)...)
		entry(v.Name, "view", names...)
	}
	return idx
}

// checkSignals validates every signals: rule (SEP-0007): that its ref resolves
// to exactly one entity kind, and that each filter key names something that
// entity can actually be filtered on.
//
// The filter-key check is what makes SEP-0007's central claim true. The proposal
// rests on a signal composing what the corpus already defines so the two cannot
// drift apart; without validating the keys, a filter naming a property that was
// renamed or never existed passes silently and the signal never fires.
func checkSignals(c *Corpus) []ValidationError {
	if len(c.Signals) == 0 {
		return nil
	}
	idx := buildEntityIndex(c)

	var out []ValidationError
	seen := make(map[string]bool, len(c.Signals))
	for _, sig := range c.Signals {
		// Shape checks first. The schema requires name and ref, but Validate also
		// runs against corpora built in memory, which never passed through it.
		if sig.Name == "" {
			out = append(out, ValidationError{
				Code:     "missing-name",
				Severity: SeverityError,
				Message:  fmt.Sprintf("signal is missing a name (ref %q)", sig.Ref),
			})
			continue
		}
		if seen[sig.Name] {
			out = append(out, ValidationError{
				Component: sig.Name,
				Code:      "merge-collision-signal",
				Severity:  SeverityError,
				Message:   fmt.Sprintf("signal name %q is defined more than once; signal names must be unique", sig.Name),
			})
		}
		seen[sig.Name] = true
		if sig.Ref == "" {
			out = append(out, ValidationError{
				Component: sig.Name,
				Code:      "missing-ref",
				Severity:  SeverityError,
				Message:   fmt.Sprintf("signal %q is missing a ref", sig.Name),
			})
			continue
		}
		e := idx[sig.Ref]
		if e == nil {
			out = append(out, ValidationError{
				Component: sig.Name,
				Code:      "signal-ref-unresolved",
				Severity:  SeverityError,
				Message:   fmt.Sprintf("signal %q: ref %q does not resolve to any component, request, message, or view", sig.Name, sig.Ref),
			})
			continue
		}
		if len(e.kinds) > 1 {
			out = append(out, ValidationError{
				Component: sig.Name,
				Code:      "signal-ref-ambiguous",
				Severity:  SeverityError,
				Message: fmt.Sprintf("signal %q: ref %q is ambiguous across entity kinds (%s)",
					sig.Name, sig.Ref, strings.Join(e.kindList(), ", ")),
			})
			continue
		}
		out = append(out, checkSignalFilter(sig, e)...)
	}
	return out
}

func checkSignalFilter(sig SignalDef, e *entityEntry) []ValidationError {
	if len(sig.Filter) == 0 {
		return nil
	}
	kind := e.kindList()[0]

	available := sortedKeys(e.props)
	var out []ValidationError
	for _, key := range sortedKeys(sig.Filter) {
		if e.props[key] {
			continue
		}
		msg := fmt.Sprintf("signal %q: filter key %q is not a property of %s %q", sig.Name, key, kind, sig.Ref)
		if len(available) > 0 {
			msg += fmt.Sprintf("; available: %s", strings.Join(quoteAll(available), ", "))
		} else {
			msg += "; it declares no properties"
		}
		out = append(out, ValidationError{
			Component: sig.Name,
			Code:      "signal-filter-unknown",
			Severity:  SeverityError,
			Message:   msg,
		})
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = fmt.Sprintf("%q", s)
	}
	return out
}
