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
	// properties it declares plus the reserved names available for its kind.
	props map[string]bool
	// acceptsFilter is false for kinds with nothing to filter on. A message's
	// own level/message already identify it fully, and a view has no extractable
	// property at all, so a filter on either is authoring confusion worth
	// reporting rather than a constraint that quietly never applies.
	acceptsFilter bool
}

func (e *entityEntry) kindList() []string {
	out := make([]string, 0, len(e.kinds))
	for k := range e.kinds {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func buildEntityIndex(c *Corpus) entityIndex {
	idx := entityIndex{}
	entry := func(name, kind string, acceptsFilter bool) *entityEntry {
		if name == "" {
			return nil
		}
		e := idx[name]
		if e == nil {
			e = &entityEntry{kinds: map[string]bool{}, props: map[string]bool{}}
			idx[name] = e
		}
		e.kinds[kind] = true
		if acceptsFilter {
			e.acceptsFilter = true
		}
		return e
	}

	for _, comp := range c.AllComponents() {
		e := entry(comp.Name, "component", true)
		if e == nil {
			continue
		}
		for _, p := range comp.Properties {
			e.props[p.Name] = true
		}
		// `value` resolves from the accessibility tree with no declaration.
		for _, r := range ReservedComponentPropertyNames {
			e.props[r] = true
		}
	}

	addRequests := func(reqs []RequestDef) {
		for _, req := range reqs {
			e := entry(req.Name, "request", true)
			if e == nil {
				continue
			}
			for _, p := range req.Properties {
				e.props[p.Name] = true
			}
			// status/method/duration resolve from the request's own HTTP
			// identity with no declaration. A declared property of the same
			// name shadows the identity; either way the key resolves, so this
			// check does not care which won.
			for _, r := range ReservedRequestPropertyNames {
				e.props[r] = true
			}
		}
	}
	addRequests(c.Requests)
	for i := range c.Views {
		addRequests(c.Views[i].Requests)
	}

	for _, msg := range c.Messages {
		entry(msg.Name, "message", false)
	}
	for _, v := range c.Views {
		entry(v.Name, "view", false)
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
	for _, sig := range c.Signals {
		if sig.Ref == "" {
			continue // required by the schema; not this check's job
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

	if !e.acceptsFilter {
		keys := sortedKeys(sig.Filter)
		return []ValidationError{{
			Component: sig.Name,
			Code:      "signal-filter-unknown",
			Severity:  SeverityError,
			Message: fmt.Sprintf("signal %q: ref %q is a %s, which has no filterable properties, but filter declares %s; reference it without a filter to fire on every match",
				sig.Name, sig.Ref, kind, strings.Join(quoteAll(keys), ", ")),
		}}
	}

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
