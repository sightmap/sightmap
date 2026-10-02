package match_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

const privacyCorpus = `
version: 1
components:
  - name: CheckoutSummary
    selector: .summary
    properties:
      - name: card
        extract: CardNumberInput.value
      - name: hasCard
        extract: exists:CardNumberInput
      - name: total
        extract: OrderTotal.amount
    children:
      - name: CardNumberInput
        selector: input.cc
        privacy: block
        properties:
          - name: value
            extract: attr=value
      - name: CheckoutForm
        selector: form
        privacy: mask
        watch: true
        properties:
          - name: label
            extract: text
          - name: plan
            extract: attr=data-plan
          - name: hasSubmit
            extract: exists:SubmitButton
        children:
          - name: SubmitButton
            selector: button
            properties:
              - name: off
                extract: attr=disabled
              - name: label
                extract: raw_text
          - name: OrderTotal
            selector: .total
            privacy: unmask
            properties:
              - name: amount
                extract: raw_text
`

func privEl(tag, class string, attrs map[string]string) *sightmap.Element {
	e := &sightmap.Element{Tag: tag, Attrs: attrs}
	if class != "" {
		e.Classes = []string{class}
		if e.Attrs == nil {
			e.Attrs = map[string]string{}
		}
		e.Attrs["class"] = class
	}
	return e
}

func TestMatch_PrivacyWithholdsProperties(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte(privacyCorpus), 0o644); err != nil {
		t.Fatal(err)
	}
	corpus, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	cc := &sightmap.ComponentNode{Id: "cc", Element: privEl("input", "cc", map[string]string{"value": "4111"})}
	submit := &sightmap.ComponentNode{Id: "submit", RawText: "Pay", State: map[string]string{"disabled": "true"},
		Element: privEl("button", "", nil)}
	total := &sightmap.ComponentNode{Id: "total", RawText: "$10.95", Element: privEl("div", "total", nil)}
	form := &sightmap.ComponentNode{Id: "form", Name: "Checkout", Children: []*sightmap.ComponentNode{submit, total},
		Element: privEl("form", "", map[string]string{"data-plan": "pro"})}
	summary := &sightmap.ComponentNode{Id: "summary", Children: []*sightmap.ComponentNode{cc, form},
		Element: privEl("div", "summary", nil)}

	res := match.NewMatcher(corpus).Match(summary, "")

	cases := []struct {
		node *sightmap.ComponentNode
		prop string
		want string // "" means withheld
	}{
		{cc, "value", ""},            // read from a blocked node
		{summary, "card", ""},        // laundering out of a blocked descendant
		{summary, "hasCard", ""},     // exists: on a blocked target
		{summary, "total", "$10.95"}, // unmask carves the total out of the mask
		{form, "label", ""},          // text under mask
		{form, "plan", ""},           // attr= under mask
		{form, "hasSubmit", "true"},  // exists: under mask reports structure
		{submit, "off", "true"},      // state attribute under mask
		{submit, "label", ""},        // raw_text under an inherited mask
		{total, "amount", "$10.95"},  // unmask
	}
	for _, c := range cases {
		got, ok := propVal(res[c.node], c.prop)
		if c.want == "" && ok {
			t.Errorf("%s.%s = %q; want withheld", res[c.node].Name, c.prop, got)
		}
		if c.want != "" && (!ok || got != c.want) {
			t.Errorf("%s.%s = %q, %v; want %q", res[c.node].Name, c.prop, got, ok, c.want)
		}
	}

	for node, want := range map[*sightmap.ComponentNode]string{
		summary: "", cc: "block", form: "mask", submit: "mask", total: "unmask",
	} {
		if got := res[node].Privacy; got != want {
			t.Errorf("%s.Privacy = %q, want %q", res[node].Name, got, want)
		}
	}
	if !res[form].Watch || res[submit].Watch {
		t.Errorf("Watch: form=%v submit=%v; want true, false (watch never inherits)", res[form].Watch, res[submit].Watch)
	}
}
