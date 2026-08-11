package coverage

import (
	"github.com/sightmap/sightmap/go/sightmap"
	"testing"
)

func TestResultStatus(t *testing.T) {
	cases := []struct {
		name      string
		total, t3 int
		wantOK    bool
		wantEmpty bool
		wantMark  string
	}{
		{"clean pass", 10, 0, true, false, "✓"},
		{"orphans", 10, 3, false, false, "✗"},
		{"blank page", 0, 0, false, true, "∅"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Result{Total: tc.total, T3: tc.t3}
			if r.OK() != tc.wantOK {
				t.Errorf("OK() = %v, want %v", r.OK(), tc.wantOK)
			}
			if r.Empty() != tc.wantEmpty {
				t.Errorf("Empty() = %v, want %v", r.Empty(), tc.wantEmpty)
			}
			if r.Mark() != tc.wantMark {
				t.Errorf("Mark() = %q, want %q", r.Mark(), tc.wantMark)
			}
		})
	}
}

func TestCountInteractive(t *testing.T) {
	tree := &sightmap.ComponentNode{
		Id: "root", Role: "document", IsVisible: true,
		Children: []*sightmap.ComponentNode{
			{Id: "btn", Role: "button", IsInteractive: true, IsVisible: true},
			{Id: "hidden-btn", Role: "button", IsInteractive: true, IsVisible: false},
			{Id: "ignored-btn", Role: "button", IsInteractive: true, IsVisible: true, IsIgnored: true},
			{Id: "text", Role: "StaticText", IsVisible: true},
		},
	}

	if got := CountInteractive(tree, true); got != 1 {
		t.Errorf("CountInteractive(visibleOnly=true) = %d, want 1 (hidden + ignored excluded)", got)
	}
	if got := CountInteractive(tree, false); got != 3 {
		t.Errorf("CountInteractive(visibleOnly=false) = %d, want 3", got)
	}
	if got := CountInteractive(nil, true); got != 0 {
		t.Errorf("CountInteractive(nil) = %d, want 0", got)
	}
}
