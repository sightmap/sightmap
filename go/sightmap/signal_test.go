package sightmap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSignalsLoadAndResolve exercises the full wiring: a corpus YAML with a
// `signals:` block loads into Corpus.Signals, resolves a view-ref to the view
// (route-active predicate) and a component-ref to the component (present
// predicate), reports an unknown name as unresolved, and validates clean with no
// spurious "unknown field" warning for `signals`.
func TestSignalsLoadAndResolve(t *testing.T) {
	dir := t.TempDir()
	yaml := `version: 1
views:
  - name: Checkout
    route: /booking/checkout
    components:
      - name: UpsellModal
        selector: .jtpsdk-popup-modal
signals:
  - name: checkout.reached
    ref: Checkout
  - name: upsell.present
    ref: UpsellModal
`
	if err := os.WriteFile(filepath.Join(dir, "checkout.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := DirLoader(dir).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Signals) != 2 {
		t.Fatalf("want 2 signals, got %d (%+v)", len(c.Signals), c.Signals)
	}

	if tgt := c.ResolveSignal("checkout.reached"); tgt.Kind != SignalRefView || tgt.View == nil || tgt.View.Route != "/booking/checkout" {
		t.Fatalf("checkout.reached: want view Checkout (/booking/checkout), got %+v", tgt)
	}
	if tgt := c.ResolveSignal("upsell.present"); tgt.Kind != SignalRefComponent || tgt.Component == nil || tgt.Component.Name != "UpsellModal" {
		t.Fatalf("upsell.present: want component UpsellModal, got %+v", tgt)
	}
	if tgt := c.ResolveSignal("nope"); tgt.Kind != SignalRefUnresolved {
		t.Fatalf("nope: want unresolved, got %+v", tgt)
	}

	for _, e := range Validate(c) {
		if strings.HasPrefix(e.Code, "signal-") {
			t.Errorf("unexpected signal validation error: %+v", e)
		}
		if strings.Contains(strings.ToLower(e.Message), "unknown") && strings.Contains(e.Message, "signals") {
			t.Errorf("`signals` wrongly flagged as an unknown field: %+v", e)
		}
	}
}

// TestCheckSignalsErrors covers each validation error path directly against a
// constructed corpus: ambiguous ref (a component and a view share a name),
// unresolved ref, missing name, missing ref, and a duplicate signal name.
func TestCheckSignalsErrors(t *testing.T) {
	c := &Corpus{
		GlobalComponents: []ComponentDef{{Name: "Button"}},
		// "Button" also names a view, so a ref to it is ambiguous across kinds.
		Views: []ViewDef{{Name: "Home", Route: "/"}, {Name: "Button", Route: "/b"}},
		Signals: []SignalDef{
			{Name: "amb", Ref: "Button"},  // ambiguous: component + view both named Button
			{Name: "good", Ref: "Home"},   // fine
			{Name: "bad", Ref: "Missing"}, // unresolved
			{Name: "", Ref: "Home"},       // missing name
			{Name: "noref"},               // missing ref
			{Name: "good", Ref: "Home"},   // duplicate name
		},
	}

	codes := map[string]int{}
	for _, e := range checkSignals(c) {
		codes[e.Code]++
	}
	for _, code := range []string{
		"signal-ref-ambiguous",
		"signal-ref-unresolved",
		"missing-name",
		"missing-ref",
		"merge-collision-signal",
	} {
		if codes[code] == 0 {
			t.Errorf("expected a %q error; got codes %v", code, codes)
		}
	}
}

// Validation accepts request and message refs, so the resolver consumers call
// must resolve them too, and their tags must reach the signal (SEP-0016).
func TestSignalsResolveRequestAndMessageRefs(t *testing.T) {
	dir := t.TempDir()
	yaml := `version: 1
views:
  - name: Checkout
    route: /checkout
    requests:
      - name: ApplyCoupon
        route: /api/coupon
        tags: [promo]
requests:
  - name: CheckoutPayment
    route: /api/checkout/pay
    tags: [payments]
messages:
  - name: ChunkLoadFailed
    level: EXCEPTION
    tags: [deploy]
signals:
  - name: payment.seen
    ref: CheckoutPayment
    tags: [defect]
  - name: coupon.seen
    ref: ApplyCoupon
  - name: chunk.failed
    ref: ChunkLoadFailed
`
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := DirLoader(dir).Load()
	if err != nil {
		t.Fatal(err)
	}

	if tgt := c.ResolveSignal("payment.seen"); tgt.Kind != SignalRefRequest || tgt.Request == nil || tgt.Request.Name != "CheckoutPayment" {
		t.Errorf("payment.seen: want request CheckoutPayment, got %+v", tgt)
	}
	if tgt := c.ResolveSignal("coupon.seen"); tgt.Kind != SignalRefRequest || tgt.Request == nil || tgt.Request.Name != "ApplyCoupon" {
		t.Errorf("coupon.seen: want view-scoped request ApplyCoupon, got %+v", tgt)
	}
	if tgt := c.ResolveSignal("chunk.failed"); tgt.Kind != SignalRefMessage || tgt.Message == nil || tgt.Message.Name != "ChunkLoadFailed" {
		t.Errorf("chunk.failed: want message ChunkLoadFailed, got %+v", tgt)
	}

	for name, want := range map[string]string{
		"payment.seen": "defect,payments",
		"coupon.seen":  "promo",
		"chunk.failed": "deploy",
	} {
		if got := strings.Join(c.TagsForSignal(name), ","); got != want {
			t.Errorf("TagsForSignal(%q) = %q, want %q", name, got, want)
		}
	}
}

// A name shared by a request and a message resolves to nothing, as validation
// reports it ambiguous.
func TestSignalsResolveAmbiguousAcrossRequestAndMessage(t *testing.T) {
	c := &Corpus{
		Requests: []RequestDef{{Name: "Dup", Route: "/api/x"}},
		Messages: []MessageDef{{Name: "Dup", Level: "error"}},
		Signals:  []SignalDef{{Name: "s", Ref: "Dup"}},
	}
	if tgt := c.ResolveSignal("s"); tgt.Kind != SignalRefUnresolved {
		t.Fatalf("want unresolved for a ref shared by a request and a message, got %+v", tgt)
	}
}
