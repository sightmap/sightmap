// Package skillgen converts a loaded sightmap Corpus into an agent skill
// router: a small SKILL.md that routes a cold, natural-language prompt to one
// reference file per app area, teaching that area's vocabulary and the
// `sightmap browser` commands that drive it.
//
// This is a new offline subsystem in the sense go/ARCHITECTURE.md uses the
// word: it imports sightmap and nothing else, and never imports browser — it
// only emits text naming `sightmap browser` commands, which is a string, not
// a dependency edge. It does not live inside go/sightmap (the corpus model,
// which stays a self-contained leaf with no rendering or filesystem
// concerns) or go/skills (the go:embed mirror of the static, hand-written
// skills, which is a different thing entirely).
package skillgen

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sightmap/sightmap/go/sightmap"
)

// Options configures Plan and the rendered prose.
type Options struct {
	// SkillName is the generated skill's directory name, e.g. "verify-fullstory".
	SkillName string
	// AppTitle is the human app name used in prose, e.g. "Fullstory".
	AppTitle string
	// DescriptionBudget caps the router frontmatter description in runes.
	// <= 0 uses descriptionBudgetDefault.
	DescriptionBudget int
}

// Area is one .sightmap/*.yaml file's worth of corpus data — the unit of
// progressive disclosure: exactly one reference file per area. Everything
// here is attributed by ComponentDef/ViewDef/RequestDef/MessageDef.SourceFile
// (the corpus file basename each was declared in), which the sightmap loader
// stamps at compile time.
type Area struct {
	// Slug is the corpus file basename (no extension), e.g. "library-ui" —
	// also the reference file's name.
	Slug string
	// Title is the derived human title, e.g. "Library UI".
	Title string

	// Views declared in this file, in corpus (declaration) order.
	Views []sightmap.ViewDef
	// Components declared at this file's root (global components), in
	// declaration/flatten order — parent immediately before its children.
	// A view's own components live on that ViewDef, not here.
	Components []sightmap.ComponentDef
	// Requests declared at this file's root.
	Requests []sightmap.RequestDef
	// Messages declared in this file (messages are always file-root).
	Messages []sightmap.MessageDef
	// Memory is this file's memory: entries (Corpus.FileMemory, filtered).
	Memory []string
}

// Router is the complete plan for one generated skill: the areas it will
// produce one reference file per, plus the options that shape its prose.
type Router struct {
	Opts  Options
	Areas []Area
}

// Plan buckets a loaded corpus into one Area per source file and returns the
// Router ready for Render. Areas are sorted by Slug for deterministic output.
// A corpus entity with no SourceFile (only possible for an in-memory Corpus
// built by hand rather than sightmap.Load) is skipped rather than crashing.
func Plan(c *sightmap.Corpus, opts Options) (*Router, error) {
	byFile := map[string]*Area{}
	get := func(slug string) *Area {
		a, ok := byFile[slug]
		if !ok {
			a = &Area{Slug: slug, Title: AreaTitle(slug)}
			byFile[slug] = a
		}
		return a
	}

	for _, v := range c.Views {
		if v.SourceFile == "" {
			continue
		}
		a := get(v.SourceFile)
		a.Views = append(a.Views, v)
	}
	for _, comp := range c.GlobalComponents {
		if comp.SourceFile == "" {
			continue
		}
		a := get(comp.SourceFile)
		a.Components = append(a.Components, comp)
	}
	for _, r := range c.Requests {
		if r.SourceFile == "" {
			continue
		}
		a := get(r.SourceFile)
		a.Requests = append(a.Requests, r)
	}
	for _, m := range c.Messages {
		if m.SourceFile == "" {
			continue
		}
		a := get(m.SourceFile)
		a.Messages = append(a.Messages, m)
	}
	for _, fm := range c.FileMemory {
		a := get(fm.SourceFile)
		a.Memory = append(a.Memory, fm.Memory...)
	}

	slugs := make([]string, 0, len(byFile))
	for slug := range byFile {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)

	areas := make([]Area, 0, len(slugs))
	for _, slug := range slugs {
		areas = append(areas, *byFile[slug])
	}

	return &Router{Opts: opts, Areas: areas}, nil
}

// Files builds the file set for generating against an existing tree at root:
// it renders every area file, peeks at how each would reconcile against
// what's already on disk (without writing), and builds the index and router
// from THOSE reconciled contents rather than fresh derivations — so an
// author's hand-edit to an area's summary blockquote (an author-owned region;
// see reconcile.go) is what the index shows on the next run, not the
// generator's original guess. Render alone doesn't do this: it has no root to
// read, which is exactly right for a from-scratch generation or a test.
//
// The returned files are still fresh (unreconciled) content — Write or
// CheckTree do the actual reconciliation against root, a second time, so this
// function has no side effects and Files+Write / Files+CheckTree stays
// consistent with using Render+Write / Render+CheckTree directly against an
// empty root.
func Files(root string, router *Router) ([]File, error) {
	summaries := make(map[string]string, len(router.Areas))
	areaFiles := make([]File, 0, len(router.Areas))
	for _, a := range router.Areas {
		path := "references/areas/" + a.Slug + ".md"
		fresh := renderArea(a)
		existing, err := os.ReadFile(filepath.Join(root, path))
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("skillgen: read %s: %w", path, err)
		}
		final, _, unmanaged := reconcile(fresh, existing)
		if unmanaged {
			// Can't safely reconcile; read back whatever summary the
			// existing (untouched) file already has instead of the fresh
			// seed's derived one.
			summaries[a.Slug] = extractSummary(existing)
		} else {
			summaries[a.Slug] = extractSummary(final)
		}
		areaFiles = append(areaFiles, File{Path: path, Content: fresh})
	}

	files := make([]File, 0, len(areaFiles)+2)
	files = append(files, File{Path: "SKILL.md", Content: renderSkill(router)})
	files = append(files, File{Path: "references/README.md", Content: renderIndex(router, summaries)})
	files = append(files, areaFiles...)
	return files, nil
}

// extractSummary reads back an area file's summary: the blockquote line
// ("> ...") immediately following the H1, before any blank-line-separated
// content. Returns "" if the file is empty or doesn't start that way (an
// unrecognized/hand-authored shape — the caller falls back to the derived
// summary in that case, same as a brand-new area).
func extractSummary(content []byte) string {
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		if i == 0 {
			continue // the H1
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if rest, ok := strings.CutPrefix(trimmed, "> "); ok {
			return rest
		}
		return "" // first non-blank line after the H1 isn't a blockquote
	}
	return ""
}
