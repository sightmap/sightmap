package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sightmap/sightmap/go/sightmap"
	"github.com/sightmap/sightmap/go/skillgen"
)

// runSkillsGenerate implements `sightmap skills generate`: compile the
// .sightmap/ corpus into an agent skill router — a small SKILL.md that routes
// a natural-language prompt ("verify the fix on the left nav") to one
// reference file per app area, teaching that area's vocabulary and the
// `sightmap browser` commands that drive it. See skillgen for the model.
//
// Validation errors are reported (to stderr) but do not block generation: a
// reference file for the areas that DO validate is strictly better than none,
// and unlike `stats` there is no numeric contract at stake here.
func runSkillsGenerate(args []string) error {
	fs := flag.NewFlagSet("skills generate", flag.ContinueOnError)
	sightmapDir := fs.String("sightmap-dir", "", "path to the .sightmap/ dir (default: nearest one at or above cwd)")
	out := fs.String("out", ".claude/skills", "skills root to write into (or check)")
	name := fs.String("name", "", "generated skill directory name (default: verify-<app>, derived from config.yaml or the corpus directory)")
	check := fs.Bool("check", false, "write nothing; report drift against --out and exit non-zero if stale")

	positional, err := parseFlagsAroundArgs(fs, args)
	if err != nil {
		return err
	}
	if len(positional) > 0 {
		return fmt.Errorf("skills generate: unexpected argument %q", positional[0])
	}

	dir := *sightmapDir
	if dir == "" {
		dir = findSightmapDir(cwd())
		if dir == "" {
			return fmt.Errorf("skills generate: no .sightmap/ directory found at or above %s — run 'sightmap init' first", cwd())
		}
	}

	corpus, err := sightmap.Load(dir)
	if err != nil {
		return fmt.Errorf("skills generate: load %s: %w", dir, err)
	}
	for _, ve := range sightmap.Validate(corpus) {
		fmt.Fprintf(os.Stderr, "sightmap skills generate: warning: %s\n", ve.Error())
	}

	skillName := *name
	if skillName == "" {
		skillName = deriveSkillName(dir)
	}
	appTitle := skillgen.AreaTitle(strings.TrimPrefix(skillName, "verify-"))

	router, err := skillgen.Plan(corpus, skillgen.Options{SkillName: skillName, AppTitle: appTitle})
	if err != nil {
		return fmt.Errorf("skills generate: plan: %w", err)
	}

	root := filepath.Join(*out, skillName)

	// Files (not Render) so a hand-edited area summary already on disk at
	// root reaches the regenerated index — see skillgen.Files.
	files, err := skillgen.Files(root, router)
	if err != nil {
		return fmt.Errorf("skills generate: render: %w", err)
	}

	if *check {
		res, err := skillgen.CheckTree(root, files)
		if err != nil {
			return fmt.Errorf("skills generate: check: %w", err)
		}
		return reportCheck(root, res)
	}

	res, err := skillgen.Write(root, files)
	if err != nil {
		return fmt.Errorf("skills generate: write: %w", err)
	}
	fmt.Fprintf(os.Stderr, "sightmap skills generate: %d area(s) → %s (%d written, %d unchanged",
		len(router.Areas), root, len(res.Written), len(res.Unchanged))
	if len(res.Unmanaged) > 0 {
		fmt.Fprintf(os.Stderr, ", %d unmanaged — left alone", len(res.Unmanaged))
	}
	fmt.Fprintf(os.Stderr, ")\n")
	for _, p := range res.Unmanaged {
		fmt.Fprintf(os.Stderr, "  unmanaged: %s (no recognizable sightmap markers — not overwritten)\n", p)
	}
	return nil
}

// deriveSkillName picks the generated skill's directory name when --name is
// not given: SiteConfig.Name (an existing tooling-only field from
// .sightmap/config.yaml) when set, else the basename of the directory
// containing .sightmap/, prefixed "verify-".
func deriveSkillName(sightmapDir string) string {
	cfg := sightmap.LoadConfig(sightmapDir)
	app := cfg.Name
	if app == "" {
		app = filepath.Base(filepath.Dir(sightmapDir))
	}
	return "verify-" + app
}

// reportCheck prints CheckTree's result in the --check output format and
// returns a non-nil error (causing a nonzero exit) iff anything is out of
// date. It never writes.
func reportCheck(root string, res skillgen.Result) error {
	total := len(res.Unchanged) + len(res.Stale) + len(res.Missing)
	for _, d := range res.Stale {
		fmt.Fprintf(os.Stderr, "stale     %s\n", filepath.Join(root, d.Path))
	}
	for _, p := range res.Missing {
		fmt.Fprintf(os.Stderr, "missing   %s\n", filepath.Join(root, p))
	}
	for _, p := range res.Unmanaged {
		fmt.Fprintf(os.Stderr, "unmanaged %s  (no generated marker — left alone; move it to references/manual/)\n", filepath.Join(root, p))
	}
	if res.OK() {
		fmt.Fprintf(os.Stderr, "%d file(s) up to date.\n", total)
		return nil
	}
	stale := len(res.Stale) + len(res.Missing)
	fmt.Fprintf(os.Stderr, "\n%d file(s) out of date. Run 'sightmap skills generate' and commit the result.\n", stale)
	return fmt.Errorf("skills generate --check: %d file(s) out of date", stale)
}
