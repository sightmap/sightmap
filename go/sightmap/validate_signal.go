package sightmap

import "fmt"

// checkSignals validates the corpus's signals — the SEP-0007 point-signal subset
// restricted to Component/View refs. Every signal needs a name and a ref, names
// must be unique, and the ref must resolve to exactly one component or one view.
// A ref matching nothing is signal-ref-unresolved; a ref matching both a
// component and a view is signal-ref-ambiguous. (Request/Message names are not in
// the Component/View lookup, so a ref to one reads as unresolved here — the
// intended restriction of this subset, not a general SEP-0007 rejection.)
func checkSignals(c *Corpus) []ValidationError {
	var errs []ValidationError
	seen := make(map[string]bool)
	for _, s := range c.Signals {
		if s.Name == "" {
			errs = append(errs, ValidationError{
				Code:     "missing-name",
				Severity: SeverityError,
				Message:  fmt.Sprintf("signal is missing a name (ref %q)", s.Ref),
			})
			continue
		}
		if seen[s.Name] {
			errs = append(errs, ValidationError{
				Component: s.Name,
				Code:      "merge-collision-signal",
				Severity:  SeverityError,
				Message:   fmt.Sprintf("signal name %q is defined more than once; signal names must be unique", s.Name),
			})
		}
		seen[s.Name] = true
		if s.Ref == "" {
			errs = append(errs, ValidationError{
				Component: s.Name,
				Code:      "missing-ref",
				Severity:  SeverityError,
				Message:   fmt.Sprintf("signal %q is missing a ref", s.Name),
			})
			continue
		}
		comp := c.componentByName(s.Ref)
		view := c.ViewByName(s.Ref)
		switch {
		case comp != nil && view != nil:
			errs = append(errs, ValidationError{
				Component: s.Name,
				Code:      "signal-ref-ambiguous",
				Severity:  SeverityError,
				Message:   fmt.Sprintf("signal %q ref %q is ambiguous: it matches both a component and a view", s.Name, s.Ref),
			})
		case comp == nil && view == nil:
			errs = append(errs, ValidationError{
				Component: s.Name,
				Code:      "signal-ref-unresolved",
				Severity:  SeverityError,
				Message:   fmt.Sprintf("signal %q ref %q does not resolve to a known component or view", s.Name, s.Ref),
			})
		}
	}
	return errs
}
