package main

import (
	"testing"
)

func TestSnapshotPath(t *testing.T) {
	tests := []struct {
		name         string
		sightmapDir  string
		viewName     string
		snapshotName string
		want         string
	}{
		{
			name:         "base snapshot",
			sightmapDir:  ".sightmap",
			viewName:     "app-home",
			snapshotName: "base",
			want:         ".sightmap/snapshots/app-home/base.snap",
		},
		{
			name:         "named snapshot",
			sightmapDir:  ".sightmap",
			viewName:     "product-list",
			snapshotName: "with-filters-open",
			want:         ".sightmap/snapshots/product-list/with-filters-open.snap",
		},
		{
			name:         "custom sightmap dir",
			sightmapDir:  "custom/.sightmap",
			viewName:     "checkout",
			snapshotName: "base",
			want:         "custom/.sightmap/snapshots/checkout/base.snap",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := snapshotPath(tt.sightmapDir, tt.viewName, tt.snapshotName)
			if got != tt.want {
				t.Errorf("snapshotPath(%q, %q, %q) = %q, want %q",
					tt.sightmapDir, tt.viewName, tt.snapshotName, got, tt.want)
			}
		})
	}
}

func TestSnapshotTreePath(t *testing.T) {
	tests := []struct {
		name     string
		snapPath string
		want     string
	}{
		{
			name:     "base snapshot",
			snapPath: ".sightmap/snapshots/app-home/base.snap",
			want:     ".sightmap/snapshots/app-home/base.snap.tree.json",
		},
		{
			name:     "named snapshot",
			snapPath: ".sightmap/snapshots/product-list/with-filters.snap",
			want:     ".sightmap/snapshots/product-list/with-filters.snap.tree.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := snapshotTreePath(tt.snapPath)
			if got != tt.want {
				t.Errorf("snapshotTreePath(%q) = %q, want %q", tt.snapPath, got, tt.want)
			}
		})
	}
}

func TestParseSnapshotPath(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantView  string
		wantStamp string
		wantOk    bool
	}{
		{
			name:      "view set capture",
			path:      ".sightmap/snapshots/app-home/20260607T193000Z.snap",
			wantView:  "app-home",
			wantStamp: "20260607T193000Z",
			wantOk:    true,
		},
		{
			name:      "view set tree json",
			path:      ".sightmap/snapshots/checkout/20260607T193000Z.snap.tree.json",
			wantView:  "checkout",
			wantStamp: "20260607T193000Z",
			wantOk:    true,
		},
		{
			name:      "legacy single-file (name as stamp)",
			path:      ".sightmap/snapshots/product-list/base.snap",
			wantView:  "product-list",
			wantStamp: "base",
			wantOk:    true,
		},
		{
			name:      "old 3-segment state collapses to (view, stamp)",
			path:      ".sightmap/snapshots/app-home/apron/20260607T193000Z.snap",
			wantView:  "app-home",
			wantStamp: "20260607T193000Z",
			wantOk:    true,
		},
		{
			name:      "legacy flat (no stamp)",
			path:      "app-home.snap",
			wantView:  "app-home",
			wantStamp: "",
			wantOk:    true,
		},
		{
			name:      "invalid path",
			path:      "random.txt",
			wantView:  "",
			wantStamp: "",
			wantOk:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotView, gotStamp, gotOk := parseSnapshotPath(tt.path)
			if gotView != tt.wantView || gotStamp != tt.wantStamp || gotOk != tt.wantOk {
				t.Errorf("parseSnapshotPath(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.path, gotView, gotStamp, gotOk, tt.wantView, tt.wantStamp, tt.wantOk)
			}
		})
	}
}
