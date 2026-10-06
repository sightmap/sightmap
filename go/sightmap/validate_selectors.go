package sightmap

import (
	"errors"
	"strings"
)

// Selector diagnostic codes (SEP-0018), beyond the profile parser's own.
const (
	// CodeSelectorUniversalSubject marks a selector whose subject compound has
	// no type, id, class or attribute outside :not(), so it matches nearly
	// every element.
	CodeSelectorUniversalSubject = "selector-universal-subject"
	// CodePrivacyUnmaskBroad marks an unmask whose subject is a type selector
	// alone (body, html, div:not(.x)).
	CodePrivacyUnmaskBroad = "privacy-unmask-broad"
)

// validateComponentSelectors checks each of a component's selectors, after
// flattening, against SEP-0018: every selector must be chain-evaluable and in
// the capture-baseline profile, and must not select nearly every element.
// A component that declares privacy or watch is a capture directive, so its
// findings are errors; on any other component they are warnings for the
// deprecation window. Selectors the general parser rejects are already
// reported and skipped here.
func validateComponentSelectors(comp ComponentDef) []ValidationError {
	capture := comp.Privacy != "" || comp.Watch
	severity := SeverityWarning
	if capture {
		severity = SeverityError
	}
	var errs []ValidationError
	for _, sel := range comp.Selectors {
		if _, err := ParseSightmapSelector(sel); err != nil {
			continue
		}
		ps, err := ParseProfileSelector(sel, ProfileCaptureBaseline)
		if err != nil {
			var pe *ProfileError
			if !errors.As(err, &pe) {
				continue
			}
			errs = append(errs, ValidationError{
				Component: comp.Name,
				Selector:  sel,
				Code:      pe.Code,
				Severity:  severity,
				Message:   pe.Msg + inheritedFrom(comp),
			})
			continue
		}
		switch subject := ps.Subject(); {
		case subject.Constraint == ConstraintNone:
			errs = append(errs, ValidationError{
				Component: comp.Name,
				Selector:  sel,
				Code:      CodeSelectorUniversalSubject,
				Severity:  severity,
				Message:   "the selector's last compound has no type, id, class or attribute, so it matches nearly every element" + inheritedFrom(comp),
			})
		case comp.Privacy == "unmask" && subject.Constraint == ConstraintType:
			errs = append(errs, ValidationError{
				Component: comp.Name,
				Selector:  sel,
				Code:      CodePrivacyUnmaskBroad,
				Severity:  SeverityWarning,
				Message:   "unmask on a type selector alone reopens every such element in the masked region; narrow it with an id, class or attribute",
			})
		}
	}
	return errs
}

// inheritedFrom notes, for a child component, that its selector includes its
// parents' selectors, where the problem may lie.
func inheritedFrom(comp ComponentDef) string {
	if len(comp.ParentChain) == 0 {
		return ""
	}
	return " (this selector includes its parents' selectors: " + strings.Join(comp.ParentChain, " > ") + ")"
}

// notChainEvaluableCode returns CodeSelectorNotChainEvaluable when a selector
// the general parser rejects is rejected because it is not chain-evaluable
// (a sibling combinator, a positional or state pseudo-class), so the finding
// says why it can never be supported; otherwise "".
func notChainEvaluableCode(sel string) string {
	var pe *ProfileError
	if _, err := ParseProfileSelector(sel, ProfileCaptureBaseline); errors.As(err, &pe) && pe.Code == CodeSelectorNotChainEvaluable {
		return pe.Code
	}
	return ""
}
