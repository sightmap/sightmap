package sightmap

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sel"
)

// ValidationError describes a structural problem in the corpus that would
// cause incorrect or undefined runtime behaviour.
type ValidationError struct {
	File      string // source YAML file path (empty for in-memory corpus)
	Component string // component name (empty if file-level error)
	Selector  string // offending selector string (empty if not selector-specific)
	Message   string
}

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

	return errs
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
