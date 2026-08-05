package main

import (
	"testing"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// TestComputeStats_DedupesSharedGlobals models the loader's output for a
// multi-file corpus where a global component (Navigation) is $ref-reused by
// two views: the loader deep-copies the global into each view's flattened
// list, so it appears three times across the corpus. Totals must count it —
// and its memory entries — exactly once (AllComponents' first-seen-name
// dedupe), while each per-view row still counts its own expanded copy.
func TestComputeStats_DedupesSharedGlobals(t *testing.T) {
	nav := match.ComponentDef{
		Name:      "Navigation",
		Selectors: []string{`nav[data-component="Navigation"]`},
		Memory:    []string{"sticky on scroll"},
	}
	corpus := &sightmap.Corpus{
		Memory: []string{"file-level note"},
		GlobalComponents: []match.ComponentDef{
			nav,
			{Name: "Footer", Selectors: []string{`footer[data-component="Footer"]`}},
		},
		Requests: []sightmap.RequestDef{
			{Name: "GetCurrentUser", Route: "/api/me", Method: "GET"},
		},
		Views: []sightmap.View{
			{
				Name:   "Checkout",
				Route:  "/checkout",
				Memory: []string{"guest checkout allowed"},
				Components: []match.ComponentDef{
					nav, // $ref-expanded copy of the global
					{Name: "CartSummary", Selectors: []string{`[data-component="CartSummary"]`}},
					{Name: "PaymentForm", Selectors: []string{`[data-component="PaymentForm"]`}},
					{Name: "submit", Selectors: []string{`[data-component="PaymentForm"] button[type="submit"]`}},
				},
				Requests: []sightmap.RequestDef{
					{Name: "PlaceOrder", Route: "/api/orders", Method: "POST",
						Memory: []string{"Idempotency-Key header is required"}},
				},
			},
			{
				Name:  "Dashboard",
				Route: "/dashboard",
				Components: []match.ComponentDef{
					nav, // same global, reused by a second view
					{Name: "ActivityFeed", Selectors: []string{`[data-component="ActivityFeed"]`},
						Properties: []match.Property{{Name: "count", Extract: "text"}}},
				},
			},
		},
	}

	s := computeStats(corpus)

	if s.Views != 2 {
		t.Errorf("Views = %d, want 2", s.Views)
	}
	// Navigation appears 3× (global + 2 views) but is one distinct component:
	// Navigation, Footer, CartSummary, PaymentForm, submit, ActivityFeed.
	if s.Components != 6 {
		t.Errorf("Components = %d, want 6 (Navigation deduped to one)", s.Components)
	}
	if s.Requests != 2 {
		t.Errorf("Requests = %d, want 2 (1 global + 1 view-scoped)", s.Requests)
	}
	if s.Properties != 1 {
		t.Errorf("Properties = %d, want 1", s.Properties)
	}
	// file(1) + Navigation(1, once despite 3 copies) + Checkout view(1) + PlaceOrder(1).
	if s.Memory != 4 {
		t.Errorf("Memory = %d, want 4 (Navigation's entry counted once)", s.Memory)
	}

	want := []viewStats{
		{Name: "Checkout", Route: "/checkout", Components: 4, Requests: 1},
		{Name: "Dashboard", Route: "/dashboard", Components: 2, Requests: 0},
	}
	if len(s.PerView) != len(want) {
		t.Fatalf("PerView has %d rows, want %d: %+v", len(s.PerView), len(want), s.PerView)
	}
	for i, w := range want {
		if s.PerView[i] != w {
			t.Errorf("PerView[%d] = %+v, want %+v", i, s.PerView[i], w)
		}
	}
}

// TestComputeStats_GlobalsOnlyCorpus: a corpus with no views still reports its
// global components and requests in the totals, with an empty (not nil)
// per_view list so the JSON contract serializes it as [].
func TestComputeStats_GlobalsOnlyCorpus(t *testing.T) {
	corpus := &sightmap.Corpus{
		GlobalComponents: []match.ComponentDef{
			{Name: "Header", Selectors: []string{"#header"}},
		},
		Requests: []sightmap.RequestDef{
			{Name: "Ping", Route: "/api/ping"},
		},
	}

	s := computeStats(corpus)

	if s.Views != 0 || s.Components != 1 || s.Requests != 1 {
		t.Errorf("got views=%d components=%d requests=%d, want 0/1/1",
			s.Views, s.Components, s.Requests)
	}
	if s.PerView == nil || len(s.PerView) != 0 {
		t.Errorf("PerView = %#v, want empty non-nil slice", s.PerView)
	}
}
