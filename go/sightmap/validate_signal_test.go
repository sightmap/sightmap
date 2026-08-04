package sightmap_test

import (
	"testing"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

func TestValidate_SignalRefUnresolved(t *testing.T) {
	errs := sightmap.Validate(&sightmap.Corpus{
		Signals: []sightmap.SignalDef{{Name: "s", Ref: "Nope"}},
	})
	if !hasCode(errs, "signal-ref-unresolved") {
		t.Fatalf("want signal-ref-unresolved, got %v", findingCodes(errs))
	}
}

func TestValidate_SignalRefAmbiguousAcrossKinds(t *testing.T) {
	errs := sightmap.Validate(&sightmap.Corpus{
		GlobalComponents: []match.ComponentDef{{Name: "Shared", Selectors: []string{".x"}}},
		Requests:         []sightmap.RequestDef{{Name: "Shared", Route: "/api/x"}},
		Signals:          []sightmap.SignalDef{{Name: "s", Ref: "Shared"}},
	})
	if !hasCode(errs, "signal-ref-ambiguous") {
		t.Fatalf("want signal-ref-ambiguous, got %v", findingCodes(errs))
	}
}

// A child component reused under several parents flattens to several same-name
// entries. That is intentional reuse, so it must not read as ambiguity.
func TestValidate_SignalRefReusedComponentNameIsNotAmbiguous(t *testing.T) {
	errs := sightmap.Validate(&sightmap.Corpus{
		GlobalComponents: []match.ComponentDef{
			{Name: "Row", Selectors: []string{".a .row"}},
			{Name: "Row", Selectors: []string{".b .row"}},
		},
		Signals: []sightmap.SignalDef{{Name: "s", Ref: "Row"}},
	})
	if hasCode(errs, "signal-ref-ambiguous") {
		t.Fatalf("reused child component names must not be ambiguous, got %v", findingCodes(errs))
	}
}

// Two messages sharing a name previously collapsed to one kind, so the ref
// resolved silently. The duplicate is now caught at its source.
func TestValidate_SignalRefDuplicateMessageIsCaught(t *testing.T) {
	errs := sightmap.Validate(&sightmap.Corpus{
		Messages: []sightmap.MessageDef{
			{Name: "Dup", Level: "ERROR", Message: "a"},
			{Name: "Dup", Level: "WARN", Message: "b"},
		},
		Signals: []sightmap.SignalDef{{Name: "s", Ref: "Dup"}},
	})
	if !hasCode(errs, "merge-collision-message") {
		t.Fatalf("want merge-collision-message so the ambiguous ref is not silent, got %v", findingCodes(errs))
	}
}

func requestSignalCorpus(filter map[string][]string) *sightmap.Corpus {
	return &sightmap.Corpus{
		Requests: []sightmap.RequestDef{{
			Name:  "CheckoutPayment",
			Route: "/api/checkout/pay",
			Properties: []sightmap.RequestProperty{
				{Name: "outcome", Source: "rsp.body", Field: "status"},
			},
		}},
		Signals: []sightmap.SignalDef{{Name: "s", Ref: "CheckoutPayment", Filter: filter}},
	}
}

func TestValidate_SignalFilterUnknownKey(t *testing.T) {
	errs := sightmap.Validate(requestSignalCorpus(map[string][]string{"typo_prop": {"x"}}))
	if !hasCode(errs, "signal-filter-unknown") {
		t.Fatalf("want signal-filter-unknown, got %v", findingCodes(errs))
	}
}

func TestValidate_SignalFilterDeclaredKeyIsClean(t *testing.T) {
	errs := sightmap.Validate(requestSignalCorpus(map[string][]string{"outcome": {"declined"}}))
	if len(errs) != 0 {
		t.Fatalf("want no findings, got %v", findingCodes(errs))
	}
}

// status/method/duration resolve on a request with no properties: declaration.
func TestValidate_SignalFilterReservedRequestNames(t *testing.T) {
	for _, key := range []string{"status", "method", "duration"} {
		errs := sightmap.Validate(&sightmap.Corpus{
			Requests: []sightmap.RequestDef{{Name: "R", Route: "/api/x"}},
			Signals:  []sightmap.SignalDef{{Name: "s", Ref: "R", Filter: map[string][]string{key: {"200"}}}},
		})
		if len(errs) != 0 {
			t.Errorf("reserved key %q should resolve with no declaration, got %v", key, findingCodes(errs))
		}
	}
}

// `value` comes from the accessibility tree, so it resolves on a component that
// declares no properties.
func TestValidate_SignalFilterComponentValueIsBuiltIn(t *testing.T) {
	errs := sightmap.Validate(&sightmap.Corpus{
		GlobalComponents: []match.ComponentDef{{Name: "Banner", Selectors: []string{".b"}}},
		Signals: []sightmap.SignalDef{{
			Name: "s", Ref: "Banner", Filter: map[string][]string{"value": {"declined"}},
		}},
	})
	if len(errs) != 0 {
		t.Fatalf("`value` should be built in, got %v", findingCodes(errs))
	}
}

// A message and a view have nothing to filter on, so a filter there is an error
// rather than a constraint that quietly never applies.
func TestValidate_SignalFilterOnMessageOrViewIsRejected(t *testing.T) {
	msg := &sightmap.Corpus{
		Messages: []sightmap.MessageDef{{Name: "M", Level: "ERROR", Message: "x"}},
		Signals: []sightmap.SignalDef{{
			Name: "s", Ref: "M", Filter: map[string][]string{"level": {"ERROR"}},
		}},
	}
	if !hasCode(sightmap.Validate(msg), "signal-filter-unknown") {
		t.Errorf("a filter on a message ref should be rejected, got %v", findingCodes(sightmap.Validate(msg)))
	}

	view := &sightmap.Corpus{
		Views: []sightmap.View{{Name: "V", Route: "/v"}},
		Signals: []sightmap.SignalDef{{
			Name: "s", Ref: "V", Filter: map[string][]string{"anything": {"x"}},
		}},
	}
	if !hasCode(sightmap.Validate(view), "signal-filter-unknown") {
		t.Errorf("a filter on a view ref should be rejected, got %v", findingCodes(sightmap.Validate(view)))
	}
}

func TestValidate_SignalRefMessageAndViewWithoutFilterAreClean(t *testing.T) {
	errs := sightmap.Validate(&sightmap.Corpus{
		Messages: []sightmap.MessageDef{{Name: "M", Level: "ERROR", Message: "x"}},
		Views:    []sightmap.View{{Name: "V", Route: "/v"}},
		Signals: []sightmap.SignalDef{
			{Name: "s1", Ref: "M"},
			{Name: "s2", Ref: "V"},
		},
	})
	if len(errs) != 0 {
		t.Fatalf("want no findings, got %v", findingCodes(errs))
	}
}

// A declared property shadowing a reserved name still resolves as a filter key;
// only the shadowing warning fires, from the request check.
func TestValidate_SignalFilterShadowedReservedResolves(t *testing.T) {
	c := &sightmap.Corpus{
		Requests: []sightmap.RequestDef{{
			Name:       "R",
			Route:      "/api/x",
			Properties: []sightmap.RequestProperty{{Name: "status", Source: "rsp.body", Field: "status"}},
		}},
		Signals: []sightmap.SignalDef{{
			Name: "s", Ref: "R", Filter: map[string][]string{"status": {"declined"}},
		}},
	}
	errs := sightmap.Validate(c)
	if hasCode(errs, "signal-filter-unknown") {
		t.Fatalf("a shadowed reserved name must still resolve, got %v", findingCodes(errs))
	}
	if !hasCode(errs, "request-property-shadows-reserved") {
		t.Fatalf("want the shadowing warning, got %v", findingCodes(errs))
	}
}

// A view-scoped request's properties must be visible to a signal too.
func TestValidate_SignalFilterViewScopedRequestProperties(t *testing.T) {
	errs := sightmap.Validate(&sightmap.Corpus{
		Views: []sightmap.View{{
			Name:  "Checkout",
			Route: "/checkout",
			Requests: []sightmap.RequestDef{{
				Name:       "ViewScoped",
				Route:      "/api/x",
				Properties: []sightmap.RequestProperty{{Name: "outcome", Source: "rsp.body", Field: "status"}},
			}},
		}},
		Signals: []sightmap.SignalDef{{
			Name: "s", Ref: "ViewScoped", Filter: map[string][]string{"outcome": {"declined"}},
		}},
	})
	if len(errs) != 0 {
		t.Fatalf("want no findings, got %v", findingCodes(errs))
	}
}

// SEP-0007's headline example was rejected by the schema this PR previously
// shipped, because `status: 200` is an unquoted integer. It must now load, and
// the integer must normalize to canonical text.
func TestLoadDir_SignalFilterScalarNormalization(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir+"/app.yaml", `
version: 1
requests:
  - name: CheckoutRetryPayment
    route: /api/checkout/pay/retry
    method: POST
    properties:
      - name: rate_limit_remaining
        source: rsp.headers
        field: X-RateLimit-Remaining
      - name: outcome
        source: rsp.body
        field: status
signals:
  - name: checkout.payment.throttled_silently
    tags: [defect]
    ref: CheckoutRetryPayment
    filter:
      status: 200
      rate_limit_remaining: "0"
      outcome: [queued, deferred]
`)
	c, err := sightmap.DirLoader(dir).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Signals) != 1 {
		t.Fatalf("want 1 signal, got %d", len(c.Signals))
	}
	f := c.Signals[0].Filter
	for key, want := range map[string][]string{
		"status":               {"200"},
		"rate_limit_remaining": {"0"},
		"outcome":              {"queued", "deferred"},
	} {
		got := f[key]
		if len(got) != len(want) {
			t.Errorf("filter[%q] = %v, want %v", key, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("filter[%q][%d] = %q, want %q", key, i, got[i], want[i])
			}
		}
	}
	if errs := sightmap.Validate(c); len(errs) != 0 {
		t.Errorf("SEP-0007's own example must validate cleanly, got %v", findingCodes(errs))
	}
}

// An integer is re-emitted from its parsed numeric value, not its lexeme. YAML
// accepts octal, hex, and underscore-separated integers, so comparing raw
// lexemes would make `0x1F` and `31` different values for the same number.
func TestLoadDir_SignalFilterIntegerCanonicalization(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir+"/app.yaml", `
version: 1
requests:
  - name: R
    route: /api/x
    properties:
      - name: octal
        source: rsp.body
        field: a
      - name: hex
        source: rsp.body
        field: b
      - name: underscored
        source: rsp.body
        field: c
      - name: flag
        source: rsp.body
        field: d
signals:
  - name: s
    ref: R
    filter:
      octal: 0200
      hex: 0x1F
      underscored: 1_000
      flag: true
`)
	c, err := sightmap.DirLoader(dir).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// 0200 is octal in YAML, so it is 128 — canonicalizing is what makes that
	// unambiguous instead of comparing the string "0200".
	want := map[string]string{"octal": "128", "hex": "31", "underscored": "1000", "flag": "true"}
	for key, exp := range want {
		got := c.Signals[0].Filter[key]
		if len(got) != 1 || got[0] != exp {
			t.Errorf("filter[%q] = %v, want [%q]", key, got, exp)
		}
	}
	if errs := sightmap.Validate(c); len(errs) != 0 {
		t.Errorf("want no findings, got %v", findingCodes(errs))
	}
}
