package viewset

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// TestViewSlotsExcludePathNormalization pins ViewSlots' candidate-exclusion
// contract: when the caller passes a non-empty excludePath, the matching
// capture is dropped from the returned "others" set regardless of how that
// path is SPELLED relative to the on-disk spelling that discovery emits.
//
// discovery (Find) returns e.Path as filepath.Join(sightmapDir, "snapshots",
// ...) — cleaned, and absolute iff sightmapDir is absolute. The capture-novelty
// command passes the operator's verbatim candidate argument (with .tree.json
// trimmed) as excludePath. Before the fix the comparison was a raw
// filepath.ToSlash string equality, so any lexical mismatch (absolute vs
// relative, or a "./" prefix) silently failed to exclude the candidate: it was
// unioned INTO "others", ComputeNovelty diffed it against itself, and a
// genuinely NOVEL capture was reported redundant with ComparedTo inflated by
// one.
//
// One capture is placed on disk; for every spelling pair below the candidate
// MUST be excluded, leaving len(others) == 0.
func TestViewSlotsExcludePathNormalization(t *testing.T) {
	// Build a single capture under <root>/.sightmap/snapshots/home/.
	root := t.TempDir()
	sdirRel := filepath.Join(root, ".sightmap")
	snapDir := filepath.Join(sdirRel, "snapshots", "home")
	if err := os.MkdirAll(snapDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	const stamp = "20260722T140000Z.snap"
	snap := filepath.Join(snapDir, stamp)
	if err := os.WriteFile(snap, []byte("[View: home]\nroute: /\n"), 0o644); err != nil {
		t.Fatalf("write snap: %v", err)
	}
	// A root-only WebArea tree matches nothing and yields an empty fingerprint,
	// so SlotsForCapture succeeds and len(others) reflects exclusion alone.
	if err := os.WriteFile(snap+".tree.json", []byte(`{"role":"WebArea"}`), 0o644); err != nil {
		t.Fatalf("write tree: %v", err)
	}

	// chdir into root so a relative ".sightmap" resolves against it, mirroring
	// the documented `sightmap capture-novelty FILE.snap` from the repo root.
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(origWd)
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	// After chdir, capture the resolved cwd so absolute spellings are built
	// from the SAME base os.Getwd reports to filepath.Abs inside ViewSlots.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd after chdir: %v", err)
	}

	relSdir := ".sightmap"
	absSdir := filepath.Join(cwd, ".sightmap")
	relExclude := filepath.Join(relSdir, "snapshots", "home", stamp)
	absExclude := filepath.Join(absSdir, "snapshots", "home", stamp)
	dotExclude := "./" + relExclude

	corpus := &sightmap.Corpus{} // matches nothing; fingerprint is empty

	cases := []struct {
		name        string
		sightmapDir string
		exclude     string
	}{
		{"rel-dir rel-exclude (baseline)", relSdir, relExclude},
		{"rel-dir abs-exclude", relSdir, absExclude},
		{"rel-dir dot-exclude", relSdir, dotExclude},
		{"abs-dir rel-exclude", absSdir, relExclude},
		{"abs-dir abs-exclude (baseline)", absSdir, absExclude},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			others := ViewSlots(c.sightmapDir, "home", corpus, c.exclude)
			if len(others) != 0 {
				t.Errorf("exclusion failed for exclude=%q vs sightmapDir=%q: len(others)=%d, want 0 (candidate must be dropped from others)",
					c.exclude, c.sightmapDir, len(others))
			}
		})
	}
}

// TestViewSlotsEmptyExcludeNeverExcludes pins the live-capture gate contract:
// an empty excludePath disables exclusion entirely, so every capture in the
// view's set is unioned into "others". capture-novelty is the only caller that
// passes a non-empty excludePath; Gate always passes "".
func TestViewSlotsEmptyExcludeNeverExcludes(t *testing.T) {
	root := t.TempDir()
	snapDir := filepath.Join(root, "snapshots", "home")
	if err := os.MkdirAll(snapDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, stamp := range []string{"20260722T090000Z.snap", "20260722T140000Z.snap"} {
		snap := filepath.Join(snapDir, stamp)
		if err := os.WriteFile(snap, []byte("[View: home]\nroute: /\n"), 0o644); err != nil {
			t.Fatalf("write snap: %v", err)
		}
		if err := os.WriteFile(snap+".tree.json", []byte(`{"role":"WebArea"}`), 0o644); err != nil {
			t.Fatalf("write tree: %v", err)
		}
	}
	others := ViewSlots(root, "home", &sightmap.Corpus{}, "")
	if len(others) != 2 {
		t.Errorf("empty exclude excluded a capture: len(others)=%d, want 2 (all captures retained)", len(others))
	}
}
