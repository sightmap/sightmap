package sightmap

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// routeParamPattern matches a ":name" route segment. SEP-0008 binds each one as a
// property carrying that segment's decoded value.
var routeParamPattern = regexp.MustCompile(`^:([a-z][a-z0-9_]*)$`)

// routeParams returns the :name segments of a route, in order. A "**" segment never
// binds: it spans a variable number of segments, so there is no single value to name.
func routeParams(route string) []string {
	var out []string
	for _, seg := range strings.Split(route, "/") {
		if m := routeParamPattern.FindStringSubmatch(seg); m != nil {
			out = append(out, m[1])
		}
	}
	return out
}

// checkViewProperties validates SEP-0008 URL properties on views, plus the route
// bindings themselves. The binding checks live here rather than beside the property
// checks because a route binds whether or not any properties[] entry is declared.
func checkViewProperties(c *Corpus) []ValidationError {
	var errs []ValidationError
	for _, v := range c.Views {
		errs = append(errs, checkRouteBindings(v.Name, v.Route, nil)...)
		seen := map[string]bool{}
		for _, p := range v.Properties {
			errs = append(errs, validateURLProperty(v.Name, p, v.Route)...)
			if seen[p.Name] {
				errs = append(errs, ValidationError{
					Component: v.Name,
					Code:      "url-property-duplicate-name",
					Severity:  SeverityError,
					Message:   fmt.Sprintf("view %q declares property %q more than once", v.Name, p.Name),
				})
			}
			seen[p.Name] = true
		}
	}
	return errs
}

// checkRouteBindings reports a :name that repeats within one route, and one that
// collides with a reserved name. reserved is nil for views, which have none.
func checkRouteBindings(owner, route string, reserved []string) []ValidationError {
	var errs []ValidationError
	seen := map[string]bool{}
	for _, name := range routeParams(route) {
		if seen[name] {
			errs = append(errs, ValidationError{
				Component: owner,
				Code:      "route-param-duplicate",
				Severity:  SeverityError,
				Message:   fmt.Sprintf("%q route binds %q more than once; each :name must be unique within a route", owner, name),
			})
		}
		seen[name] = true
		for _, r := range reserved {
			if name == r {
				errs = append(errs, ValidationError{
					Component: owner,
					Code:      "route-param-shadows-reserved",
					Severity:  SeverityError,
					Message:   fmt.Sprintf("%q route binds %q, which is a reserved identity name; rename the segment", owner, name),
				})
			}
		}
	}
	return errs
}

// validateURLProperty checks a view's SEP-0008 URL property: the name pattern and
// its extract.
func validateURLProperty(owner string, p URLPropertyDef, route string) []ValidationError {
	var errs []ValidationError
	if !requestPropertyNamePattern.MatchString(p.Name) {
		errs = append(errs, ValidationError{
			Component: owner,
			Code:      "url-property-invalid-name",
			Severity:  SeverityError,
			Message: fmt.Sprintf("%q declares a URL property named %q; names must match %s",
				owner, p.Name, requestPropertyNamePattern),
		})
	}
	return append(errs, validateURLExtract(owner, p.Name, p.Extract, route)...)
}

// validateURLExtract checks a url.query or url.path extract: a path is required,
// join is not supported, and url.path must name a segment the route binds.
func validateURLExtract(owner, prop string, e Extract, route string) []ValidationError {
	invalid := func(msg string) []ValidationError {
		return []ValidationError{{
			Component: owner,
			Code:      "url-property-extract-invalid",
			Severity:  SeverityError,
			Message:   fmt.Sprintf("%q property %q: %s", owner, prop, msg),
		}}
	}
	switch {
	case e.mixed:
		return []ValidationError{{Component: owner, Code: "extract-shape-mixed", Severity: SeverityError,
			Message: fmt.Sprintf("%q property %q declares extract alongside source/field/pattern; use extract alone", owner, prop)}}
	case e.From == "" && e.IsLegacy():
		return invalid(fmt.Sprintf("extract must be an object, got the string %q", e.Legacy))
	case !slices.Contains(URLExtractSources, e.From):
		return invalid(fmt.Sprintf("from: %q is not a URL source (expected one of %s)", e.From, strings.Join(URLExtractSources, ", ")))
	case e.Path == "":
		return invalid(fmt.Sprintf("from: %s requires path", e.From))
	case e.joinSet:
		return []ValidationError{{Component: owner, Code: "extract-join-invalid", Severity: SeverityError,
			Message: fmt.Sprintf("%q property %q: join is only valid with from: component", owner, prop)}}
	}
	if e.Pattern != "" {
		if _, err := compilePattern(e.Pattern); err != nil {
			return invalid(fmt.Sprintf("pattern does not compile: %v", err))
		}
	}
	// url.path is the one form that can dangle: it names a segment which must
	// actually exist in this entity's own route.
	if e.From == FromURLPath && !slices.Contains(routeParams(route), e.Path) {
		return []ValidationError{{
			Component: owner,
			Code:      "url-property-param-unresolved",
			Severity:  SeverityError,
			Message: fmt.Sprintf("%q property %q reads url.path %q, but the route %q binds no such segment",
				owner, prop, e.Path, route),
		}}
	}
	return nil
}
