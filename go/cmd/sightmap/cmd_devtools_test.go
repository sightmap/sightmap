package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

func TestAnnotateConsole(t *testing.T) {
	corpus := &sightmap.Corpus{
		Messages: []sightmap.MessageDef{
			{Name: "CartVersionMismatch", Level: "error", Message: "cart version mismatch"},
			{Name: "AnyError", Level: "error"},
		},
	}
	msgs := []sightmap.Message{
		{Index: 1, Level: "error", Text: "cart version mismatch detected"}, // both defs
		{Index: 2, Level: "info", Text: "all good"},                        // none
	}
	got := annotateConsole(corpus, msgs)

	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d", len(got))
	}
	// Record fields survive (embedding) and matches are attached in corpus order.
	if got[0].Index != 1 || !reflect.DeepEqual(got[0].Matches, []string{"CartVersionMismatch", "AnyError"}) {
		t.Errorf("entry0 = %+v", got[0])
	}
	if got[1].Index != 2 || len(got[1].Matches) != 0 {
		t.Errorf("entry1 should have no matches, got %+v", got[1])
	}
}

func TestAnnotateNetwork(t *testing.T) {
	corpus := &sightmap.Corpus{
		Requests: []sightmap.RequestDef{
			{Name: "CheckoutPayment", Route: "/api/checkout/pay", Method: "POST"},
			{Name: "AnyCheckout", Route: "/api/checkout/**"}, // any method
		},
	}
	reqs := []sightmap.Request{
		{Index: 1, Method: "POST", URL: "https://x.test/api/checkout/pay"}, // both
		{Index: 2, Method: "GET", URL: "https://x.test/api/other"},         // none
	}
	got := annotateNetwork(corpus, reqs)

	if got[0].URL != "https://x.test/api/checkout/pay" ||
		!reflect.DeepEqual(got[0].Matches, []string{"CheckoutPayment", "AnyCheckout"}) {
		t.Errorf("entry0 = %+v", got[0])
	}
	if len(got[1].Matches) != 0 {
		t.Errorf("entry1 should have no matches, got %+v", got[1])
	}
}

func TestMatchSlot(t *testing.T) {
	cases := []struct {
		name    string
		matches []string
		want    string // trimmed of the alignment padding
	}{
		{"unmatched", nil, "[--]"},
		{"single", []string{"CartVersionMismatch"}, "[CartVersionMismatch]"},
		{"multi (ambiguity up front)", []string{"CheckoutPayment", "AnyCheckout"}, "[CheckoutPayment, AnyCheckout]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.TrimRight(matchSlot(tc.matches), " ")
			if got != tc.want {
				t.Errorf("matchSlot(%v) = %q, want %q", tc.matches, got, tc.want)
			}
		})
	}
	// The unmatched token stays quiet (no def name could be mistaken for it).
	if strings.Contains(matchSlot(nil), "match") {
		t.Error("unmatched token should be an unobtrusive placeholder, not a word")
	}
}

// A nil corpus (none loaded, or a load error) must degrade to entries with no
// matches rather than panicking — devtools stays useful without a corpus.
func TestAnnotate_NilCorpusDegrades(t *testing.T) {
	c := annotateConsole(nil, []sightmap.Message{{Index: 1, Level: "error", Text: "x"}})
	if len(c) != 1 || len(c[0].Matches) != 0 {
		t.Errorf("console nil-corpus = %+v", c)
	}
	n := annotateNetwork(nil, []sightmap.Request{{Index: 1, Method: "GET", URL: "https://x.test/a"}})
	if len(n) != 1 || len(n[0].Matches) != 0 {
		t.Errorf("network nil-corpus = %+v", n)
	}
}
