package sightmap_test

import (
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// findingCodes returns the diagnostic codes from a validation run, so tests can
// assert on codes rather than message text.
func findingCodes(errs []sightmap.ValidationError) []string {
	out := make([]string, 0, len(errs))
	for _, e := range errs {
		out = append(out, e.Code)
	}
	return out
}

func hasCode(errs []sightmap.ValidationError, code string) bool {
	for _, e := range errs {
		if e.Code == code {
			return true
		}
	}
	return false
}

func requestCorpus(props ...sightmap.RequestProperty) *sightmap.Corpus {
	return &sightmap.Corpus{
		Requests: []sightmap.RequestDef{{
			Name:       "CheckoutPayment",
			Route:      "/api/checkout/pay",
			Method:     "POST",
			Properties: props,
		}},
	}
}

func TestValidate_RequestPropertyBothExtractors(t *testing.T) {
	errs := sightmap.Validate(requestCorpus(sightmap.RequestProperty{
		Name:    "outcome",
		Field:   "rsp.body.status",
		Pattern: "declined",
	}))
	if !hasCode(errs, "request-property-both-extractors") {
		t.Fatalf("want request-property-both-extractors, got %v", findingCodes(errs))
	}
}

func TestValidate_RequestPropertyNoExtractor(t *testing.T) {
	errs := sightmap.Validate(requestCorpus(sightmap.RequestProperty{Name: "outcome"}))
	if !hasCode(errs, "request-property-no-extractor") {
		t.Fatalf("want request-property-no-extractor, got %v", findingCodes(errs))
	}
}

func TestValidate_RequestPropertyInvalidName(t *testing.T) {
	for _, name := range []string{"", "Outcome", "1st", "out-come", "out.come"} {
		errs := sightmap.Validate(requestCorpus(sightmap.RequestProperty{
			Name:  name,
			Field: "rsp.body.status",
		}))
		if !hasCode(errs, "request-property-invalid-name") {
			t.Errorf("name %q: want request-property-invalid-name, got %v", name, findingCodes(errs))
		}
	}
}

// A header path through `field` is the form SEP-0007's example depends on. It
// must validate cleanly; the contradicting "headers are always via pattern"
// rule was removed from SEP-0005.
func TestValidate_RequestPropertyHeaderFieldIsClean(t *testing.T) {
	errs := sightmap.Validate(requestCorpus(sightmap.RequestProperty{
		Name:      "rate_limit_remaining",
		Field:     "rsp.headers.X-RateLimit-Remaining",
		Transform: "number",
	}))
	if len(errs) != 0 {
		t.Fatalf("want no findings, got %v", findingCodes(errs))
	}
}

func TestValidate_RequestPropertyPatternOnlyIsClean(t *testing.T) {
	errs := sightmap.Validate(requestCorpus(sightmap.RequestProperty{
		Name:    "outcome",
		Pattern: `(?:declined|approved)`,
	}))
	if len(errs) != 0 {
		t.Fatalf("want no findings, got %v", findingCodes(errs))
	}
}

// Declaring a reserved identity name is legal (SEP-0005's motivating example
// does it) but shadows the HTTP identity, so it warns rather than erroring.
func TestValidate_RequestPropertyShadowsReserved(t *testing.T) {
	for _, name := range []string{"status", "method", "duration"} {
		errs := sightmap.Validate(requestCorpus(sightmap.RequestProperty{
			Name:  name,
			Field: "rsp.body." + name,
		}))
		var found *sightmap.ValidationError
		for i := range errs {
			if errs[i].Code == "request-property-shadows-reserved" {
				found = &errs[i]
			}
		}
		if found == nil {
			t.Fatalf("name %q: want request-property-shadows-reserved, got %v", name, findingCodes(errs))
		}
		if found.IsError() {
			t.Errorf("name %q: shadowing must be a warning, not an error", name)
		}
		if !strings.Contains(found.Message, name) {
			t.Errorf("name %q: message should name the property: %s", name, found.Message)
		}
	}
}

// View-scoped requests carry properties too, and are not reachable through the
// dedupe-by-name whole-corpus accessors.
func TestValidate_RequestPropertyViewScoped(t *testing.T) {
	c := &sightmap.Corpus{
		Views: []sightmap.View{{
			Name:  "Checkout",
			Route: "/checkout",
			Requests: []sightmap.RequestDef{{
				Name:  "ViewScoped",
				Route: "/api/x",
				Properties: []sightmap.RequestProperty{
					{Name: "outcome", Field: "rsp.body.a", Pattern: "b"},
				},
			}},
		}},
	}
	if !hasCode(sightmap.Validate(c), "request-property-both-extractors") {
		t.Fatalf("view-scoped request properties must be checked, got %v", findingCodes(sightmap.Validate(c)))
	}
}

// The same request name declared globally and under a view must report once,
// not once per copy.
func TestValidate_RequestPropertyDedupedAcrossScopes(t *testing.T) {
	bad := []sightmap.RequestProperty{{Name: "outcome"}}
	c := &sightmap.Corpus{
		Requests: []sightmap.RequestDef{{Name: "Shared", Route: "/api/x", Properties: bad}},
		Views: []sightmap.View{{
			Name:     "Checkout",
			Route:    "/checkout",
			Requests: []sightmap.RequestDef{{Name: "Shared", Route: "/api/x", Properties: bad}},
		}},
	}
	n := 0
	for _, e := range sightmap.Validate(c) {
		if e.Code == "request-property-no-extractor" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("want 1 request-property-no-extractor, got %d", n)
	}
}
