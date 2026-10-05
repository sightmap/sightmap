package sightmap_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

func viewCorpus(v sightmap.ViewDef) *sightmap.Corpus {
	return &sightmap.Corpus{Views: []sightmap.ViewDef{v}}
}

// A bound :name needs no properties[] entry, and an entry naming a query parameter
// or renaming a bound segment is clean. This is the SEP-0008 happy path on a view.
func TestValidate_ViewURLPropertiesClean(t *testing.T) {
	errs := sightmap.Validate(viewCorpus(sightmap.ViewDef{
		Name: "OrgSettings", Route: "/org/:org_id/settings",
		Properties: []sightmap.URLPropertyDef{
			{Name: "tenant", Extract: sightmap.Extract{From: sightmap.FromURLPath, Path: "org_id"}},
			{Name: "variant", Extract: sightmap.Extract{From: sightmap.FromURLQuery, Path: "variant"}},
		},
	}))
	if len(errs) != 0 {
		t.Errorf("want no diagnostics, got %v", findingCodes(errs))
	}
}

// param: is the one extract form that can dangle: it names a segment which must
// actually be bound by this view's own route. query: cannot, since any key is legal.
func TestValidate_ViewParamUnresolved(t *testing.T) {
	errs := sightmap.Validate(viewCorpus(sightmap.ViewDef{
		Name: "ProductDetail", Route: "/shop/p/:product_id",
		Properties: []sightmap.URLPropertyDef{{Name: "sku", Extract: sightmap.Extract{From: sightmap.FromURLPath, Path: "nope"}}},
	}))
	if !hasCode(errs, "url-property-param-unresolved") {
		t.Errorf("want url-property-param-unresolved, got %v", findingCodes(errs))
	}
}

func TestValidate_ViewExtractGrammar(t *testing.T) {
	for _, bad := range []sightmap.Extract{
		{},
		{From: "url.host", Path: "x"},
		{From: "url.fragment", Path: "x"},
		{From: sightmap.FromURLQuery},
		{From: sightmap.FromDOMText},
		{From: sightmap.FromRspBody, Path: "status"},
		{From: sightmap.FromURLQuery, Path: "v", Pattern: "("},
		{Legacy: "query:variant"},
	} {
		errs := sightmap.Validate(viewCorpus(sightmap.ViewDef{
			Name: "V", Route: "/a", Properties: []sightmap.URLPropertyDef{{Name: "x", Extract: bad}},
		}))
		if !hasCode(errs, "url-property-extract-invalid") {
			t.Errorf("extract %+v: want url-property-extract-invalid, got %v", bad, findingCodes(errs))
		}
	}
}

// A :name repeating in one route has no single value to bind, so it is refused
// rather than resolved by a precedence rule nobody would guess.
func TestValidate_RouteParamDuplicate(t *testing.T) {
	errs := sightmap.Validate(viewCorpus(sightmap.ViewDef{Name: "V", Route: "/a/:id/b/:id"}))
	if !hasCode(errs, "route-param-duplicate") {
		t.Errorf("want route-param-duplicate, got %v", findingCodes(errs))
	}
}

// ** spans a variable number of segments, so it never binds and never collides.
func TestValidate_DoubleStarDoesNotBind(t *testing.T) {
	errs := sightmap.Validate(viewCorpus(sightmap.ViewDef{Name: "V", Route: "/a/**"}))
	if len(errs) != 0 {
		t.Errorf("** should bind nothing, got %v", findingCodes(errs))
	}
}

// On a request, a bound name colliding with a reserved identity name is refused
// rather than silently shadowing it. Views have no reserved names, so the same
// route on a view is clean.
func TestValidate_RequestRouteParamShadowsReserved(t *testing.T) {
	errs := sightmap.Validate(&sightmap.Corpus{
		Requests: []sightmap.RequestDef{{Name: "R", Route: "/api/:status"}},
	})
	if !hasCode(errs, "route-param-shadows-reserved") {
		t.Errorf("want route-param-shadows-reserved, got %v", findingCodes(errs))
	}
	if clean := sightmap.Validate(viewCorpus(sightmap.ViewDef{Name: "V", Route: "/a/:status"})); len(clean) != 0 {
		t.Errorf("a view has no reserved names; want clean, got %v", findingCodes(clean))
	}
}

// A request property reads with the extract object or the deprecated keys, never both.
func TestValidate_RequestPropertyShapeMixed(t *testing.T) {
	dir := t.TempDir()
	yaml := "version: 1\nrequests:\n  - name: R\n    route: /api/x\n    properties:\n" +
		"      - name: v\n        extract: { from: url.query, path: v }\n        source: rsp.body\n"
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if errs := sightmap.Validate(c); !hasCode(errs, "extract-shape-mixed") {
		t.Errorf("want extract-shape-mixed, got %v", findingCodes(errs))
	}
}

// Both shapes side by side on one request is the documented case and must be clean.
func TestValidate_RequestBothPropertyShapes(t *testing.T) {
	errs := sightmap.Validate(&sightmap.Corpus{Requests: []sightmap.RequestDef{{
		Name: "GetOrder", Route: "/api/orders/:order_id",
		Properties: []sightmap.RequestPropertyDef{
			{Name: "variant", Extract: sightmap.Extract{From: sightmap.FromURLQuery, Path: "variant"}},
			{Name: "outcome", Extract: sightmap.Extract{From: sightmap.FromRspBody, Path: "status"}},
		},
	}}})
	if len(errs) != 0 {
		t.Errorf("want no diagnostics, got %v", findingCodes(errs))
	}
}

// A declared property sharing a name with a binding the route still produces is
// a conflict; renaming the binding with url.path, or restating it, is not.
func TestValidate_RouteBindingConflict(t *testing.T) {
	q := func(key string) sightmap.Extract { return sightmap.Extract{From: sightmap.FromURLQuery, Path: key} }
	p := func(seg string) sightmap.Extract { return sightmap.Extract{From: sightmap.FromURLPath, Path: seg} }
	for _, tc := range []struct {
		name  string
		props []sightmap.URLPropertyDef
		want  bool
	}{
		{"query under a bound name", []sightmap.URLPropertyDef{{Name: "org_id", Extract: q("org")}}, true},
		{"rename", []sightmap.URLPropertyDef{{Name: "tenant", Extract: p("org_id")}}, false},
		{"restate", []sightmap.URLPropertyDef{{Name: "org_id", Extract: p("org_id")}}, false},
		{"rename frees the name", []sightmap.URLPropertyDef{{Name: "tenant", Extract: p("org_id")}, {Name: "org_id", Extract: q("org")}}, false},
		{"unrelated name", []sightmap.URLPropertyDef{{Name: "variant", Extract: q("variant")}}, false},
	} {
		errs := sightmap.Validate(viewCorpus(sightmap.ViewDef{Name: "V", Route: "/org/:org_id/settings", Properties: tc.props}))
		if got := hasCode(errs, "route-binding-conflict"); got != tc.want {
			t.Errorf("%s: route-binding-conflict = %v, want %v (all: %v)", tc.name, got, tc.want, findingCodes(errs))
		}
	}
	errs := sightmap.Validate(&sightmap.Corpus{Requests: []sightmap.RequestDef{{
		Name: "GetOrder", Route: "/api/orders/:order_id",
		Properties: []sightmap.RequestPropertyDef{{Name: "order_id", Extract: sightmap.Extract{From: sightmap.FromRspBody, Path: "id"}}},
	}}})
	if !hasCode(errs, "route-binding-conflict") {
		t.Errorf("request: a payload property named like a binding should conflict, got %v", findingCodes(errs))
	}
}

func loadAndValidate(t *testing.T, yaml string) []sightmap.ValidationError {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte("version: 1\n"+yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return sightmap.Validate(c)
}

// A :segment that is not a valid property name still matches, but binds
// nothing, so it is surfaced rather than silently ignored.
func TestValidate_RouteParamUnbound(t *testing.T) {
	for route, want := range map[string]bool{
		"/org/:orgId":    true,
		"/org/:org-id":   true,
		"/org/:org_id":   false,
		"/org/**":        false,
		"/org/:id/:Name": true,
	} {
		errs := sightmap.Validate(viewCorpus(sightmap.ViewDef{Name: "V", Route: route}))
		if got := hasCode(errs, "route-param-unbound"); got != want {
			t.Errorf("route %q: route-param-unbound = %v, want %v (%v)", route, got, want, findingCodes(errs))
		}
	}
}

// url.query and url.path are reachable only through the extract object; the
// deprecated source key never accepted them.
func TestValidate_LegacySourceRejectsURL(t *testing.T) {
	errs := loadAndValidate(t, "requests:\n  - name: R\n    route: /r\n    properties:\n      - name: v\n        source: url.query\n        field: v\n")
	if !hasCode(errs, "request-property-source-invalid") {
		t.Errorf("want request-property-source-invalid, got %v", findingCodes(errs))
	}
}

// A URL-sourced request property under a reserved name still warns.
func TestValidate_URLRequestPropertyShadowsReserved(t *testing.T) {
	errs := sightmap.Validate(&sightmap.Corpus{Requests: []sightmap.RequestDef{{
		Name: "R", Route: "/r",
		Properties: []sightmap.RequestPropertyDef{{Name: "status", Extract: sightmap.Extract{From: sightmap.FromURLQuery, Path: "status"}}},
	}}})
	if !hasCode(errs, "request-property-shadows-reserved") {
		t.Errorf("want request-property-shadows-reserved, got %v", findingCodes(errs))
	}
}

// A view property name must be a string, as the JSON Schema requires.
func TestValidate_ViewPropertyNameIsString(t *testing.T) {
	errs := loadAndValidate(t, "views:\n  - name: V\n    route: /v\n    properties:\n      - name: true\n        extract: { from: url.query, path: v }\n")
	if !hasCode(errs, "field-type-invalid") {
		t.Errorf("want field-type-invalid, got %v", findingCodes(errs))
	}
}
