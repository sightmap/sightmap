package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResolveLintTreeFiles covers how lint decides which snapshot tree files to
// reconcile against — in particular the default auto-discovery that fixes the
// [multi-instance-no-property] false positive on single-match components.
func TestResolveLintTreeFiles(t *testing.T) {
	t.Run("explicit --snapshot normalizes extension, not auto", func(t *testing.T) {
		files, auto, err := resolveLintTreeFiles(t.TempDir(), "shots/home.snap", false)
		if err != nil {
			t.Fatal(err)
		}
		if auto {
			t.Error("explicit --snapshot should not be an auto run")
		}
		if len(files) != 1 || files[0] != "shots/home.snap.tree.json" {
			t.Errorf("files = %v, want [shots/home.snap.tree.json]", files)
		}
	})

	t.Run("default with no snapshots dir: empty, auto, no error", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), ".sightmap") // exists, but no snapshots/ subdir
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		files, auto, err := resolveLintTreeFiles(dir, "", false)
		if err != nil {
			t.Fatalf("missing snapshots/ dir must not error: %v", err)
		}
		if !auto {
			t.Error("no snapshot flags should be an auto run")
		}
		if len(files) != 0 {
			t.Errorf("files = %v, want none", files)
		}
	})

	t.Run("default auto-discovers a captured snapshot", func(t *testing.T) {
		dir := seedSnapshot(t)
		files, auto, err := resolveLintTreeFiles(dir, "", false)
		if err != nil {
			t.Fatal(err)
		}
		if !auto {
			t.Error("no snapshot flags should be an auto run")
		}
		if len(files) != 1 {
			t.Fatalf("files = %v, want the one seeded snapshot", files)
		}
	})

	t.Run("--all-snapshots discovers snapshots, not auto", func(t *testing.T) {
		dir := seedSnapshot(t)
		files, auto, err := resolveLintTreeFiles(dir, "", true)
		if err != nil {
			t.Fatal(err)
		}
		if auto {
			t.Error("explicit --all-snapshots should not be flagged auto")
		}
		if len(files) != 1 {
			t.Fatalf("files = %v, want the one seeded snapshot", files)
		}
	})
}

// seedSnapshot makes a <tmp>/.sightmap dir with one snapshots/*.snap.tree.json.
func seedSnapshot(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), ".sightmap")
	snaps := filepath.Join(dir, "snapshots")
	if err := os.MkdirAll(snaps, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snaps, "home.snap.tree.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
