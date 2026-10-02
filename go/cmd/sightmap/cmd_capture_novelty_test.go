package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCaptureNoveltyExcludePathSpelling pins the end-to-end contract of
// `sightmap capture-novelty`: the candidate is excluded from "others" by
// IDENTITY, so a genuinely-novel capture is reported NOVEL regardless of how
// the operator spells the candidate path on the command line (absolute vs
// relative, or with a "./" prefix).
//
// Before the fix, ViewSlots excluded the candidate with a raw
// filepath.ToSlash string compare, so any lexical mismatch with the on-disk
// spelling discovery emitted left the candidate in "others":
// ComputeNovelty diffed it against itself, the verdict flipped NOVEL →
// REDUNDANT, and the "vs N existing capture(s)" header was inflated by one.
//
// The fixture is a two-capture set (one genuinely-novel candidate vs one stale
// capture), run through runCaptureNovelty across the full spelling matrix;
// every row must print NOVEL with ComparedTo == 1.
func TestCaptureNoveltyExcludePathSpelling(t *testing.T) {
	root := t.TempDir()
	sdir := filepath.Join(root, ".sightmap")

	// One view (route "/") declaring one Hero component keyed on ".hero" —
	// the candidate matches it; the stale capture does not.
	if err := os.MkdirAll(sdir, 0o755); err != nil {
		t.Fatalf("mkdir .sightmap: %v", err)
	}
	appYAML := `version: 1
views:
  - name: home
    route: /
    components:
      - name: Hero
        selector: '.hero'
`
	if err := os.WriteFile(filepath.Join(sdir, "app.yaml"), []byte(appYAML), 0o644); err != nil {
		t.Fatalf("write app.yaml: %v", err)
	}

	snapDir := filepath.Join(sdir, "snapshots", "home")
	if err := os.MkdirAll(snapDir, 0o755); err != nil {
		t.Fatalf("mkdir snapshots: %v", err)
	}

	// Stale capture: matches nothing — the union's empty baseline.
	stale := filepath.Join(snapDir, "20260722T090000Z.snap")
	writeNoveltyCapture(t, stale, `{"id":"1","role":"WebArea"}`)

	// Candidate: adds div.hero — matches Hero → genuinely novel vs the stale one.
	cand := filepath.Join(snapDir, "20260722T140000Z.snap")
	writeNoveltyCapture(t, cand, `{"id":"1","role":"WebArea","children":[{"id":"2","element":{"tag":"div","classes":["hero"]},"isVisible":true}]}`)

	// chdir into root so ".sightmap" is a genuinely relative dir relative to
	// cwd, exactly like running `sightmap capture-novelty FILE.snap` from the
	// repo root. filepath.Abs inside ViewSlots then resolves relative paths
	// against this same cwd.
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(origWd)
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd after chdir: %v", err)
	}

	// Pre-built spellings of the SAME on-disk candidate file.
	const relSdir = ".sightmap"
	absSdir := filepath.Join(cwd, ".sightmap")
	relCand := filepath.Join(relSdir, "snapshots", "home", "20260722T140000Z.snap")
	absCand := filepath.Join(absSdir, "snapshots", "home", "20260722T140000Z.snap")
	dotCand := "./" + relCand

	cases := []struct {
		name        string
		sightmapDir string
		candidate   string
	}{
		{"0 rel-dir rel-cand (baseline)", relSdir, relCand},
		{"1 rel-dir abs-cand", relSdir, absCand},
		{"2 rel-dir dot-cand", relSdir, dotCand},
		{"3 dot-dir rel-cand", "./" + relSdir, relCand},
		{"4 abs-dir rel-cand", absSdir, relCand},
		{"5 abs-dir abs-cand (baseline)", absSdir, absCand},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := captureStdout(func() error {
				return runCaptureNovelty([]string{"--sightmap-dir", c.sightmapDir, c.candidate})
			})
			if err != nil {
				t.Fatalf("runCaptureNovelty(--sightmap-dir=%q cand=%q): %v\n%s", c.sightmapDir, c.candidate, err, out)
			}
			// Candidate is genuinely novel vs the single stale capture: the
			// verdict must be NOVEL and the header must report exactly one
			// existing capture (the candidate correctly excluded).
			if !strings.Contains(out, "vs 1 existing capture(s)") {
				t.Errorf("expected ComparedTo=1 in output (candidate excluded), got:\n%s", out)
			}
			if !strings.Contains(out, "→ NOVEL: adds 1 component") {
				t.Errorf("expected NOVEL verdict, got:\n%s", out)
			}
			if strings.Contains(out, "redundant") {
				t.Errorf("verdict flipped to redundant — candidate was not excluded:\n%s", out)
			}
		})
	}
}

// writeNoveltyCapture writes a minimal .snap + .snap.tree.json pair: the .snap
// header carries the view and route (RouteOf/ComponentsForURL resolve the home
// view), and the tree JSON is the caller's capture structure.
func writeNoveltyCapture(t *testing.T, snap, treeJSON string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(snap), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(snap, []byte("[View: home]\nroute: /\n"), 0o644); err != nil {
		t.Fatalf("write snap: %v", err)
	}
	if err := os.WriteFile(snap+".tree.json", []byte(treeJSON), 0o644); err != nil {
		t.Fatalf("write tree: %v", err)
	}
}

// captureStdout runs fn with os.Stdout swapped to a pipe and returns the
// captured bytes (and fn's error). fmt.Printf in printNovelty writes to
// os.Stdout, so this is enough to intercept the capture-novelty report. Not
// safe for t.Parallel (mutates process-global os.Stdout), matching withStdin.
func captureStdout(fn func() error) (string, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	var buf strings.Builder
	done := make(chan struct{})
	go func() {
		io.Copy(&buf, r)
		r.Close()
		close(done)
	}()

	runErr := fn()
	w.Close()
	<-done
	return buf.String(), runErr
}
