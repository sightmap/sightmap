package sightmap

import (
	"fmt"
	"regexp"
	"slices"
)

// requestPropertyNamePattern mirrors $defs.requestProperty.properties.name in
// sightmap.schema.json, and componentProperty.name before it.
var requestPropertyNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// checkRequestProperties validates the shape of every declared request property
// (SEP-0005).
//
// These constraints live in the JSON Schema too, but only ajv enforces them
// there. The Go CLI never validates against sightmap.schema.json, so without
// this a corpus declaring both field and pattern (or neither, or an invalid
// name) passes `sightmap validate` and fails `npm run validate:conformance` —
// two conformance checkers disagreeing about the same file.
//
// Requests are read from Corpus.Requests and each View.Requests directly rather
// than through a whole-corpus accessor: those dedupe by first-seen name, which
// would skip a view-scoped request whose name matches a global one.
func checkRequestProperties(c *Corpus) []ValidationError {
	var errs []ValidationError
	seen := map[string]bool{}

	check := func(reqs []RequestDef) {
		for _, req := range reqs {
			for _, prop := range req.Properties {
				for _, e := range validateRequestProperty(req.Name, prop) {
					// Dedupe on code + request + property so a request declared
					// both globally and under a view reports once.
					key := e.Code + "\x00" + req.Name + "\x00" + prop.Name
					if seen[key] {
						continue
					}
					seen[key] = true
					errs = append(errs, e)
				}
			}
		}
	}

	check(c.Requests)
	for i := range c.Views {
		check(c.Views[i].Requests)
	}
	return errs
}

func validateRequestProperty(reqName string, prop RequestProperty) []ValidationError {
	var errs []ValidationError

	if !requestPropertyNamePattern.MatchString(prop.Name) {
		errs = append(errs, ValidationError{
			Component: reqName,
			Code:      "request-property-invalid-name",
			Severity:  SeverityError,
			Message: fmt.Sprintf("request %q declares a property named %q; names must match %s",
				reqName, prop.Name, requestPropertyNamePattern),
		})
	}

	switch {
	case prop.Field != "" && prop.Pattern != "":
		errs = append(errs, ValidationError{
			Component: reqName,
			Code:      "request-property-both-extractors",
			Severity:  SeverityError,
			Message: fmt.Sprintf("request %q property %q declares both field and pattern; exactly one is allowed",
				reqName, prop.Name),
		})
	case prop.Field == "" && prop.Pattern == "":
		errs = append(errs, ValidationError{
			Component: reqName,
			Code:      "request-property-no-extractor",
			Severity:  SeverityError,
			Message: fmt.Sprintf("request %q property %q declares neither field nor pattern; exactly one is required",
				reqName, prop.Name),
		})
	}

	// A declared property shadows the reserved identity name, which is legal and
	// is what SEP-0005's own motivating example does (`name: status` extracting
	// `rsp.body.status`). It is worth a warning because the HTTP identity then
	// becomes unreachable from a signal filter.
	if slices.Contains(ReservedRequestPropertyNames, prop.Name) {
		errs = append(errs, ValidationError{
			Component: reqName,
			Code:      "request-property-shadows-reserved",
			Severity:  SeverityWarning,
			Message: fmt.Sprintf("request %q declares a property named %q, shadowing the reserved request identity of the same name; a signal filtering on %q will see the extracted value, not the HTTP %s",
				reqName, prop.Name, prop.Name, prop.Name),
		})
	}

	return errs
}
