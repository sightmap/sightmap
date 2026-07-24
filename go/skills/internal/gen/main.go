// Command gen syncs the canonical skill corpora at the repository root
// (<repo>/skills/) into this Go package (<repo>/go/skills/) so they can be
// embedded via go:embed.
//
// go:embed cannot reach outside the Go module (the module root is <repo>/go)
// and cannot follow symlinks, so the canonical top-level skills/ directory
// cannot be embedded directly. Instead these committed copies are regenerated
// from the canonical source and checked in — the same "generate and commit"
// pattern used for the docs schema page. CI runs this generator and fails on
// any drift (see .github/workflows/go.yml).
//
// Run via `go generate ./skills/...` from the go/ module directory. Do not edit
// the generated go/skills/<name>/ copies by hand; edit <repo>/skills/<name>/
// and regenerate.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "gen-skills: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// This source file lives at <repo>/go/skills/internal/gen/main.go, so the
	// repo root is four directories up. Deriving paths from the source location
	// (rather than the working directory) keeps the generator correct no matter
	// where it is invoked from.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("cannot resolve source file location")
	}
	genDir := filepath.Dir(thisFile)              // .../go/skills/internal/gen
	pkgDir := filepath.Join(genDir, "..", "..")   // .../go/skills
	repoRoot := filepath.Join(pkgDir, "..", "..") // .../
	srcRoot := filepath.Join(repoRoot, "skills")  // canonical corpora

	entries, err := os.ReadDir(srcRoot)
	if err != nil {
		return fmt.Errorf("read canonical skills %s: %w", srcRoot, err)
	}

	// Names of the canonical skills. Everything else under pkgDir that looks like
	// a generated skill copy is an orphan (a skill renamed or dropped upstream)
	// and must be removed — otherwise a stale copy lingers and CI drift checks
	// would not catch it.
	canonical := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() {
			canonical[e.Name()] = true
		}
	}

	// pkgKeep are non-skill entries in pkgDir that the generator must never touch.
	pkgKeep := map[string]bool{"internal": true}

	pkgEntries, err := os.ReadDir(pkgDir)
	if err != nil {
		return fmt.Errorf("read package dir %s: %w", pkgDir, err)
	}
	for _, e := range pkgEntries {
		if !e.IsDir() || pkgKeep[e.Name()] || canonical[e.Name()] {
			continue
		}
		orphan := filepath.Join(pkgDir, e.Name())
		if err := os.RemoveAll(orphan); err != nil {
			return fmt.Errorf("remove orphaned skill %s: %w", orphan, err)
		}
		fmt.Printf("gen-skills: removed orphaned skill copy %s\n", e.Name())
	}

	var synced []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		src := filepath.Join(srcRoot, name)
		dst := filepath.Join(pkgDir, name)

		if err := os.RemoveAll(dst); err != nil {
			return fmt.Errorf("clean %s: %w", dst, err)
		}
		if err := copyTree(src, dst); err != nil {
			return fmt.Errorf("copy %s: %w", name, err)
		}
		synced = append(synced, name)
	}

	fmt.Printf("gen-skills: synced %d skill(s) into %s\n", len(synced), pkgDir)
	for _, s := range synced {
		fmt.Printf("  %s\n", s)
	}
	return nil
}

// copyTree copies the file tree rooted at src into dst, preserving relative
// layout. Regular files only — the skill corpora contain no symlinks or other
// irregular files (which go:embed would reject anyway).
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("refusing to copy irregular file %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
