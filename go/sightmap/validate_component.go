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

// checkComponentProperties validates every component's properties[] (SEP-0010):
// names must be unique within a component, and each extract directive must be one
// of the four forms text | attr=NAME | PATH.prop | exists:PATH. Anything else —
// the removed DOM modes (inner_text, text_only, inner_html), a bare CSS
// sub-selector, or a mistyped attr/exists prefix — is an error.
func checkComponentProperties(c *Corpus) []ValidationError {
	var errs []ValidationError
	for _, comp := range c.AllComponents() {
		seen := make(map[string]bool, len(comp.Properties))
		for _, p := range comp.Properties {
			// name mirrors the schema pattern (sightmap.schema.json). The JSON
			// Schema enforces it via ajv; the Go CLI never validates against the
			// schema, so without this a property named "5" or "Header Text" passes
			// `sightmap validate` while the reference checker rejects it. Matches
			// request-property-invalid-name / message-property-invalid-name. The
			// empty string also fails the pattern, which is the right call here: a
			// `name:` (null) or `name: ""` is already a schema violation, and the
			// duplicate-dedup guard below intentionally skips empty names rather
			// than treating two empty names as one duplicate.
			if !propertyNameRe.MatchString(p.Name) {
				errs = append(errs, ValidationError{
					Component: comp.Name,
					Code:      "component-property-invalid-name",
					Severity:  SeverityError,
					Message:   fmt.Sprintf("component %q declares a property named %q; names must match %s", comp.Name, p.Name, propertyNameRe),
				})
			}
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
			if msg := checkExtractMode(p.Extract); msg != "" {
				errs = append(errs, ValidationError{
					Component: comp.Name,
					Code:      "component-property-extract-invalid",
					Severity:  SeverityError,
					Message:   fmt.Sprintf("component %q property %q: %s", comp.Name, p.Name, msg),
				})
			}
		}
	}
	return errs
}

// checkExtractMode returns "" when extract is a valid SEP-0010 directive, else a
// human-readable reason it is rejected.
func checkExtractMode(extract string) string {
	switch {
	case extract == "text":
		return ""
	case strings.HasPrefix(extract, "attr="):
		if extract == "attr=" {
			return "attr= requires an attribute name"
		}
		return ""
	case strings.HasPrefix(extract, "exists:"):
		return checkComponentPath(strings.TrimPrefix(extract, "exists:"))
	default: // PATH.prop
		dot := strings.LastIndex(extract, ".")
		if dot <= 0 || dot == len(extract)-1 {
			return fmt.Sprintf("unrecognized extract mode %q (expected text, attr=NAME, PATH.prop, or exists:PATH)", extract)
		}
		if msg := checkComponentPath(extract[:dot]); msg != "" {
			return msg
		}
		if prop := extract[dot+1:]; !propertyNameRe.MatchString(prop) {
			return fmt.Sprintf("invalid referenced property name %q in %q", prop, extract)
		}
		return ""
	}
}

// checkComponentPath validates a dotted path of component names.
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
