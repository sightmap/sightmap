package coverage

import (
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

func TestSelectorCandidates_Nil(t *testing.T) {
	if got := SelectorCandidates(nil); got != nil {
		t.Errorf("SelectorCandidates(nil) = %v, want nil", got)
	}
}

func TestSelectorCandidates(t *testing.T) {
	tests := []struct {
		name    string
		el      *sightmap.Element
		wantTop string   // expected first (highest-ranked) candidate
		want    []string // candidates that must all be present (order-insensitive)
		absent  []string // candidates that must NOT appear
	}{
		{
			name:    "data-testid wins",
			el:      &sightmap.Element{Tag: "button", Attrs: map[string]string{"data-testid": "save-btn"}},
			wantTop: `[data-testid="save-btn"]`,
		},
		{
			name:    "data-component strips version suffix",
			el:      &sightmap.Element{Tag: "div", Attrs: map[string]string{"data-component": "forceChatterFeed:v1.2.3-abc"}},
			wantTop: `[data-component^="forceChatterFeed"]`,
		},
		{
			name:    "custom-element tag is a strong hook",
			el:      &sightmap.Element{Tag: "one-appnav"},
			wantTop: "one-appnav",
		},
		{
			name:   "design-system class combines with tag; hashed class dropped",
			el:     &sightmap.Element{Tag: "article", Classes: []string{"forceBaseCard", "lwc-3k9f2h1", "css-1a2b3c4d"}},
			want:   []string{"article.forceBaseCard"},
			absent: []string{"article.lwc-3k9f2h1", "article.css-1a2b3c4d"},
		},
		{
			name: "stable id yields #id",
			el:   &sightmap.Element{Tag: "input", Id: "account-name"},
			want: []string{"#account-name"},
		},
		{
			name:   "aura-style id is hashed and dropped",
			el:     &sightmap.Element{Tag: "div", Id: "1:1;a"},
			absent: []string{"#1:1;a"},
		},
		{
			name: "trailing-counter id prefers [id^=] prefix form",
			el:   &sightmap.Element{Tag: "button", Id: "combobox-button-15"},
			want: []string{`[id^="combobox-button"]`},
		},
		{
			name: "other stable data-* attr is surfaced",
			el:   &sightmap.Element{Tag: "a", Attrs: map[string]string{"data-target-selection-name": "Home"}},
			want: []string{`[data-target-selection-name="Home"]`},
		},
		{
			name: "form control name",
			el:   &sightmap.Element{Tag: "input", Attrs: map[string]string{"name": "AccountName"}},
			want: []string{`input[name="AccountName"]`},
		},
		{
			name: "link href suffix is portable form",
			el:   &sightmap.Element{Tag: "a", Attrs: map[string]string{"href": "https://x.example.com/lightning/page/home?foo=1"}},
			want: []string{`a[href$="/home"]`},
		},
		{
			name:   "dynamic href tail is dropped",
			el:     &sightmap.Element{Tag: "a", Attrs: map[string]string{"href": "/lightning/r/Account/001A000001abcdefg"}},
			absent: []string{`a[href$="/001A000001abcdefg"]`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SelectorCandidates(tt.el)
			if tt.wantTop != "" {
				if len(got) == 0 || got[0] != tt.wantTop {
					t.Errorf("top candidate = %v, want %q first", got, tt.wantTop)
				}
			}
			for _, w := range tt.want {
				if !contains(got, w) {
					t.Errorf("candidates %v missing %q", got, w)
				}
			}
			for _, a := range tt.absent {
				if contains(got, a) {
					t.Errorf("candidates %v should not contain %q", got, a)
				}
			}
			if len(got) > 4 {
				t.Errorf("returned %d candidates, want <= 4: %v", len(got), got)
			}
		})
	}
}

func TestSelectorCandidates_RankingDataAttrOverClass(t *testing.T) {
	// data-* must rank above a design-system class, but the class must still be
	// present (data-testid is one input, not an override).
	el := &sightmap.Element{
		Tag:     "article",
		Classes: []string{"forceBaseCard"},
		Attrs:   map[string]string{"data-testid": "card"},
	}
	got := SelectorCandidates(el)
	if len(got) < 2 {
		t.Fatalf("want at least 2 candidates, got %v", got)
	}
	if got[0] != `[data-testid="card"]` {
		t.Errorf("top = %q, want data-testid first", got[0])
	}
	if !contains(got, "article.forceBaseCard") {
		t.Errorf("class candidate suppressed: %v", got)
	}
}

func TestNearestHookAncestor(t *testing.T) {
	leaf := &sightmap.ComponentNode{Id: "leaf", Role: "button", Element: &sightmap.Element{Tag: "button"}}
	mid := &sightmap.ComponentNode{Id: "mid", Element: &sightmap.Element{Tag: "div"}, Children: []*sightmap.ComponentNode{leaf}}
	nav := &sightmap.ComponentNode{Id: "nav", Element: &sightmap.Element{Tag: "one-appnav"}, Children: []*sightmap.ComponentNode{mid}}
	root := &sightmap.ComponentNode{Id: "root", Element: &sightmap.Element{Tag: "div"}, Children: []*sightmap.ComponentNode{nav}}
	pm := BuildParentMap(root)

	anc, sel := NearestHookAncestor(leaf, pm)
	if anc != nav {
		t.Errorf("ancestor = %v, want the one-appnav node", anc)
	}
	if sel != "one-appnav" {
		t.Errorf("ancestor selector = %q, want one-appnav", sel)
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s || strings.TrimSpace(x) == s {
			return true
		}
	}
	return false
}
