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
