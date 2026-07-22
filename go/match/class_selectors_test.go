package match_test

import (
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/match"
)

// TestClassAttrSelectors verifies that attribute selectors targeting "class"
// work correctly now that classes are duplicated into Attrs["class"].
func TestClassAttrSelectors(t *testing.T) {
	tests := []struct {
		name     string
		selector string
		classes  []string
		want     bool
	}{
		// Partial match
		{
			name:     "[class*=\"primary\"] matches btn-primary",
			selector: `[class*="primary"]`,
			classes:  []string{"btn", "btn-primary"},
			want:     true,
		},
		{
			name:     "[class*=\"primary\"] does not match secondary",
			selector: `[class*="primary"]`,
			classes:  []string{"btn", "btn-secondary"},
			want:     false,
		},
		// Prefix match
		{
			name:     "[class^=\"btn-\"] matches btn-primary",
			selector: `[class^="btn-"]`,
			classes:  []string{"btn-primary"},
			want:     true,
		},
		{
			name:     "[class^=\"btn-\"] does not match primary-btn",
			selector: `[class^="btn-"]`,
			classes:  []string{"primary-btn"},
			want:     false,
		},
		// Suffix match
		{
			name:     "[class$=\"-primary\"] matches btn-primary",
			selector: `[class$="-primary"]`,
			classes:  []string{"btn-primary"},
			want:     true,
		},
		// Word match (space-separated)
		{
			name:     "[class~=\"active\"] matches exact word",
			selector: `[class~="active"]`,
			classes:  []string{"btn", "active"},
			want:     true,
		},
		{
			name:     "[class~=\"active\"] does not match partial",
			selector: `[class~="active"]`,
			classes:  []string{"btn", "inactive"},
			want:     false,
		},
		// Exact match
		{
			name:     "[class=\"btn\"] matches single class",
			selector: `[class="btn"]`,
			classes:  []string{"btn"},
			want:     true,
		},
		{
			name:     "[class=\"btn\"] does not match multiple classes",
			selector: `[class="btn"]`,
			classes:  []string{"btn", "btn-primary"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &comps.ComponentNode{
				Id:   "test-node",
				Role: "button",
				Selector: &comps.SelectorPart{
					Tag:     "button",
					Classes: tt.classes,
					Attrs:   map[string]string{"class": strings.Join(tt.classes, " ")},
				},
			}

			comp := match.SightmapComponent{
				Name:      "TestButton",
				Selectors: []string{tt.selector},
			}

			matches := match.ApplySightmap(node, []match.SightmapComponent{comp})
			got := len(matches) > 0

			if got != tt.want {
				t.Errorf("selector %q with classes %v: got match=%v, want %v",
					tt.selector, tt.classes, got, tt.want)
			}
		})
	}
}
