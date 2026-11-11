package match_test

import (
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

func TestMatchChain_CarriesIDs(t *testing.T) {
	m := chainMatcher(
		sightmap.ComponentDef{ID: "form0001", Name: "CheckoutForm", Selectors: []string{"form.checkout"}},
		sightmap.ComponentDef{ID: "btn00001", RefID: "ref00001", Name: "SubmitButton", Selectors: []string{"button.submit"}},
	)
	got := m.MatchChain([]sightmap.Element{el("form", "checkout"), el("button", "submit")}, "")
	if len(got) != 2 {
		t.Fatalf("MatchChain = %v, want 2 matches", got)
	}
	if got[0].ID != "form0001" || got[1].ID != "btn00001" || got[1].RefID != "ref00001" {
		t.Errorf("ids = %q %q %q, want form0001 btn00001 ref00001", got[0].ID, got[1].ID, got[1].RefID)
	}
}
