package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/sightmap/sightmap/go/skills"
)

func runSkills(args []string) error {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "usage: sightmap skills install [--target DIR]\n"+
			"       sightmap skills generate [--sightmap-dir DIR] [--out DIR] [--name NAME] [--check]\n")
		return nil
	}
	switch args[0] {
	case "install":
		return runSkillsInstall(args[1:])
	case "generate":
		return runSkillsGenerate(args[1:])
	default:
		return fmt.Errorf("skills: unknown subcommand %q", args[0])
	}
}

func runSkillsInstall(args []string) error {
	fset := flag.NewFlagSet("skills install", flag.ContinueOnError)
	home, _ := os.UserHomeDir()
	defaultTarget := filepath.Join(home, ".agents", "skills")
	targetFlag := fset.String("target", defaultTarget, "Directory to install skills into (each skill becomes a subdirectory)")
	if err := fset.Parse(args); err != nil {
		return err
	}
	baseTarget := *targetFlag

	// Each top-level directory in the embedded FS is one skill.
	entries, err := fs.ReadDir(skills.FS, ".")
	if err != nil {
		return fmt.Errorf("skills install: read embedded skills: %w", err)
	}

	var installed []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillName := e.Name()
		target := filepath.Join(baseTarget, skillName)

		// Remove old install, then re-extract.
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("skills install: remove old %s: %w", skillName, err)
		}

		if err := fs.WalkDir(skills.FS, skillName, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, relErr := filepath.Rel(skillName, path)
			if relErr != nil {
				return relErr
			}
			dest := filepath.Join(target, rel)
			if d.IsDir() {
				return os.MkdirAll(dest, 0o755)
			}
			data, readErr := skills.FS.ReadFile(path)
			if readErr != nil {
				return fmt.Errorf("read embedded %s: %w", path, readErr)
			}
			if mkErr := os.MkdirAll(filepath.Dir(dest), 0o755); mkErr != nil {
				return mkErr
			}
			return os.WriteFile(dest, data, 0o644)
		}); err != nil {
			return fmt.Errorf("skills install: extract %s: %w", skillName, err)
		}
		installed = append(installed, skillName)
	}

	fmt.Printf("installed %d sightmap skill(s) → %s\n", len(installed), baseTarget)
	for _, s := range installed {
		fmt.Printf("  %s\n", s)
	}
	fmt.Printf("  version: %s\n", Version)
	return nil
}
