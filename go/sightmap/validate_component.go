package sightmap

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// propertyNameRe is the schema pattern for a property name.
	propertyNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	// pathSegmentRe is a component-name segment inside a PATH reference. It is
	// intentionally identifier-ish so an old CSS sub-selector (brackets, spaces,
	// '#', ':', '=', …) is rejected rather than mistaken for a path.
	pathSegmentRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

// checkComponentProperties validates every component's properties[]: names must
// be unique within a component, and each extract must be a valid SEP-0017
// directive for a component. A deprecated string form that lowers cleanly draws
// extract-legacy-form; one that does not (a removed DOM mode such as inner_text,
// a bare CSS sub-selector, a mistyped attr/exists prefix) is an error.
func checkComponentProperties(c *Corpus) []ValidationError {
	var errs []ValidationError
	for _, comp := range c.AllComponents() {
		seen := make(map[string]bool, len(comp.Properties))
		for _, p := range comp.Properties {
			if p.Name != "" {
				if seen[p.Name] {
					errs = append(errs, ValidationError{
						Component: comp.Name,
						Code:      "component-property-duplicate",
						Severity:  SeverityError,
						Message:   fmt.Sprintf("component %q declares property %q more than once", comp.Name, p.Name),
					})
				}
				seen[p.Name] = true
			}
			code, msg := checkComponentExtract(p.Extract)
			if msg != "" {
				errs = append(errs, ValidationError{
					Component: comp.Name,
					Code:      code,
					Severity:  SeverityError,
					Message:   fmt.Sprintf("component %q property %q: %s", comp.Name, p.Name, msg),
				})
				continue
			}
			if p.Extract.IsLegacy() {
				errs = append(errs, legacyExtractWarning(comp.Name, p.Name, p.Extract))
			}
			if msg := privacyWithholds(comp.Privacy, p.Extract.From); msg != "" {
				errs = append(errs, ValidationError{
					Component: comp.Name,
					Code:      "extract-privacy-withheld",
					Severity:  SeverityWarning,
					Message: fmt.Sprintf("component %q property %q reads %s from a component whose privacy is %s, so no capture consumer will surface it; %s",
						comp.Name, p.Name, p.Extract.From, comp.Privacy, msg),
				})
			}
		}
	}
	return errs
}

// checkComponentExtract returns a diagnostic code and message when e is not a
// valid component directive, else ("", "").
func checkComponentExtract(e Extract) (code, msg string) {
	const invalid = "component-property-extract-invalid"
	if e.From == "" {
		if e.IsLegacy() {
			return invalid, fmt.Sprintf("unrecognized extract %q (expected an object with from: one of %s)", e.Legacy, strings.Join(ComponentExtractSources, ", "))
		}
		return invalid, "extract has no from"
	}
	if e.joinSet && e.From != FromComponent {
		return "extract-join-invalid", fmt.Sprintf("join is only valid with from: %s", FromComponent)
	}
	if e.joinSet && e.Join == "" {
		return "extract-join-invalid", "join must be a non-empty string"
	}
	switch e.From {
	case FromDOMText, FromDOMRawText:
		if e.Path != "" {
			return invalid, fmt.Sprintf("from: %s takes no path", e.From)
		}
	case FromDOMAttr:
		if e.Path == "" {
			return invalid, "from: dom.attr requires path: an attribute name"
		}
	case FromDOMState:
		if !containsString(StateNames, e.Path) {
			return invalid, fmt.Sprintf("from: dom.state requires path: one of %s", strings.Join(StateNames, ", "))
		}
	case FromComponent:
		dot := strings.LastIndex(e.Path, ".")
		if dot <= 0 || dot == len(e.Path)-1 {
			return invalid, fmt.Sprintf("from: component requires path: Component(.Component)*.property, got %q", e.Path)
		}
		if m := checkComponentPath(e.Path[:dot]); m != "" {
			return invalid, m
		}
		if prop := e.Path[dot+1:]; !propertyNameRe.MatchString(prop) {
			return invalid, fmt.Sprintf("invalid referenced property name %q in %q", prop, e.Path)
		}
	case FromComponentExists:
		if m := checkComponentPath(e.Path); m != "" {
			return invalid, m
		}
		if e.Pattern != "" {
			return invalid, "from: component.exists takes no pattern"
		}
	default:
		return invalid, fmt.Sprintf("from: %q is not a component source (expected one of %s)", e.From, strings.Join(ComponentExtractSources, ", "))
	}
	if e.Pattern != "" {
		if _, err := compilePattern(e.Pattern); err != nil {
			return invalid, fmt.Sprintf("pattern does not compile: %v", err)
		}
	}
	return "", ""
}

// privacyWithholds returns a fix hint when a component's own privacy guarantees
// that a capture consumer withholds a read from source (SEP-0009, per SEP-0017),
// else "". A warning, not an error: a consumer that captures no content ignores
// privacy and still resolves the value.
func privacyWithholds(privacy, source string) string {
	const hint = "change the component's privacy to extract it"
	switch source {
	case FromDOMText, FromDOMRawText, FromDOMAttr:
		if privacy == "block" || privacy == "mask" {
			return hint
		}
	case FromDOMState:
		if privacy == "block" {
			return hint
		}
	}
	return ""
}

// legacyExtractWarning reports a deprecated string form with its object
// equivalent (SEP-0017).
func legacyExtractWarning(owner, prop string, e Extract) ValidationError {
	return ValidationError{
		Component: owner,
		Code:      "extract-legacy-form",
		Severity:  SeverityWarning,
		Message:   fmt.Sprintf("property %q uses the deprecated string form %q; write extract: %s", prop, e.Legacy, e),
	}
}

func checkComponentPath(path string) string {
	if path == "" {
		return "empty component path"
	}
	for _, seg := range strings.Split(path, ".") {
		if !pathSegmentRe.MatchString(seg) {
			return fmt.Sprintf("invalid component name %q in path %q", seg, path)
		}
	}
	return ""
}
