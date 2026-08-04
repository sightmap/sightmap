package sightmap_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// A Corpus serializes to the lean wire form (memory/globals/views/requests),
// with view authoring fields (url/access/snapshots/sourceFile/stability)
// excluded, and round-trips its wire fields without loss.
func TestCorpusWireRoundTrip(t *testing.T) {
	orig := &sightmap.Corpus{
		Memory:           []string{"file note"},
		GlobalComponents: []match.ComponentDef{{Name: "Nav", Selectors: []string{"nav"}}},
		Requests: []sightmap.RequestDef{
			{Name: "Search", Route: "/api/search", Method: "POST",
				Request: &sightmap.Payload{Fields: []sightmap.Field{{Name: "q", Type: "string"}}}},
		},
		Messages: []sightmap.MessageDef{{Name: "CartVersionMismatch", Level: "ERROR", Message: "cart version mismatch"}},
		Views: []sightmap.View{{
			Name:       "Home",
			Route:      "/",
			Components: []match.ComponentDef{{Name: "Hero", Selectors: []string{".hero"}}},
			Requests:   []sightmap.RequestDef{{Name: "Ping", Route: "/api/ping"}},
			// Authoring fields that must NOT appear on the wire:
			URL:        "https://x.test/",
			SourceFile: "homefile",
			Stability:  "stub",
			Access:     sightmap.Access{Status: "blocked", Reason: "admin"},
			Snapshots:  []sightmap.Snapshot{{Name: "base"}},
		}},
	}

	blob, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	js := string(blob)

	for _, want := range []string{`"memory"`, `"globals"`, `"views"`, `"requests"`, `"route":"/api/search"`, `"method":"POST"`, `"fields"`, `"messages"`, `"level":"ERROR"`} {
		if !strings.Contains(js, want) {
			t.Errorf("wire JSON missing %s:\n%s", want, js)
		}
	}
	for _, bad := range []string{`"url"`, `"sourceFile"`, `"access"`, `"snapshots"`, `"stability"`, `"reason"`, `"blocked"`, `"homefile"`} {
		if strings.Contains(js, bad) {
			t.Errorf("wire JSON should not contain authoring token %s:\n%s", bad, js)
		}
	}

	var back sightmap.Corpus
	if err := json.Unmarshal(blob, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(back.Views) != 1 || back.Views[0].Name != "Home" || back.Views[0].Route != "/" {
		t.Fatalf("view did not round-trip: %+v", back.Views)
	}
	if len(back.Views[0].Requests) != 1 || back.Views[0].Requests[0].Name != "Ping" {
		t.Errorf("view requests did not round-trip: %+v", back.Views[0].Requests)
	}
	if len(back.Requests) != 1 || back.Requests[0].Method != "POST" ||
		back.Requests[0].Request == nil || len(back.Requests[0].Request.Fields) != 1 {
		t.Errorf("global request did not round-trip: %+v", back.Requests)
	}
	if len(back.Messages) != 1 || back.Messages[0].Name != "CartVersionMismatch" ||
		back.Messages[0].Level != "ERROR" || back.Messages[0].Message != "cart version mismatch" {
		t.Errorf("messages did not round-trip: %+v", back.Messages)
	}
	if back.Views[0].URL != "" || back.Views[0].Stability != "" || back.Views[0].SourceFile != "" {
		t.Errorf("authoring fields should be empty after a wire round-trip: %+v", back.Views[0])
	}
}

// The three SEP entities must survive serialization into the uploaded wire
// form. A field without a JSON tag would silently vanish from the payload.
func TestCorpusWire_IncludesSEPEntities(t *testing.T) {
	c := &sightmap.Corpus{
		Requests: []sightmap.RequestDef{{
			Name:  "CheckoutPayment",
			Route: "/api/checkout/pay",
			Tags:  []string{"checkout"},
			Properties: []sightmap.RequestProperty{
				{Name: "outcome", Source: "rsp.body", Field: "status", Transform: "slug"},
			},
		}},
		Messages: []sightmap.MessageDef{{Name: "CartVersionMismatch", Level: "ERROR", Message: "mismatch"}},
		Signals: []sightmap.SignalDef{{
			Name:   "checkout.payment.declined",
			Ref:    "CheckoutPayment",
			Tags:   []string{"defect"},
			Filter: map[string][]string{"outcome": {"declined"}},
		}},
	}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	wire := string(b)
	for _, want := range []string{
		`"requests"`, `"properties"`, `"source":"rsp.body"`, `"field":"status"`, `"transform":"slug"`, `"tags":["checkout"]`,
		`"messages"`, `"level":"ERROR"`,
		`"signals"`, `"ref":"CheckoutPayment"`, `"filter":{"outcome":["declined"]}`,
	} {
		if !strings.Contains(wire, want) {
			t.Errorf("wire form is missing %s\ngot: %s", want, wire)
		}
	}
}
