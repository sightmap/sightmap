package main

import (
	"strings"
	"testing"
)

func TestOfflineDivergenceHints(t *testing.T) {
	cases := []struct {
		name     string
		selector string
		wantSub  string
	}{
		{"id attribute selector", `[id^="context-menu"]`, "`id` is a dedicated node field"},
		{"class attribute selector", `[class*="lucide"]`, "use a `.class` selector"},
		{"unparseable", `[[[not valid`, "could not parse"},
		{"generic fallback", `input[placeholder="Search"]`, "offline node model may not capture"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hints := offlineDivergenceHints(tc.selector)
			if len(hints) == 0 {
				t.Fatalf("expected at least one hint for %q", tc.selector)
			}
			joined := strings.Join(hints, "\n")
			if !strings.Contains(joined, tc.wantSub) {
				t.Errorf("hints for %q = %v, want one mentioning %q", tc.selector, hints, tc.wantSub)
			}
		})
	}
}

// TestOfflineDivergenceHintsDedup verifies a selector referencing the same trap
// twice yields a single hint.
func TestOfflineDivergenceHintsDedup(t *testing.T) {
	hints := offlineDivergenceHints(`[id^="a"] [id$="b"]`)
	n := 0
	for _, h := range hints {
		if strings.Contains(h, "`id` is a dedicated node field") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("expected the id hint exactly once, got %d (%v)", n, hints)
	}
}
