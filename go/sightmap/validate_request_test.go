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

// field and pattern compose now (anyOf), so declaring both is legal, not an
// error. This is the SEP-0005 reshape: `field` selects, `pattern` refines.
func TestValidate_RequestPropertyBothExtractorsCompose(t *testing.T) {
	errs := sightmap.Validate(requestCorpus(sightmap.RequestProperty{
		Name:    "outcome",
		Source:  "rsp.body",
		Field:   "status",
		Pattern: `(\w+)`,
	}))
	if len(errs) != 0 {
		t.Fatalf("field+pattern must compose cleanly, got %v", findingCodes(errs))
	}
}

func TestValidate_RequestPropertyNoExtractor(t *testing.T) {
	errs := sightmap.Validate(requestCorpus(sightmap.RequestProperty{Name: "outcome", Source: "rsp.body"}))
	if !hasCode(errs, "request-property-no-extractor") {
		t.Fatalf("want request-property-no-extractor, got %v", findingCodes(errs))
	}
}

func TestValidate_RequestPropertyInvalidName(t *testing.T) {
	for _, name := range []string{"", "Outcome", "1st", "out-come", "out.come"} {
		errs := sightmap.Validate(requestCorpus(sightmap.RequestProperty{
			Name:   name,
			Source: "rsp.body",
			Field:  "status",
		}))
		if !hasCode(errs, "request-property-invalid-name") {
			t.Errorf("name %q: want request-property-invalid-name, got %v", name, findingCodes(errs))
		}
	}
}

// source is required and closed to four values.
func TestValidate_RequestPropertySourceInvalid(t *testing.T) {
	for _, src := range []string{"", "rsp", "rsp.cookies", "body", "req.query"} {
		errs := sightmap.Validate(requestCorpus(sightmap.RequestProperty{
			Name:   "outcome",
			Source: src,
			Field:  "status",
		}))
		if !hasCode(errs, "request-property-source-invalid") {
			t.Errorf("source %q: want request-property-source-invalid, got %v", src, findingCodes(errs))
		}
	}
}

// A headers source has no structure below a header value, so field is required
// there — a bare pattern scan across the raw header block is the foot-gun the
// source/field split removes.
func TestValidate_RequestPropertyHeadersRequireField(t *testing.T) {
	for _, src := range []string{"req.headers", "rsp.headers"} {
		errs := sightmap.Validate(requestCorpus(sightmap.RequestProperty{
			Name:    "rate_limit_remaining",
			Source:  src,
			Pattern: `(\d+)`,
		}))
		if !hasCode(errs, "request-property-headers-require-field") {
			t.Errorf("source %q: want request-property-headers-require-field, got %v", src, findingCodes(errs))
		}
	}
}

// A header value refined by a regex is the motivating rate-limit case: source
// names the block, field names the header, pattern extracts the digits.
func TestValidate_RequestPropertyHeaderFieldIsClean(t *testing.T) {
	errs := sightmap.Validate(requestCorpus(sightmap.RequestProperty{
		Name:    "rate_limit_remaining",
		Source:  "rsp.headers",
		Field:   "X-RateLimit-Remaining",
		Pattern: `(\d+)`,
	}))
	if len(errs) != 0 {
		t.Fatalf("want no findings, got %v", findingCodes(errs))
	}
}

// A body source may carry pattern with no field (the form-encoded body case).
func TestValidate_RequestPropertyPatternOnlyIsClean(t *testing.T) {
	errs := sightmap.Validate(requestCorpus(sightmap.RequestProperty{
		Name:    "outcome",
		Source:  "rsp.body",
		Pattern: `(?:declined|approved)`,
	}))
	if len(errs) != 0 {
		t.Fatalf("want no findings, got %v", findingCodes(errs))
	}
}

// pattern is an RE2 regex, compiled at validation time (mirrors the message
// entity). An uncompilable pattern is an error, not a silently stored string.
func TestValidate_RequestPropertyPatternInvalid(t *testing.T) {
	for _, pat := range []string{"(", "[a-", `a{2,1}`} {
		errs := sightmap.Validate(requestCorpus(sightmap.RequestProperty{
			Name:    "outcome",
			Source:  "rsp.body",
			Pattern: pat,
		}))
		if !hasCode(errs, "request-property-pattern-invalid") {
			t.Errorf("pattern %q: want request-property-pattern-invalid, got %v", pat, findingCodes(errs))
		}
	}
}

// Declaring a reserved identity name is legal (SEP-0005's motivating example
// does it) but shadows the HTTP identity, so it warns rather than erroring.
func TestValidate_RequestPropertyShadowsReserved(t *testing.T) {
	for _, name := range []string{"status", "method", "duration"} {
		errs := sightmap.Validate(requestCorpus(sightmap.RequestProperty{
			Name:   name,
			Source: "rsp.body",
			Field:  name,
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
					{Name: "outcome", Source: "rsp.body"}, // no extractor
				},
			}},
		}},
	}
	if !hasCode(sightmap.Validate(c), "request-property-no-extractor") {
		t.Fatalf("view-scoped request properties must be checked, got %v", findingCodes(sightmap.Validate(c)))
	}
}

// The same request name declared globally and under a view must report once,
// not once per copy.
func TestValidate_RequestPropertyDedupedAcrossScopes(t *testing.T) {
	bad := []sightmap.RequestProperty{{Name: "outcome", Source: "rsp.body"}}
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
