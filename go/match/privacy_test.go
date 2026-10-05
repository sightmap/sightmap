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
        extract: { from: component, path: CardNumberInput.value }
      - name: hasCard
        extract: { from: component.exists, path: CardNumberInput }
      - name: total
        extract: { from: component, path: OrderTotal.amount }
    children:
      - name: CardNumberInput
        selector: input.cc
        privacy: block
        properties:
          - name: value
            extract: { from: dom.attr, path: value }
      - name: CheckoutForm
        selector: form
        privacy: mask
        watch: true
        properties:
          - name: label
            extract: { from: dom.text }
          - name: plan
            extract: { from: dom.attr, path: data-plan }
          - name: hasSubmit
            extract: { from: component.exists, path: SubmitButton }
        children:
          - name: SubmitButton
            selector: button
            properties:
              - name: off
                extract: { from: dom.state, path: disabled }
              - name: label
                extract: { from: dom.raw_text }
          - name: OrderTotal
            selector: .total
            privacy: unmask
            properties:
              - name: amount
                extract: { from: dom.raw_text }
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

// A Name[] read judges privacy at each match: the blocked tag is dropped from
// the joined value while the others survive.
func TestMatch_PrivacyJudgedPerMatchInJoin(t *testing.T) {
	dir := t.TempDir()
	corpus := `
version: 1
components:
  - name: Card
    selector: .card
    properties:
      - name: tags
        extract: { from: component, path: 'Tag[].value', join: ',' }
    children:
      - name: Tag
        selector: .tag
        properties:
          - name: value
            extract: { from: dom.text }
      - name: SecretTag
        selector: .secret
        privacy: block
`
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte(corpus), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	tag := func(id, text string) *sightmap.ComponentNode {
		return &sightmap.ComponentNode{Id: id, Name: text, Element: privEl("span", "tag", nil)}
	}
	secret := &sightmap.ComponentNode{Id: "s", Element: privEl("div", "secret", nil),
		Children: []*sightmap.ComponentNode{tag("t2", "internal")}}
	card := &sightmap.ComponentNode{Id: "card", Element: privEl("div", "card", nil),
		Children: []*sightmap.ComponentNode{tag("t1", "sale"), secret, tag("t3", "new")}}
	res := match.NewMatcher(c).Match(card, "")
	if v, ok := propVal(res[card], "tags"); !ok || v != "sale,new" {
		t.Errorf("tags = %q, %v; want \"sale,new\" (the tag inside the blocked node is withheld)", v, ok)
	}
}
