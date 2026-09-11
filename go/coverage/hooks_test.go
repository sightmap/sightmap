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
			name:   "single trailing digit is not treated as a counter",
			el:     &sightmap.Element{Tag: "div", Id: "step2"},
			want:   []string{"#step2"},
			absent: []string{`[id^="step"]`},
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

func TestSelectorCandidates_OtherDataAttrOrderIsDeterministic(t *testing.T) {
	// Two non-testid/component data-* attrs tie at the same score. Attrs is a
	// map, so without sorting the keys first, their relative order (and thus
	// which one survives the top-4 cap) would vary run to run.
	el := &sightmap.Element{
		Tag: "a",
		Attrs: map[string]string{
			"data-target-selection-name": "Home",
			"data-row-key":               "row-Home",
		},
	}
	first := SelectorCandidates(el)
	for i := 0; i < 20; i++ {
		got := SelectorCandidates(el)
		if strings.Join(got, ",") != strings.Join(first, ",") {
			t.Fatalf("run %d: got %v, want %v (order must be stable across calls)", i, got, first)
		}
	}
}

func TestViewScopedMatchCount(t *testing.T) {
	globals := map[string]bool{"GlobalHeader": true, "AppShell": true}
	n1 := &sightmap.ComponentNode{Id: "1"}
	n2 := &sightmap.ComponentNode{Id: "2"}
	n3 := &sightmap.ComponentNode{Id: "3"}

	// All matches are global → 0 view-scoped (the chrome-only case).
	chromeOnly := Matches{
		n1: {Name: "GlobalHeader"},
		n2: {Name: "AppShell"},
	}
	if got := ViewScopedMatchCount(chromeOnly, globals); got != 0 {
		t.Errorf("chrome-only: got %d, want 0", got)
	}

	// One view-scoped match → count 1.
	mixed := Matches{
		n1: {Name: "GlobalHeader"},
		n2: {Name: "ListSearchBox"},
		n3: {Name: "AppShell"},
	}
	if got := ViewScopedMatchCount(mixed, globals); got != 1 {
		t.Errorf("mixed: got %d, want 1", got)
	}
}

func TestHrefSuffix(t *testing.T) {
	// hrefSuffix's documented contract: returns "" for hrefs whose tail is
	// dynamic (numeric or hashed) or that carry no usable path; otherwise the
	// portable trailing segment "/seg".
	tests := []struct {
		name string
		href string
		want string
	}{
		// Numeric tails — per-instance record ids, dropped per the contract.
		{"single digit", "/s/detail/1", ""},
		{"two digit", "/orders/42", ""},
		{"three digit", "/users/123", ""},
		{"four digit", "/items/9999", ""},
		{"date day last segment", "/blog/2023/06/15", ""},
		{"numeric with query stripped", "/orders/42?print=1", ""},
		{"numeric with hash stripped", "/orders/42#notes", ""},
		{"numeric with trailing slash", "/orders/42/", ""},
		{"long numeric id", "/lightning/r/Account/001A000001abcdefg", ""},

		// Hashed / machine-generated tails — dropped by looksHashed.
		{"aura id", "/x/1:1;a", ""},
		{"hex run", "/u/abc123def456", ""},

		// Stable, non-numeric tails — kept.
		{"word segment", "/lightning/page/home", "/home"},
		{"versioned route", "/products/v2", "/v2"},
		{"api version", "/api/v2/users", "/users"},
		{"mixed alpha slug", "/p/some-detail", "/some-detail"},
		{"word with query", "/home?foo=1", "/home"},
		{"word with hash", "/about#top", "/about"},

		// No usable path.
		{"empty", "", ""},
		{"root", "/", ""},
		{"bare segment no slash", "home", ""},
		{"query only no slash", "?foo=1", ""},
		{"hash only no slash", "#top", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hrefSuffix(tt.href); got != tt.want {
				t.Errorf("hrefSuffix(%q) = %q, want %q", tt.href, got, tt.want)
			}
		})
	}
}

func TestIsNumeric(t *testing.T) {
	trues := []string{"0", "1", "9", "42", "123", "9999", "2023", "15", "0012300", "0123456789"}
	for _, s := range trues {
		if !isNumeric(s) {
			t.Errorf("isNumeric(%q) = false, want true", s)
		}
	}
	falses := []string{"", "v2", "item42", "1:1;a", "abc", "12-34", "12.5", "1a", "a1", "0x1", "001A000001", "0012300abcdefg"}
	for _, s := range falses {
		if isNumeric(s) {
			t.Errorf("isNumeric(%q) = true, want false", s)
		}
	}
}

func TestSelectorCandidatesRecordLinkNoNumericHint(t *testing.T) {
	// A hook-poor link to a numeric record id: no id/class/data-* compete with
	// the href suffix. Before the fix, hrefSuffix returned "/42" and the only
	// candidate surfaced was the per-instance a[href$="/42"]. Ensure no numeric
	// suffix candidate appears, and that a non-numeric adjacent link still
	// offers its portable suffix.
	numericEl := &sightmap.Element{Tag: "a", Attrs: map[string]string{"href": "/orders/42"}}
	for _, c := range SelectorCandidates(numericEl) {
		if c == `a[href$="/42"]` {
			t.Errorf("numeric per-instance suffix surfaced as stable hint: %v", SelectorCandidates(numericEl))
		}
	}
	stableEl := &sightmap.Element{Tag: "a", Attrs: map[string]string{"href": "/orders/v2"}}
	if !contains(SelectorCandidates(stableEl), `a[href$="/v2"]`) {
		t.Errorf("stable versioned suffix dropped: %v", SelectorCandidates(stableEl))
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
