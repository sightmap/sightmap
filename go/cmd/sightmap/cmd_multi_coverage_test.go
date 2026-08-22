package main

import (
	"reflect"
	"testing"
)

// col is a tiny viewColumn builder for the tests below.
func col(view string, current bool, counts map[string]int) viewColumn {
	return viewColumn{View: view, Snaps: 1, Counts: counts, Current: current}
}

// TestGlobalCandidatesAcrossViews_CurrentOnly is the #248 regression: a stale or
// renamed capture dir (Current=false) must not manufacture "appears in 2+ views"
// evidence, and callers are expected to pre-filter with currentColumns before
// calling globalCandidatesAcrossViews.
func TestGlobalCandidatesAcrossViews_CurrentOnly(t *testing.T) {
	tests := []struct {
		name    string
		cols    []viewColumn
		globals map[string]bool
		want    []globalCandidate
	}{
		{
			name: "stale duplicate of a current view yields no candidates",
			cols: []viewColumn{
				col("home", true, map[string]int{"Card": 3, "Header": 1}),
				col("views", false, map[string]int{"Card": 3, "Header": 1}), // stale dup of home
			},
			globals: map[string]bool{},
			want:    nil,
		},
		{
			name: "component in two current views is still promoted; stale view excluded from hits",
			cols: []viewColumn{
				col("home", true, map[string]int{"Nav": 1, "Card": 2}),
				col("search", true, map[string]int{"Nav": 1, "Result": 4}),
				col("legacy", false, map[string]int{"Nav": 1}), // stale: must not inflate Nav's hits
			},
			globals: map[string]bool{"AlreadyGlobal": true},
			want: []globalCandidate{
				{Name: "Nav", Hits: []viewHit{{View: "home", Count: 1}, {View: "search", Count: 1}}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := globalCandidatesAcrossViews(currentColumns(tt.cols), tt.globals)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCurrentAndStaleColumns_Partition(t *testing.T) {
	cols := []viewColumn{
		col("home", true, nil),
		col("views", false, nil),
		col("old-dir", false, nil),
	}

	current := currentColumns(cols)
	if len(current) != 1 || current[0].View != "home" {
		t.Errorf("currentColumns = %+v, want just [home]", current)
	}

	stale := staleColumns(cols)
	if len(stale) != 2 || stale[0].View != "views" || stale[1].View != "old-dir" {
		t.Errorf("staleColumns = %+v, want [views, old-dir]", stale)
	}
}

func TestViewColumnLabel(t *testing.T) {
	tests := []struct {
		name string
		col  viewColumn
		want string
	}{
		{"current single-capture", viewColumn{View: "home", Snaps: 1, Current: true}, "home"},
		{"current multi-capture", viewColumn{View: "home", Snaps: 3, Current: true}, "home·3"},
		{"stale single-capture", viewColumn{View: "views", Snaps: 1, Current: false}, "views*"},
		{"stale multi-capture", viewColumn{View: "views", Snaps: 2, Current: false}, "views·2*"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.col.label(); got != tt.want {
				t.Errorf("label() = %q, want %q", got, tt.want)
			}
		})
	}
}
