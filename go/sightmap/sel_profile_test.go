package sightmap_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

const (
	notChain   = sightmap.CodeSelectorNotChainEvaluable
	notProfile = sightmap.CodeSelectorNotInProfile
)

// TestParseProfileSelector_Classification maps each construct to the code the
// capture-baseline profile gives it ("" = accepted).
func TestParseProfileSelector_Classification(t *testing.T) {
	cases := []struct {
		sel, code string
	}{
		// Accepted.
		{`input`, ""},
		{`my-widget`, ""},
		{`#card`, ""},
		{`.card`, ""},
		{`div.a.b#c`, ""},
		{`[data-x]`, ""},
		{`[data-x=a]`, ""},
		{`[data-x="a b"]`, ""},
		{`[data-x='a']`, ""},
		{`[data-x=""]`, ""},
		{`[a~=w][a|=en][a^=p][a$=s][a*=m]`, ""},
		{`[class*="Card"][class*="link"]`, ""},
		{`[class]`, ""},
		{`[class~=x]`, ""},
		{`input:not([type=hidden])`, ""},
		{`input:not([type=hidden]):not(.public)`, ""},
		{`li:not(#x)`, ""},
		{`a:not(span)`, ""},
		{`.a .b`, ""},
		{`.a > .b`, ""},
		{`.a>.b`, ""},
		{`.a  >  .b   .c`, ""},
		{`.a > * > input`, ""},
		{`*`, ""},
		{`.a\:b`, ""},
		{`.café`, ""},
		{`  .padded  `, ""},
		{`.-x`, ""},
		{`.a .b .c .d .e .f .g .h`, ""}, // 8 compounds

		// Not chain-evaluable.
		{`section:has(input)`, notChain},
		{`.a + .b`, notChain},
		{`.a ~ .b`, notChain},
		{`li:first-child`, notChain},
		{`li:nth-child(2)`, notChain},
		{`p:empty`, notChain},
		{`a:hover`, notChain},
		{`input:checked`, notChain},
		{`input:not(:checked)`, notChain},
		{`div:not(:has(.x))`, notChain},

		// Not in the profile.
		{``, notProfile},
		{`   `, notProfile},
		{`:is(.a, .b)`, notProfile},
		{`:where(.a)`, notProfile},
		{`:root`, notProfile},
		{`:not(.a, .b)`, notProfile},
		{`:not(.a.b)`, notProfile},
		{`:not(.a .b)`, notProfile},
		{`:not(*)`, notProfile},
		{`:not()`, notProfile},
		{`:NOT(.a)`, notProfile},
		{`p::before`, notProfile},
		{`[a=b i]`, notProfile},
		{`[a=b s]`, notProfile},
		{`[ a=b]`, notProfile},
		{`[a = b]`, notProfile},
		{`[xlink|href]`, notProfile},
		{`[A=b]`, notProfile},
		{`DIV`, notProfile},
		{`[a^=""]`, notProfile},
		{`[a*=""]`, notProfile},
		{`[a~=""]`, notProfile},
		{`[a~="p q"]`, notProfile},
		{`[class=x]`, notProfile},
		{`[class^=x]`, notProfile},
		{`[x=1]`, notProfile},
		{`.a, .b`, notProfile},
		{`> .a`, notProfile},
		{`.a >`, notProfile},
		{`*.a`, notProfile},
		{`.a*`, notProfile},
		{`[x]div`, notProfile},
		{`:not(.x)div`, notProfile},
		{`#a#b`, notProfile},
		{`.\31 0`, notProfile},
		{`.a` + "\t" + `.b`, notProfile},
		{`.a` + " " + `.b`, notProfile},
		{`.--x`, notProfile},
		{`.2col`, notProfile},
		{`.a .b .c .d .e .f .g .h .i`, notProfile}, // 9 compounds
		{strings.Repeat(".a", 520), notProfile},   // 1,040 bytes
	}
	for _, c := range cases {
		t.Run(c.sel, func(t *testing.T) {
			_, err := sightmap.ParseProfileSelector(c.sel, sightmap.ProfileCaptureBaseline)
			got := ""
			if err != nil {
				var pe *sightmap.ProfileError
				if !errors.As(err, &pe) {
					t.Fatalf("error is not a *ProfileError: %v", err)
				}
				got = pe.Code
			}
			if got != c.code {
				t.Errorf("code = %q, want %q (err: %v)", got, c.code, err)
			}
		})
	}
}

func TestParseProfileSelector_Compounds(t *testing.T) {
	ps, err := sightmap.ParseProfileSelector(`form.checkout > * input[type=password]:not([data-x])`, sightmap.ProfileCaptureBaseline)
	if err != nil {
		t.Fatal(err)
	}
	wantText := []string{`form.checkout`, `*`, `input[type=password]:not([data-x])`}
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
		`*`:                sightmap.ConstraintNone,
		`.a *`:             sightmap.ConstraintNone,
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
		`input`, `.a > .b .c`, `[data-x="a b"]`, `input:not([type=hidden])`, `.a > * > input`,
		`section:has(input)`, `.a + .b`, `[a=b i]`, `:is(.a)`, `.café`, `.a\:b`, `#x`, `*`,
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
