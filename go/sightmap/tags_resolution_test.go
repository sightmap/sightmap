package sightmap_test

import (
	"reflect"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// View tags resolve as a union across EVERY matching view, not just the
// most-specific one that wins identity. That is the whole point of SEP-0004's
// rule: a broad tagged view must not be shadowed by a narrower untagged one.
func TestTagsForURL_UnionNotIdentity(t *testing.T) {
	c := &sightmap.Corpus{Views: []sightmap.ViewDef{
		{Name: "Checkout", Route: "/checkout/**", Tags: []string{"defect"}},
		{Name: "CheckoutPayment", Route: "/checkout/payment", Tags: []string{"payments"}},
		{Name: "Unrelated", Route: "/account", Tags: []string{"nope"}},
	}}
	// Identity here is CheckoutPayment (most specific), which carries only
	// "payments". The union must still include the broader view's "defect".
	if got, want := c.TagsForURL("/checkout/payment"), []string{"defect", "payments"}; !reflect.DeepEqual(got, want) {
		t.Errorf("TagsForURL = %v, want %v", got, want)
	}
	if got, want := c.TagsForURL("/checkout/cart"), []string{"defect"}; !reflect.DeepEqual(got, want) {
		t.Errorf("TagsForURL = %v, want %v", got, want)
	}
	if got := c.TagsForURL("/account"); !reflect.DeepEqual(got, []string{"nope"}) {
		t.Errorf("TagsForURL = %v, want [nope]", got)
	}
	if got := c.TagsForURL("/nothing"); got != nil {
		t.Errorf("TagsForURL = %v, want nil for a URL matching no view", got)
	}
}

func TestTagsForURL_DedupedAndSorted(t *testing.T) {
	c := &sightmap.Corpus{Views: []sightmap.ViewDef{
		{Name: "A", Route: "/**", Tags: []string{"zeta", "alpha"}},
		{Name: "B", Route: "/x", Tags: []string{"alpha", "mid"}},
	}}
	if got, want := c.TagsForURL("/x"), []string{"alpha", "mid", "zeta"}; !reflect.DeepEqual(got, want) {
		t.Errorf("TagsForURL = %v, want %v (deduped, sorted)", got, want)
	}
}

// A signal is a classification ABOUT an entity, so it inherits that entity's
// tags. Without this, a request tagged "payments" would be invisible on every
// signal about it unless each author remembered to repeat it.
func TestTagsForSignal_InheritsFromRef(t *testing.T) {
	c := &sightmap.Corpus{
		GlobalComponents: []sightmap.ComponentDef{
			{Name: "CheckoutForm", Selectors: []string{".f"}, Tags: []string{"ui-risk", "defect"}},
		},
		Views: []sightmap.ViewDef{{Name: "Checkout", Route: "/checkout", Tags: []string{"flow"}}},
		Signals: []sightmap.SignalDef{
			{Name: "form.present", Ref: "CheckoutForm", Tags: []string{"state-risk"}},
			{Name: "checkout.active", Ref: "Checkout"},
			{Name: "dangling", Ref: "NoSuchThing", Tags: []string{"own"}},
		},
	}
	cases := []struct {
		signal string
		want   []string
	}{
		{"form.present", []string{"defect", "state-risk", "ui-risk"}},
		{"checkout.active", []string{"flow"}}, // no tags of its own; inherits the view's
		{"dangling", []string{"own"}},         // unresolved ref contributes nothing
	}
	for _, tc := range cases {
		if got := c.TagsForSignal(tc.signal); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("TagsForSignal(%q) = %v, want %v", tc.signal, got, tc.want)
		}
	}
	if got := c.TagsForSignal("unknown"); got != nil {
		t.Errorf("TagsForSignal(unknown) = %v, want nil", got)
	}
}

// Messages carry tags through the loader verbatim; the union across matching
// entries is the consumer's job, and survives the identity ambiguity SEP-0006
// requires a consumer to surface.
func TestMessageTagsLoad(t *testing.T) {
	c := &sightmap.Corpus{Messages: []sightmap.MessageDef{
		{Name: "AnyCheckoutError", Level: "ERROR", Tags: []string{"checkout"}},
		{Name: "CartVersionMismatch", Level: "ERROR", Message: "cart version mismatch", Tags: []string{"defect"}},
	}}
	if errs := sightmap.Validate(c); !hasCode(errs, "message-conflict") {
		t.Errorf("want the identity ambiguity reported, got %v", findingCodes(errs))
	}
	// Both definitions keep their own tags; nothing is shadowed by the conflict.
	if got := c.Messages[0].Tags; !reflect.DeepEqual(got, []string{"checkout"}) {
		t.Errorf("broad entry tags = %v, want [checkout]", got)
	}
	if got := c.Messages[1].Tags; !reflect.DeepEqual(got, []string{"defect"}) {
		t.Errorf("narrow entry tags = %v, want [defect]", got)
	}
}
