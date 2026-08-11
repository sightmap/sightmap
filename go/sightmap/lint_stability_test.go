package sightmap_test

import (
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

func TestLint_StabilitySuppression(t *testing.T) {
	tests := []struct {
		name     string
		comp     sightmap.ComponentDef
		wantRule string // empty if no warning expected
	}{
		{
			name: "unstable suppresses multi-instance warning",
			comp: sightmap.ComponentDef{
				Name:       "VolatileButton",
				Selectors:  []string{"button.css-module-xyz"},
				Stability:  "unstable",
				Properties: nil,
			},
			wantRule: "", // no warning
		},
		{
			name: "uncertain does not suppress multi-instance warning",
			comp: sightmap.ComponentDef{
				Name:       "UncertainButton",
				Selectors:  []string{"button.btn-primary"},
				Stability:  "uncertain",
				Properties: nil,
			},
			wantRule: "multi-instance-no-property", // warning expected
		},
		{
			name: "no stability triggers warning",
			comp: sightmap.ComponentDef{
				Name:       "GenericButton",
				Selectors:  []string{"button.btn"},
				Stability:  "",
				Properties: nil,
			},
			wantRule: "multi-instance-no-property", // warning expected
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := corpusFrom([]sightmap.ComponentDef{tt.comp}, nil)
			warnings := sightmap.Lint(c)

			foundRule := ""
			for _, w := range warnings {
				if w.Component == tt.comp.Name {
					foundRule = w.Rule
					break
				}
			}

			if tt.wantRule == "" {
				if foundRule != "" {
					t.Errorf("expected no warning, got rule %q: %v", foundRule, warnings)
				}
			} else {
				if foundRule != tt.wantRule {
					t.Errorf("expected rule %q, got %q; warnings: %v", tt.wantRule, foundRule, warnings)
				}
			}
		})
	}
}
