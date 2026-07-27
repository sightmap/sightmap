package sightmap

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sel"
)

// Severity classifies a validation finding. Errors fail validation (nonzero
// exit); warnings are advisory and do not (see runValidate).
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// ValidationError describes a structural problem in the corpus. Despite the
// name it carries a Severity: an error is a hard problem that fails validation,
// a warning is an advisory finding (e.g. a corpus conflict resolved by a
// fallback rule) that the author should see but that does not fail the corpus.
type ValidationError struct {
	File      string   // source YAML file path (empty for in-memory corpus)
	Component string   // component name (empty if file-level error)
	Selector  string   // offending selector string (empty if not selector-specific)
	Message   string   //
	Code      string   // stable diagnostic code, e.g. "ref-circular", "merge-collision-view"
	Severity  Severity // empty is treated as SeverityError
}

// IsError reports whether this finding fails validation. The zero-value
// Severity ("") is treated as an error, so existing checks stay errors.
func (e ValidationError) IsError() bool { return e.Severity != SeverityWarning }

func (e ValidationError) Error() string {
	parts := []string{}
	if e.File != "" {
		parts = append(parts, e.File)
	}
	if e.Component != "" {
		parts = append(parts, e.Component)
	}
	if e.Selector != "" {
		parts = append(parts, fmt.Sprintf("selector %q", e.Selector))
	}
	parts = append(parts, e.Message)
	return strings.Join(parts, ": ")
}

// Validate checks the corpus for structural errors.
// Returns one ValidationError per problem; empty slice means valid.
func Validate(c *Corpus) []ValidationError {
	var errs []ValidationError

	// Structural problems detected at load time (e.g. circular $ref chains that
	// are expanded away before this point).
	errs = append(errs, c.loadDiagnostics...)

	// Check global components.
	globalNames := make(map[string]string)
	for _, comp := range c.GlobalComponents {
		errs = append(errs, validateComponent(comp, globalNames)...)
	}

	// Check views.
	for _, view := range c.Views {
		if view.Route == "" {
			errs = append(errs, ValidationError{
				Component: view.Name,
				Message:   "view has empty route",
			})
		}
		// Validate view stability
		if view.Stability != "" && view.Stability != "stub" && view.Stability != "deferred" {
			errs = append(errs, ValidationError{
				Component: view.Name,
				Message:   fmt.Sprintf(`invalid stability value %q (must be empty, "stub", or "deferred")`, view.Stability),
			})
		}
		// Validate view access
		switch view.Access.Status {
		case "", "open", "blocked", "needs-data":
			// valid
		default:
			errs = append(errs, ValidationError{
				Component: view.Name,
				Message:   fmt.Sprintf(`invalid access.status %q (must be empty, "open", "blocked", or "needs-data")`, view.Access.Status),
			})
		}
		if (view.Access.Status == "blocked" || view.Access.Status == "needs-data") && view.Access.Reason == "" {
			errs = append(errs, ValidationError{
				Component: view.Name,
				Message:   fmt.Sprintf(`access.status %q requires a non-empty reason`, view.Access.Status),
			})
		}
		viewNames := make(map[string]string)
		for _, comp := range view.Components {
			errs = append(errs, validateComponent(comp, viewNames)...)
		}
	}

	// Corpus-conflict warnings: definitions that collide on identity and are
	// resolved silently by a fallback rule. These are advisory (warnings), not
	// errors — the corpus still loads, but the author should know one definition
	// is shadowing another.
	// Duplicate GLOBAL component names are detected at load time (loader.go),
	// not here: flattening a child component reused under several parents
	// produces multiple same-name entries in GlobalComponents, so the collision
	// is only distinguishable from the raw top-level definitions.
	errs = append(errs, checkViewNameCollisions(c.Views)...)
	errs = append(errs, checkRouteCollisions(c.Views)...)

	return errs
}

// checkViewNameCollisions warns when two or more views share a name. View names
// should be unique across the corpus; when they collide, lookups by name and the
// snapshot header become ambiguous. (Merging same-named views is a future SEP,
// not v0 behaviour.)
func checkViewNameCollisions(views []View) []ValidationError {
	byName := map[string][]View{}
	var order []string
	for _, v := range views {
		if v.Name == "" {
			continue
		}
		if _, seen := byName[v.Name]; !seen {
			order = append(order, v.Name)
		}
		byName[v.Name] = append(byName[v.Name], v)
	}
	var out []ValidationError
	for _, name := range order {
		vs := byName[name]
		if len(vs) < 2 {
			continue
		}
		out = append(out, ValidationError{
			Component: name,
			Code:      "merge-collision-view",
			Severity:  SeverityWarning,
			Message: fmt.Sprintf("view name %q is defined %d times (%s); view names should be unique",
				name, len(vs), viewLocList(vs)),
		})
	}
	return out
}

// checkRouteCollisions warns when two or more views share the same (normalized)
// route. Only one view can win for a given URL — resolution falls back to
// declaration order — so the later views' components silently stop applying.
func checkRouteCollisions(views []View) []ValidationError {
	byRoute := map[string][]View{}
	var order []string
	for _, v := range views {
		if v.Route == "" {
			continue
		}
		r := normalizeRoutePath(v.Route)
		if _, seen := byRoute[r]; !seen {
			order = append(order, r)
		}
		byRoute[r] = append(byRoute[r], v)
	}
	var out []ValidationError
	for _, r := range order {
		vs := byRoute[r]
		if len(vs) < 2 {
			continue
		}
		names := make([]string, 0, len(vs))
		for _, v := range vs {
			names = append(names, v.Name)
		}
		out = append(out, ValidationError{
			Code:     "route-conflict",
			Severity: SeverityWarning,
			Message: fmt.Sprintf("route %q is claimed by %d views (%s); only the first (%s) applies to that URL",
				r, len(vs), strings.Join(names, ", "), vs[0].Name),
		})
	}
	return out
}

// viewLocList formats a set of views as "file:route" entries for a collision
// message.
func viewLocList(vs []View) string {
	parts := make([]string, 0, len(vs))
	for _, v := range vs {
		f := v.SourceFile
		if f == "" {
			f = "?"
		}
		parts = append(parts, fmt.Sprintf("%s:%s", f, v.Route))
	}
	return strings.Join(parts, ", ")
}

// validateComponent validates a single component against the shared seen map.
// seen maps component name → sorted-selector fingerprint so that the same
// name is only flagged as a duplicate when the selector set is also identical.
// Same name + different selectors = intentional child-component reuse = OK.
func validateComponent(comp match.SightmapComponent, seen map[string]string) []ValidationError {
	var errs []ValidationError

	// empty-name: no further named checks are possible.
	if comp.Name == "" {
		return append(errs, ValidationError{Message: "component has empty name"})
	}

	// invalid-stability
	if comp.Stability != "" && comp.Stability != "uncertain" && comp.Stability != "unstable" {
		errs = append(errs, ValidationError{
			Component: comp.Name,
			Message:   fmt.Sprintf(`invalid stability value %q (must be empty, "uncertain", or "unstable")`, comp.Stability),
		})
	}

	// no-selector
	if len(comp.Selectors) == 0 {
		errs = append(errs, ValidationError{
			Component: comp.Name,
			Message:   "component has no selectors",
		})
	}

	// selector-parse
	for _, selStr := range comp.Selectors {
		if _, err := sel.ParseSightmapSelector(selStr); err != nil {
			errs = append(errs, ValidationError{
				Component: comp.Name,
				Selector:  selStr,
				Message:   "selector parse error: " + err.Error(),
			})
		}
	}

	// duplicate-name+selector within scope: same name AND same selector set is
	// a true duplicate. Same name with different selectors is intentional reuse
	// (e.g. CarouselScrollButton as a child of multiple carousel components).
	fingerprint := selectorFingerprint(comp.Selectors)
	if prev, exists := seen[comp.Name]; exists && prev == fingerprint {
		errs = append(errs, ValidationError{
			Component: comp.Name,
			Message:   "duplicate component name and selector",
		})
	}
	seen[comp.Name] = fingerprint

	return errs
}

// selectorFingerprint returns a canonical string for a set of selectors,
// used to distinguish true duplicates from intentional same-name reuse.
func selectorFingerprint(selectors []string) string {
	if len(selectors) == 0 {
		return ""
	}
	sorted := make([]string, len(selectors))
	copy(sorted, selectors)
	sort.Strings(sorted)
	return strings.Join(sorted, "|")
}
