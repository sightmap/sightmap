package sightmap

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// jsonBody is a small helper for building a JSON *Body.
func jsonBody(content string) *Body {
	return &Body{Content: content, ContentType: "application/json", Size: len(content)}
}

func TestExtractProperties_BodyField(t *testing.T) {
	d := &RequestDef{
		Name: "CheckoutPayment", Route: "/api/checkout/pay", Method: "POST",
		Properties: []RequestPropertyDef{
			{Name: "outcome", Source: "rsp.body", Field: "status"},
		},
	}
	rec := Request{RspBody: jsonBody(`{"status":"declined","amount":42}`)}

	got := d.ExtractProperties(rec)
	want := []PropertyValue{{Name: "outcome", Value: "declined"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestExtractProperties_BodyNestedAndArrayIndex(t *testing.T) {
	d := &RequestDef{
		Properties: []RequestPropertyDef{
			{Name: "first_item", Source: "rsp.body", Field: "items.0.name"},
			{Name: "count", Source: "rsp.body", Field: "meta.count"},
			{Name: "flag", Source: "rsp.body", Field: "meta.ok"},
		},
	}
	rec := Request{RspBody: jsonBody(`{"items":[{"name":"widget"},{"name":"gadget"}],"meta":{"count":8,"ok":true}}`)}

	got := d.ExtractProperties(rec)
	want := []PropertyValue{
		{Name: "first_item", Value: "widget"},
		{Name: "count", Value: "8"}, // float64 8 must render "8", not "8.0"
		{Name: "flag", Value: "true"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestExtractProperties_HeaderWithPattern(t *testing.T) {
	d := &RequestDef{
		Properties: []RequestPropertyDef{
			{Name: "rate_limit_remaining", Source: "rsp.headers", Field: "X-RateLimit-Remaining", Pattern: `(\d+)`},
		},
	}
	// Header name differs in case from the def — lookup is case-insensitive.
	rec := Request{RspHeaders: []Header{{Name: "x-ratelimit-remaining", Value: "0"}}}

	got := d.ExtractProperties(rec)
	want := []PropertyValue{{Name: "rate_limit_remaining", Value: "0"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestExtractProperties_DuplicateHeadersJoined(t *testing.T) {
	d := &RequestDef{
		Properties: []RequestPropertyDef{
			{Name: "cookies", Source: "rsp.headers", Field: "Set-Cookie"},
		},
	}
	rec := Request{RspHeaders: []Header{
		{Name: "Set-Cookie", Value: "a=1"},
		{Name: "set-cookie", Value: "b=2"},
	}}
	got := d.ExtractProperties(rec)
	want := []PropertyValue{{Name: "cookies", Value: "a=1, b=2"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestExtractProperties_PatternNoFieldScansRawBody(t *testing.T) {
	d := &RequestDef{
		Properties: []RequestPropertyDef{
			// Form-encoded body: no JSON to walk, pattern scans the raw text.
			{Name: "legacy_outcome", Source: "rsp.body", Pattern: `(?:declined|approved|deferred)`},
		},
	}
	rec := Request{RspBody: &Body{Content: "result=declined&code=51", ContentType: "application/x-www-form-urlencoded"}}

	got := d.ExtractProperties(rec)
	want := []PropertyValue{{Name: "legacy_outcome", Value: "declined"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestExtractProperties_SilentOmission(t *testing.T) {
	d := &RequestDef{
		Properties: []RequestPropertyDef{
			{Name: "missing_key", Source: "rsp.body", Field: "nope"},
			{Name: "missing_body", Source: "req.body", Field: "x"}, // ReqBody nil
			{Name: "missing_header", Source: "rsp.headers", Field: "X-Absent"},
			{Name: "not_json", Source: "rsp.body", Field: "a.b"}, // body isn't JSON
			{Name: "no_pattern_match", Source: "rsp.body", Field: "status", Pattern: `zzz`},
			{Name: "null_leaf", Source: "rsp.body", Field: "maybe"}, // JSON null → absent
			{Name: "resolved", Source: "rsp.body", Field: "status"}, // the one that should survive
		},
	}
	rec := Request{RspBody: jsonBody(`{"status":"ok","maybe":null}`)}

	got := d.ExtractProperties(rec)
	want := []PropertyValue{{Name: "resolved", Value: "ok"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v (omission should drop every unresolved property)", got, want)
	}
}

func TestExtractProperties_CompositeLeafEncodedForPattern(t *testing.T) {
	// A field resolving to an object, refined by a pattern scanning its JSON.
	d := &RequestDef{
		Properties: []RequestPropertyDef{
			{Name: "err_code", Source: "rsp.body", Field: "error", Pattern: `"code":"([A-Z0-9_]+)"`},
		},
	}
	rec := Request{RspBody: jsonBody(`{"error":{"code":"CARD_DECLINED","msg":"no"}}`)}
	got := d.ExtractProperties(rec)
	want := []PropertyValue{{Name: "err_code", Value: "CARD_DECLINED"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestExtractProperties_InvalidPatternOmits(t *testing.T) {
	// A record built from an unvalidated corpus may carry a bad pattern; it must
	// omit silently, not panic.
	d := &RequestDef{
		Properties: []RequestPropertyDef{
			{Name: "x", Source: "rsp.body", Field: "status", Pattern: `(unclosed`},
		},
	}
	rec := Request{RspBody: jsonBody(`{"status":"ok"}`)}
	if got := d.ExtractProperties(rec); got != nil {
		t.Fatalf("bad pattern should omit, got %+v", got)
	}
}

func TestRequestsForRecord_IdentityPlusExtraction(t *testing.T) {
	c := &Corpus{
		Requests: []RequestDef{
			{
				Name: "CheckoutPayment", Route: "/api/checkout/pay", Method: "POST",
				Tags: []string{"defect-prone"}, Memory: []string{"declines look like 200s"},
				Properties: []RequestPropertyDef{{Name: "outcome", Source: "rsp.body", Field: "status"}},
			},
			{Name: "GetUser", Route: "/api/me", Method: "GET"}, // different route, shouldn't match
		},
	}
	rec := Request{
		Method: "POST", URL: "https://shop.example.com/api/checkout/pay?ref=x",
		Status: 200, RspBody: jsonBody(`{"status":"declined"}`),
	}

	got := c.RequestsForRecord(rec)
	if len(got) != 1 {
		t.Fatalf("want 1 match, got %d (%+v)", len(got), got)
	}
	m := got[0]
	if m.Name != "CheckoutPayment" {
		t.Errorf("name = %q", m.Name)
	}
	if !reflect.DeepEqual(m.Tags, []string{"defect-prone"}) || !reflect.DeepEqual(m.Memory, []string{"declines look like 200s"}) {
		t.Errorf("memory/tags not carried: %+v", m)
	}
	if !reflect.DeepEqual(m.Properties, []PropertyValue{{Name: "outcome", Value: "declined"}}) {
		t.Errorf("properties = %+v", m.Properties)
	}
}

func TestRequestsForRecord_NoMatch(t *testing.T) {
	c := &Corpus{Requests: []RequestDef{{Name: "X", Route: "/a", Method: "GET"}}}
	if got := c.RequestsForRecord(Request{Method: "GET", URL: "https://h/b"}); got != nil {
		t.Fatalf("want nil, got %+v", got)
	}
}

func TestRequestsForRecord_MatchWithNoPropertiesHasNilProps(t *testing.T) {
	c := &Corpus{Requests: []RequestDef{{Name: "Ping", Route: "/ping", Method: "GET"}}}
	got := c.RequestsForRecord(Request{Method: "GET", URL: "https://h/ping"})
	if len(got) != 1 || got[0].Properties != nil {
		t.Fatalf("want one match with nil Properties, got %+v", got)
	}
}

func TestRequest_WireStaysLeanWhenUnpopulated(t *testing.T) {
	// A record with no captured payload must marshal without the optional
	// fields — the #205 "faithful but optional" promise: a lazy producer's wire
	// is unchanged from before the extension.
	rec := Request{Index: 3, Method: "GET", URL: "https://h/x", Status: 200}
	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"durationMs", "reqHeaders", "rspHeaders", "reqBody", "rspBody"} {
		if strings.Contains(string(b), f) {
			t.Errorf("unpopulated %q leaked onto the wire: %s", f, b)
		}
	}
}
