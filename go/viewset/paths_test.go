package viewset

import (
	"testing"
)

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
			got := TreePath(tt.snapPath)
			if got != tt.want {
				t.Errorf("TreePath(%q) = %q, want %q", tt.snapPath, got, tt.want)
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
			gotView, gotStamp, gotOk := ParsePath(tt.path)
			if gotView != tt.wantView || gotStamp != tt.wantStamp || gotOk != tt.wantOk {
				t.Errorf("ParsePath(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.path, gotView, gotStamp, gotOk, tt.wantView, tt.wantStamp, tt.wantOk)
			}
		})
	}
}
