package skillgen

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Result reports what Write or CheckTree found across a set of files.
type Result struct {
	// Written (Write only): files created or updated on disk.
	Written []string
	// Unchanged: files whose reconciled content matched what was already there.
	Unchanged []string
	// Stale (CheckTree only): files that exist but whose reconciled content
	// would differ from what's on disk — run generate to fix.
	Stale []Divergence
	// Missing (CheckTree only): files the plan produces that don't exist yet.
	Missing []string
	// Unmanaged: an existing file has no recognizable managed-region markers,
	// so it was left alone rather than risk clobbering hand-written content.
	Unmanaged []string
}

// OK reports whether a CheckTree Result found no drift at all.
func (r Result) OK() bool {
	return len(r.Stale) == 0 && len(r.Missing) == 0 && len(r.Unmanaged) == 0
}

// Divergence is one file whose on-disk content differs from what the plan
// would write.
type Divergence struct {
	Path      string
	Want, Got string
}

// region is one named managed region located in a file: the byte range
// strictly between its begin and end markers (markers themselves excluded).
type region struct {
	name       string
	start, end int
}

// findRegions returns every named managed region in b, in document order.
// A region whose begin marker has no matching end marker is not returned
// (the scan stops there) — reconcile treats that as "no markers" for
// whatever fresh/existing it belongs to.
func findRegions(b []byte) []region {
	const beginPrefix = "<!-- sightmap:begin "
	const beginSuffix = " -->"
	var regions []region
	pos := 0
	for {
		bi := bytes.Index(b[pos:], []byte(beginPrefix))
		if bi < 0 {
			return regions
		}
		bi += pos
		lineEnd := bytes.IndexByte(b[bi:], '\n')
		if lineEnd < 0 {
			return regions
		}
		beginLine := string(b[bi : bi+lineEnd])
		name, ok := strings.CutPrefix(beginLine, beginPrefix)
		if !ok {
			return regions
		}
		name, ok = strings.CutSuffix(name, beginSuffix)
		if !ok {
			return regions
		}
		contentStart := bi + lineEnd + 1
		endMarker := endRegion(name)
		ei := bytes.Index(b[contentStart:], []byte(endMarker))
		if ei < 0 {
			return regions
		}
		contentEnd := contentStart + ei
		regions = append(regions, region{name: name, start: contentStart, end: contentEnd})
		pos = contentEnd + len(endMarker)
	}
}

// reconcile splices fresh's named managed regions into existing's shell, so
// an author's edits to everything else — the H1, the summary, the
// "How to get to it" and "Gotchas" prose an area file sandwiches between its
// regions — survive regeneration untouched.
//
// A file with no managed regions in its fresh render (SKILL.md,
// references/README.md — see render.go) is fully generated: overwritten
// whenever it differs, no splicing. A file whose fresh render declares a
// region the existing content doesn't have (hand-authored, predating this
// generator, or a region renamed by a newer skillgen version) is reported
// unmanaged and left untouched entirely, rather than guess at a partial
// splice that might destroy prose the marker convention exists to protect.
func reconcile(fresh, existing []byte) (result []byte, changed bool, unmanaged bool) {
	if len(existing) == 0 {
		return fresh, true, false
	}
	freshRegions := findRegions(fresh)
	if len(freshRegions) == 0 {
		changed = !bytes.Equal(fresh, existing)
		return fresh, changed, false
	}

	existingByName := map[string]region{}
	for _, r := range findRegions(existing) {
		existingByName[r.name] = r
	}
	for _, fr := range freshRegions {
		if _, ok := existingByName[fr.name]; !ok {
			return existing, false, true
		}
	}

	var buf bytes.Buffer
	prevEnd := 0
	for _, fr := range freshRegions {
		er := existingByName[fr.name]
		buf.Write(existing[prevEnd:er.start]) // author-owned text before this region
		buf.Write(fresh[fr.start:fr.end])     // the fresh render of the region itself
		prevEnd = er.end
	}
	buf.Write(existing[prevEnd:]) // author-owned text after the last region
	result = buf.Bytes()
	changed = !bytes.Equal(result, existing)
	return result, changed, false
}

// Write reconciles every file against root and writes anything that changed,
// atomically (temp file + rename). It never deletes: a stale area left over
// from a corpus change is reported by CheckTree, not removed by Write, so a
// rename doesn't silently drop a file a human might still be reading.
func Write(root string, files []File) (Result, error) {
	var res Result
	for _, f := range files {
		full := filepath.Join(root, f.Path)
		existing, err := os.ReadFile(full)
		if err != nil && !os.IsNotExist(err) {
			return res, fmt.Errorf("skillgen: read %s: %w", f.Path, err)
		}
		final, changed, unmanaged := reconcile(f.Content, existing)
		switch {
		case unmanaged:
			res.Unmanaged = append(res.Unmanaged, f.Path)
		case !changed:
			res.Unchanged = append(res.Unchanged, f.Path)
		default:
			if err := writeAtomic(full, final); err != nil {
				return res, fmt.Errorf("skillgen: write %s: %w", f.Path, err)
			}
			res.Written = append(res.Written, f.Path)
		}
	}
	return res, nil
}

// CheckTree reconciles every file against root without writing anything,
// reporting drift. This is `sightmap skills generate --check`: a clean result
// (Result.OK()) means the committed output matches what generate would
// produce right now.
func CheckTree(root string, files []File) (Result, error) {
	var res Result
	for _, f := range files {
		full := filepath.Join(root, f.Path)
		existing, err := os.ReadFile(full)
		if err != nil {
			if os.IsNotExist(err) {
				res.Missing = append(res.Missing, f.Path)
				continue
			}
			return res, fmt.Errorf("skillgen: read %s: %w", f.Path, err)
		}
		final, changed, unmanaged := reconcile(f.Content, existing)
		switch {
		case unmanaged:
			res.Unmanaged = append(res.Unmanaged, f.Path)
		case changed:
			res.Stale = append(res.Stale, Divergence{Path: f.Path, Want: string(final), Got: string(existing)})
		default:
			res.Unchanged = append(res.Unchanged, f.Path)
		}
	}
	return res, nil
}

// writeAtomic writes data to path via a temp file + rename in the same
// directory, so a reader never observes a partially written file.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
