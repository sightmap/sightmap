package sightmap_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

const notProfile = sightmap.CodeSelectorNotInProfile

// TestParseProfileSelector_Classification maps each construct to the code the
// capture-baseline profile gives it ("" = accepted).
func TestParseProfileSelector_Classification(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "spec", "conformance", "036-selector-classification.fixture", "cases.json"))
	if err != nil {
		t.Fatalf("read classification cases: %v", err)
	}
	var cases []struct {
		Selector string `json:"selector"`
		Code     string `json:"code"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("parse classification cases: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("no classification cases")
	}
	for _, c := range cases {
		t.Run(c.Selector, func(t *testing.T) {
			_, err := sightmap.ParseProfileSelector(c.Selector, sightmap.ProfileCaptureBaseline)
			got := ""
			if err != nil {
				var pe *sightmap.ProfileError
				if !errors.As(err, &pe) {
					t.Fatalf("error is not a *ProfileError: %v", err)
				}
				got = pe.Code
			}
			if got != c.Code {
				t.Errorf("code = %q, want %q (err: %v)", got, c.Code, err)
			}
		})
	}
}

func TestParseProfileSelector_DigitLeadingValue(t *testing.T) {
	for _, c := range []struct {
		sel  string
		pos  int
		want string
	}{
		{`[tabindex=0]`, 10, `[tabindex="0"]`},
		{`div[data-index=12]`, 15, `[data-index="12"]`},
		{`a:not([x=1])`, 9, `[x="1"]`},
	} {
		_, err := sightmap.ParseProfileSelector(c.sel, sightmap.ProfileCaptureBaseline)
		var pe *sightmap.ProfileError
		if !errors.As(err, &pe) {
			t.Fatalf("%s: want *ProfileError, got %v", c.sel, err)
		}
		if pe.Code != notProfile || pe.Pos != c.pos {
			t.Errorf("%s: code %q pos %d, want %q pos %d", c.sel, pe.Code, pe.Pos, notProfile, c.pos)
		}
		if !strings.Contains(pe.Msg, "quote it") || !strings.Contains(pe.Msg, c.want) {
			t.Errorf("%s: message %q should advise quoting as %s", c.sel, pe.Msg, c.want)
		}
	}
}

func TestParseProfileSelector_SVGNames(t *testing.T) {
	ps, err := sightmap.ParseProfileSelector(`svg linearGradient[gradientUnits=userSpaceOnUse]:not(feBlend)`, sightmap.ProfileCaptureBaseline)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps.Compounds) != 2 || ps.Compounds[0].Text != "svg" || ps.Compounds[0].Constraint != sightmap.ConstraintType {
		t.Fatalf("compounds = %+v", ps.Compounds)
	}
	sub := ps.Subject()
	if sub.Text != `linearGradient[gradientUnits=userSpaceOnUse]:not(feBlend)` || sub.Constraint != sightmap.ConstraintSpecific {
		t.Errorf("subject = %+v", sub)
	}
	if len(sub.Attrs) != 1 || sub.Attrs[0] != "gradientUnits" {
		t.Errorf("attrs = %v, want [gradientUnits]", sub.Attrs)
	}
	for sel, pos := range map[string]int{`myWidget`: 2, `a:not(myWidget)`: 8, `[dataFoo=x]`: 5, `div[ViewBox=x]`: 4, `ClipPath`: 0} {
		_, err := sightmap.ParseProfileSelector(sel, sightmap.ProfileCaptureBaseline)
		var pe *sightmap.ProfileError
		if !errors.As(err, &pe) || pe.Pos != pos || !strings.Contains(pe.Msg, "lowercase") {
			t.Errorf("%s: err = %v, want lowercase rejection at %d", sel, err, pos)
		}
	}
}

func TestParseProfileSelector_Compounds(t *testing.T) {
	ps, err := sightmap.ParseProfileSelector(`form.checkout > fieldset input[type=password]:not([data-x])`, sightmap.ProfileCaptureBaseline)
	if err != nil {
		t.Fatal(err)
	}
	wantText := []string{`form.checkout`, `fieldset`, `input[type=password]:not([data-x])`}
	wantComb := []string{"", ">", " "}
	if len(ps.Compounds) != len(wantText) {
		t.Fatalf("got %d compounds, want %d", len(ps.Compounds), len(wantText))
	}
	for i, c := range ps.Compounds {
		if c.Text != wantText[i] || ps.Combinators[i] != wantComb[i] {
			t.Errorf("compound %d = %q via %q, want %q via %q", i, c.Text, ps.Combinators[i], wantText[i], wantComb[i])
		}
	}
	if got := ps.Subject().Attrs; len(got) != 2 || got[0] != "type" || got[1] != "data-x" {
		t.Errorf("subject attrs = %v, want [type data-x]", got)
	}
}

func TestParseProfileSelector_SubjectConstraint(t *testing.T) {
	for sel, want := range map[string]sightmap.Constraint{
		`:not(.x)`:         sightmap.ConstraintNone,
		`.a > :not(.b)`:    sightmap.ConstraintNone,
		`body`:             sightmap.ConstraintType,
		`div:not(.x)`:      sightmap.ConstraintType,
		`.card`:            sightmap.ConstraintSpecific,
		`input[type=text]`: sightmap.ConstraintSpecific,
		`#id`:              sightmap.ConstraintSpecific,
	} {
		ps, err := sightmap.ParseProfileSelector(sel, sightmap.ProfileCaptureBaseline)
		if err != nil {
			t.Fatalf("%s: %v", sel, err)
		}
		if got := ps.Subject().Constraint; got != want {
			t.Errorf("%s: subject constraint = %d, want %d", sel, got, want)
		}
	}
}

func TestParseProfileSelector_UnknownProfile(t *testing.T) {
	if _, err := sightmap.ParseProfileSelector(`.a`, "modern"); err == nil {
		t.Error("an unknown profile must be an error")
	}
}

// FuzzParseProfileSelector: the profile parser never panics, everything it
// accepts the general parser accepts too, and re-joining the compounds with
// their combinators parses back to the same compounds.
func FuzzParseProfileSelector(f *testing.F) {
	for _, s := range []string{
		`input`, `.a > .b .c`, `[data-x="a b"]`, `input:not([type=hidden])`, `.a > .b > input`,
		`section:has(input)`, `.a + .b`, `[a=b i]`, `:is(.a)`, `.café`, `.a\:b`, `#x`, `*`,
		`clipPath`, `svg linearGradient[gradientUnits=x]`, `:not(feBlend)`, `[viewBox="0 0 1 1"]`, `[tabindex=0]`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, sel string) {
		ps, err := sightmap.ParseProfileSelector(sel, sightmap.ProfileCaptureBaseline)
		if err != nil {
			return
		}
		if _, err := sightmap.ParseSightmapSelector(strings.TrimSpace(sel)); err != nil {
			t.Fatalf("profile accepts %q but ParseSightmapSelector rejects it: %v", sel, err)
		}
		var b strings.Builder
		for i, c := range ps.Compounds {
			if i > 0 {
				b.WriteString(" " + strings.TrimSpace(ps.Combinators[i]) + " ")
			}
			b.WriteString(c.Text)
		}
		again, err := sightmap.ParseProfileSelector(b.String(), sightmap.ProfileCaptureBaseline)
		if err != nil {
			t.Fatalf("rejoined %q (from %q) no longer parses: %v", b.String(), sel, err)
		}
		if len(again.Compounds) != len(ps.Compounds) {
			t.Fatalf("rejoined %q has %d compounds, want %d", b.String(), len(again.Compounds), len(ps.Compounds))
		}
		for i := range ps.Compounds {
			if again.Compounds[i].Text != ps.Compounds[i].Text {
				t.Fatalf("rejoined compound %d = %q, want %q", i, again.Compounds[i].Text, ps.Compounds[i].Text)
			}
		}
	})
}
