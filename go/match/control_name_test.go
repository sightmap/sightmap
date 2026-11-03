package match_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// controlNameCorpus masks or blocks the field under test ([data-f] / [data-fb])
// and lets a case mask, block or unmask any other element by class.
const controlNameCorpus = `
components:
  - name: Field
    selector: '[data-f]'
    privacy: mask
    properties:
      - name: label
        extract: { from: dom.text }
      - name: value
        extract: { from: dom.attr, path: value }
      - name: aria
        extract: { from: dom.attr, path: aria-label }
      - name: raw
        extract: { from: dom.raw_text }
  - name: BlockedField
    selector: '[data-fb]'
    privacy: block
    properties:
      - name: label
        extract: { from: dom.text }
  - name: Masked
    selector: '.masked'
    privacy: mask
  - name: Blocked
    selector: '.blocked'
    privacy: block
  - name: Unmasked
    selector: '.unmasked'
    privacy: unmask
`

// cn builds a node: tag, then attribute name/value pairs. "id" also sets
// Element.Id, "class" also sets Element.Classes.
func cn(tag string, kv ...string) *sightmap.ComponentNode {
	e := &sightmap.Element{Tag: tag, Attrs: map[string]string{}}
	for i := 0; i+1 < len(kv); i += 2 {
		e.Attrs[kv[i]] = kv[i+1]
		switch kv[i] {
		case "id":
			e.Id = kv[i+1]
		case "class":
			e.Classes = strings.Fields(kv[i+1])
		}
	}
	return &sightmap.ComponentNode{Element: e, Properties: map[string]string{}}
}

func nk(n *sightmap.ComponentNode, role, name string, kids ...*sightmap.ComponentNode) *sightmap.ComponentNode {
	n.Role, n.Name, n.Children = role, name, kids
	return n
}

func textNode(s string) *sightmap.ComponentNode {
	return &sightmap.ComponentNode{Role: "StaticText", Name: s, Text: s, Properties: map[string]string{}}
}

func span(s string, kv ...string) *sightmap.ComponentNode {
	n := cn("span", kv...)
	n.Text, n.RawText = s, s
	n.Children = []*sightmap.ComponentNode{textNode(s)}
	return n
}

type controlCase struct {
	name string
	// build returns the page root and the field under test.
	build func(c *controlCase) (root, field *sightmap.ComponentNode)
	want  string // the label that resolves; "" when it must be withheld
}

// TestControlNameUnderMask is the table for SEP-0009's one text exception: under
// mask, an input, textarea or select's accessible name may resolve through
// dom.text when nothing it could be computed from is user content. Each
// withheld case is a way the name could carry content.
func TestControlNameUnderMask(t *testing.T) {
	field := func(tag, role, name string, kv ...string) *sightmap.ComponentNode {
		n := cn(tag, append([]string{"data-f", ""}, kv...)...)
		n.Role, n.Name = role, name
		return n
	}
	page := func(kids ...*sightmap.ComponentNode) *sightmap.ComponentNode {
		root := cn("body")
		root.Children = kids
		return root
	}
	label := func(kv ...string) *sightmap.ComponentNode { n := cn("label", kv...); n.Role = "LabelText"; return n }
	// solo: a page holding just the field.
	solo := func(f *sightmap.ComponentNode) func(*controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
		return func(*controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) { return page(f), f }
	}

	cases := []controlCase{
		// --- Names a control's construction keeps apart from its value.
		{name: "aria-label", want: "Promo code",
			build: solo(field("input", "textbox", "Promo code", "type", "text", "aria-label", "Promo code", "value", "SAVE10"))},
		{name: "placeholder only", want: "Search",
			build: solo(field("input", "searchbox", "Search", "type", "search", "placeholder", "Search"))},
		{name: "title only", want: "Postcode",
			build: solo(field("input", "textbox", "Postcode", "type", "text", "title", "Postcode", "value", "SW1A 1AA"))},
		{name: "missing type is text", want: "Email",
			build: solo(field("input", "textbox", "Email", "placeholder", "Email", "value", "a@example.com"))},
		{name: "unknown type is text", want: "Code",
			build: solo(field("input", "textbox", "Code", "type", "bogus", "aria-label", "Code"))},
		{name: "password with aria-label", want: "Password",
			build: solo(field("input", "textbox", "Password", "type", "password", "aria-label", "Password", "value", "hunter2"))},
		{name: "label for", want: "Street Address", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "textbox", "Street Address", "type", "text", "id", "street", "value", "290 Main St")
			return page(nk(label("for", "street"), "LabelText", "", textNode("Street Address")), f), f
		}},
		{name: "label for, id only in attrs", want: "City", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "textbox", "City", "type", "text", "value", "Springfield")
			f.Element.Attrs["id"] = "city"
			return page(nk(label("for", "city"), "LabelText", "", textNode("City")), f), f
		}},
		{name: "wrapping label", want: "Last Name", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "textbox", "Last Name", "type", "text", "value", "Doe")
			return page(nk(label(), "LabelText", "", textNode("Last Name"), f)), f
		}},
		{name: "checkbox whose value attribute is its label", want: "Bank Transfer", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "radio", "Bank Transfer", "type", "radio", "value", "Bank Transfer")
			return page(nk(label(), "LabelText", "", f, textNode("Bank Transfer"))), f
		}},
		{name: "checkbox with aria-label", want: "Remember me",
			build: solo(field("input", "checkbox", "Remember me", "type", "checkbox", "aria-label", "Remember me"))},
		{name: "range with aria-label", want: "Volume",
			build: solo(field("input", "slider", "Volume", "type", "range", "aria-label", "Volume", "value", "30"))},
		{name: "date with label", want: "Birthday", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "Date", "Birthday", "type", "date", "id", "bd", "value", "1990-01-01")
			return page(nk(label("for", "bd"), "LabelText", "", textNode("Birthday")), f), f
		}},
		{name: "textarea with placeholder", want: "Notes", build: func(*controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("textarea", "textbox", "Notes", "placeholder", "Notes")
			f.Value, f.Text, f.RawText = "call me at 555", "call me at 555", "call me at 555"
			return page(f), f
		}},
		{name: "select with aria-label", want: "Country", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := nk(field("select", "combobox", "Country", "aria-label", "Country"), "combobox", "Country",
				nk(cn("option", "selected", ""), "option", "United States"), nk(cn("option"), "option", "Canada"))
			f.Value = "United States"
			return page(f), f
		}},
		{name: "aria-labelledby to plain text", want: "Card holder", build: func(*controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "textbox", "Card holder", "type", "text", "aria-labelledby", "h")
			return page(span("Card holder", "id", "h"), f), f
		}},
		{name: "aria-labelledby to two references", want: "Billing Card holder", build: func(*controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "textbox", "Billing Card holder", "type", "text", "aria-labelledby", "a b")
			return page(span("Billing", "id", "a"), span("Card holder", "id", "b"), f), f
		}},
		{name: "label under an unmask inside a masked form", want: "Coupon", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "textbox", "Coupon", "type", "text", "id", "cp")
			form := nk(cn("form", "class", "masked"), "form", "", nk(label("for", "cp", "class", "unmasked"), "LabelText", "", textNode("Coupon")), f)
			return page(form), f
		}},
		{name: "aria-label inside a masked form", want: "Zip", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "textbox", "Zip", "type", "text", "aria-label", "Zip")
			return page(nk(cn("form", "class", "masked"), "form", "", f)), f
		}},

		// --- The name could repeat the control's own content.
		{name: "wrapping label whose name embeds the value", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "spinbutton", "Quantity 5", "type", "number", "value", "5")
			f.Value = "5"
			return page(nk(label(), "LabelText", "", textNode("Quantity"), f)), f
		}},
		{name: "aria-label mirroring the value", build: solo(field("input", "textbox", "Ann Smith", "type", "text", "aria-label", "Ann Smith", "value", "Ann Smith"))},
		{name: "current value differs from the value attribute", build: func(*controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "textbox", "Name: Ann Smith", "type", "text", "title", "Name: Ann Smith", "value", "")
			f.Value = "Ann Smith"
			return page(f), f
		}},
		{name: "textarea text in the name", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("textarea", "textbox", "Message my secret")
			f.RawText = "my secret"
			return page(nk(label(), "LabelText", "", textNode("Message"), f)), f
		}},
		{name: "select inside its label, name embeds the selected option", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := nk(field("select", "combobox", "State Ohio"), "combobox", "State Ohio",
				nk(cn("option"), "option", "Iowa"), nk(cn("option", "selected", ""), "option", "Ohio"))
			return page(nk(label(), "LabelText", "", textNode("State"), f)), f
		}},
		{name: "select named by an option and nothing else", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := nk(field("select", "combobox", "Alabama"), "combobox", "Alabama", nk(cn("option"), "option", "Alabama"))
			return page(f), f
		}},
		{name: "name with no source on the control", build: solo(field("input", "textbox", "290 Main St", "type", "text"))},
		{name: "aria-label that is not the name", build: solo(field("input", "textbox", "Jane Doe", "type", "text", "aria-label", "Name"))},
		{name: "placeholder that is not the name", build: solo(field("input", "textbox", "Jane Doe", "type", "text", "placeholder", "Name"))},

		// --- aria-labelledby can point anywhere.
		{name: "aria-labelledby to itself", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "textbox", "Amount 40", "type", "text", "id", "amt", "aria-labelledby", "lbl amt", "value", "40")
			return page(span("Amount", "id", "lbl"), f), f
		}},
		{name: "aria-labelledby to its own ancestor", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "textbox", "Tip", "type", "text", "aria-labelledby", "row")
			return page(nk(cn("div", "id", "row"), "generic", "", textNode("Tip"), f)), f
		}},
		{name: "aria-labelledby to a masked element", build: func(*controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "textbox", "Ann Smith", "type", "text", "aria-labelledby", "who")
			return page(span("Ann Smith", "id", "who", "class", "masked"), f), f
		}},
		{name: "aria-labelledby to a blocked element", build: func(*controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "textbox", "Ann Smith", "type", "text", "aria-labelledby", "who")
			return page(span("Ann Smith", "id", "who", "class", "blocked"), f), f
		}},
		{name: "aria-labelledby to an element with a masked descendant", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "textbox", "Card ending 4242", "type", "text", "aria-labelledby", "h")
			return page(nk(cn("div", "id", "h"), "generic", "", textNode("Card ending"), span("4242", "class", "masked")), f), f
		}},
		{name: "aria-labelledby to an element holding another input", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "textbox", "Flash 3 times", "type", "text", "aria-labelledby", "h")
			other := cn("input", "type", "text", "value", "3")
			other.Role, other.Value = "textbox", "3"
			return page(nk(cn("div", "id", "h"), "generic", "", textNode("Flash"), other, textNode("times")), f), f
		}},
		{name: "aria-labelledby to contenteditable text", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "textbox", "my draft", "type", "text", "aria-labelledby", "ed")
			return page(nk(cn("div", "id", "ed", "contenteditable", "true"), "generic", "", textNode("my draft")), f), f
		}},
		{name: "aria-labelledby to a role=textbox element", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "textbox", "typed", "type", "text", "aria-labelledby", "tb")
			return page(nk(cn("div", "id", "tb", "role", "textbox"), "textbox", "", textNode("typed")), f), f
		}},
		{name: "aria-labelledby to a missing id", build: solo(field("input", "textbox", "Name", "type", "text", "aria-labelledby", "gone", "aria-label", "Name"))},
		{name: "aria-labelledby to a duplicated id, one copy masked", build: func(*controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "textbox", "Owner", "type", "text", "aria-labelledby", "o")
			return page(span("Owner", "id", "o"), span("Ann Smith", "id", "o", "class", "masked"), f), f
		}},

		// --- Labels are content too when they are masked or hold another control.
		{name: "label inside a masked form", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "textbox", "Coupon", "type", "text", "id", "cp")
			form := nk(cn("form", "class", "masked"), "form", "", nk(label("for", "cp"), "LabelText", "", textNode("Coupon")), f)
			return page(form), f
		}},
		{name: "label for, itself masked", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "textbox", "Ann's card", "type", "text", "id", "x")
			return page(nk(label("for", "x", "class", "masked"), "LabelText", "", textNode("Ann's card")), f), f
		}},
		{name: "wrapping label with a masked descendant", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "checkbox", "Ship to Ann Smith", "type", "checkbox")
			return page(nk(label(), "LabelText", "", f, textNode("Ship to"), span("Ann Smith", "class", "masked"))), f
		}},
		{name: "wrapping label holding another input", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "checkbox", "Remind me in 3 days", "type", "checkbox")
			other := cn("input", "type", "text", "value", "3")
			other.Role, other.Value = "textbox", "3"
			return page(nk(label(), "LabelText", "", f, textNode("Remind me in"), other, textNode("days"))), f
		}},
		{name: "label for holding a select", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "textbox", "Amount USD", "type", "text", "id", "amt")
			sel := nk(cn("select"), "combobox", "", nk(cn("option", "selected", ""), "option", "USD"))
			return page(nk(label("for", "amt"), "LabelText", "", textNode("Amount"), sel), f), f
		}},

		// --- Not a control whose name labels it.
		{name: "input type=submit", build: solo(field("input", "button", "Pay $40.00", "type", "submit", "value", "Pay $40.00"))},
		{name: "input type=button with aria-label", build: solo(field("input", "button", "Go", "type", "button", "aria-label", "Go"))},
		{name: "input type=reset", build: solo(field("input", "button", "Reset", "type", "reset"))},
		{name: "input type=image", build: solo(field("input", "button", "Ann's avatar", "type", "image", "alt", "Ann's avatar"))},
		{name: "input type=file", build: solo(field("input", "button", "Upload", "type", "file", "aria-label", "Upload"))},
		{name: "input type=hidden", build: solo(field("input", "", "token", "type", "hidden", "aria-label", "token"))},
		{name: "role=button on a checkbox", build: solo(field("input", "button", "Accept", "type", "checkbox", "role", "button", "aria-label", "Accept"))},
		{name: "computed role is not a control role", build: solo(field("input", "link", "Profile", "type", "text", "aria-label", "Profile"))},
		{name: "contenteditable div", build: solo(field("div", "textbox", "Comment", "contenteditable", "true", "aria-label", "Comment"))},
		{name: "role=textbox div", build: solo(field("div", "textbox", "Comment", "role", "textbox", "aria-label", "Comment"))},
		{name: "output", build: func(c *controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("output", "status", "Total", "id", "t")
			return page(nk(label("for", "t"), "LabelText", "", textNode("Total")), f), f
		}},
		{name: "button", build: solo(field("button", "button", "Ann Smith", "aria-label", "Ann Smith"))},
		{name: "empty name never falls back to text", build: func(*controlCase) (*sightmap.ComponentNode, *sightmap.ComponentNode) {
			f := field("input", "textbox", "", "type", "text")
			f.Text = "typed text"
			return page(f), f
		}},
	}

	m := loadControlCorpus(t)
	for i := range cases {
		c := &cases[i]
		t.Run(c.name, func(t *testing.T) {
			root, f := c.build(c)
			res := m.Match(root, "")
			cm := res[f]
			if cm == nil || cm.Privacy != "mask" {
				t.Fatalf("field match = %+v; want a masked Field", cm)
			}
			got, ok := cm.Property("label")
			switch {
			case c.want == "" && ok:
				t.Errorf("label = %q; want withheld", got.Value)
			case c.want == "" && !slices.Contains(cm.Withheld, "label"):
				t.Errorf("Withheld = %v; want label in it", cm.Withheld)
			case c.want != "" && (!ok || got.Value != c.want):
				t.Errorf("label = %q, %v; want %q", got.Value, ok, c.want)
			}
			// Content reads stay withheld whatever the name does: the value, the
			// attribute an aria-label name came from, and raw text.
			for _, p := range []string{"value", "aria", "raw"} {
				if v, ok := cm.Property(p); ok {
					t.Errorf("%s = %q; want withheld", p, v.Value)
				}
			}
		})
	}
}

// block withholds a control's name too: nothing of a blocked element is
// captured.
func TestControlNameUnderBlock(t *testing.T) {
	m := loadControlCorpus(t)
	f := cn("input", "data-fb", "", "type", "text", "aria-label", "Card number")
	f.Role, f.Name = "textbox", "Card number"
	root := cn("body")
	root.Children = []*sightmap.ComponentNode{f}
	cm := match.NewMatcher(m.corpus).Match(root, "")[f]
	if cm == nil || cm.Privacy != "block" {
		t.Fatalf("match = %+v; want a blocked BlockedField", cm)
	}
	if v, ok := cm.Property("label"); ok {
		t.Errorf("label = %q; want withheld under block", v.Value)
	}
	if !slices.Contains(cm.Withheld, "label") {
		t.Errorf("Withheld = %v; want label", cm.Withheld)
	}
}

type controlCorpus struct {
	*match.Matcher
	corpus *sightmap.Corpus
}

func loadControlCorpus(t *testing.T) controlCorpus {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte(controlNameCorpus), 0o644); err != nil {
		t.Fatal(err)
	}
	corpus, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return controlCorpus{match.NewMatcher(corpus), corpus}
}
