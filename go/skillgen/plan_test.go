package skillgen

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// loadFixture writes files (name -> content) into a fresh temp .sightmap dir
// and loads it, failing the test on any error.
func loadFixture(t *testing.T, files map[string]string) *sightmap.Corpus {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	corpus, err := sightmap.Load(dir)
	if err != nil {
		t.Fatalf("sightmap.Load: %v", err)
	}
	return corpus
}

func TestPlan_groupsGlobalsByFile(t *testing.T) {
	corpus := loadFixture(t, map[string]string{
		"library-ui.yaml": `
version: 1
views:
  - name: LibraryView
    route: /library
    description: The object library
components:
  - name: LibraryTable
    selector: '[data-component="LibraryTable"]'
    description: The main listing table
    children:
      - name: LibrarySearchBar
        selector: input
        description: Search bar
`,
		"requests.yaml": `
version: 1
requests:
  - name: GetLibrary
    route: /api/library
    method: GET
`,
	})

	router, err := Plan(corpus, Options{SkillName: "verify-app", AppTitle: "App"})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(router.Areas) != 2 {
		t.Fatalf("want 2 areas, got %d: %+v", len(router.Areas), router.Areas)
	}
	// Areas is sorted by slug.
	if router.Areas[0].Slug != "library-ui" || router.Areas[1].Slug != "requests" {
		t.Fatalf("areas not sorted by slug: %+v", router.Areas)
	}

	lib := router.Areas[0]
	if len(lib.Views) != 1 || lib.Views[0].Name != "LibraryView" {
		t.Errorf("library-ui.Views = %+v", lib.Views)
	}
	if len(lib.Components) != 2 {
		t.Fatalf("library-ui.Components = %+v, want 2 (parent + child)", lib.Components)
	}
	if lib.Components[0].Name != "LibraryTable" || lib.Components[1].Name != "LibrarySearchBar" {
		t.Errorf("library-ui.Components order = %+v, want parent before child", lib.Components)
	}

	reqs := router.Areas[1]
	if len(reqs.Requests) != 1 || reqs.Requests[0].Name != "GetLibrary" {
		t.Errorf("requests.Requests = %+v", reqs.Requests)
	}
	if len(reqs.Views) != 0 {
		t.Errorf("a requests-only file should have no views: %+v", reqs.Views)
	}
}

func TestPlan_attributesRefExpansionToTheDefiningFile(t *testing.T) {
	corpus := loadFixture(t, map[string]string{
		"globals.yaml": `
version: 1
components:
  - name: Sidebar
    selector: nav.rail
`,
		"home.yaml": `
version: 1
views:
  - name: Home
    route: /
    components:
      - $ref: Sidebar
`,
	})

	router, err := Plan(corpus, Options{SkillName: "verify-app"})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	var home *Area
	for i := range router.Areas {
		if router.Areas[i].Slug == "home" {
			home = &router.Areas[i]
		}
	}
	if home == nil {
		t.Fatalf("no home area: %+v", router.Areas)
	}
	if len(home.Views) != 1 || len(home.Views[0].Components) != 1 {
		t.Fatalf("home view components = %+v", home.Views)
	}
	// The $ref'd Sidebar shows up on the Home view (it's usable there), but
	// its area attribution is skillgen.Plan's job via ComponentDef.SourceFile,
	// which the loader sets to the DEFINING file (globals), not home — the
	// component still belongs to the file that named it, matching how
	// Source/Tags/Memory already work.
	if got := home.Views[0].Components[0].SourceFile; got != "globals" {
		t.Errorf("Sidebar SourceFile = %q, want %q", got, "globals")
	}
}

func TestPlan_requestsOnlyFileBecomesAnArea(t *testing.T) {
	corpus := loadFixture(t, map[string]string{
		"requests.yaml": `
version: 1
requests:
  - name: Ping
    route: /api/ping
    method: GET
`,
	})
	router, err := Plan(corpus, Options{SkillName: "verify-app"})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(router.Areas) != 1 || router.Areas[0].Slug != "requests" {
		t.Fatalf("Areas = %+v", router.Areas)
	}
	if len(router.Areas[0].Requests) != 1 {
		t.Errorf("Requests = %+v", router.Areas[0].Requests)
	}
}

func TestRender_isDeterministic(t *testing.T) {
	corpus := loadFixture(t, map[string]string{
		"library-ui.yaml": `
version: 1
views:
  - name: LibraryView
    route: /library
    description: The object library
components:
  - name: LibraryTable
    selector: '[data-component="LibraryTable"]'
    description: The main listing table
    children:
      - name: LibrarySearchBar
        selector: input
        description: Search bar
`,
	})
	router, err := Plan(corpus, Options{SkillName: "verify-app", AppTitle: "App"})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	first, err := Render(router)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for i := 0; i < 20; i++ {
		again, err := Render(router)
		if err != nil {
			t.Fatalf("Render (iter %d): %v", i, err)
		}
		if len(again) != len(first) {
			t.Fatalf("iter %d: file count changed: %d vs %d", i, len(again), len(first))
		}
		for j := range first {
			if first[j].Path != again[j].Path {
				t.Fatalf("iter %d: path order changed at %d: %q vs %q", i, j, first[j].Path, again[j].Path)
			}
			if !bytes.Equal(first[j].Content, again[j].Content) {
				t.Fatalf("iter %d: content changed for %s", i, first[j].Path)
			}
		}
	}
}

func TestRender_producesFourH2sPerAreaFileInOrder(t *testing.T) {
	corpus := loadFixture(t, map[string]string{
		"library-ui.yaml": `
version: 1
views:
  - name: LibraryView
    route: /library
`,
	})
	router, _ := Plan(corpus, Options{SkillName: "verify-app"})
	files, err := Render(router)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	var areaFile *File
	for i := range files {
		if files[i].Path == "references/areas/library-ui.md" {
			areaFile = &files[i]
		}
	}
	if areaFile == nil {
		t.Fatal("no library-ui area file rendered")
	}
	want := []string{
		"## Sub-features",
		"## How to get to it (user POV)",
		"## Driving it with sightmap browser",
		"## Gotchas",
	}
	content := string(areaFile.Content)
	var got []string
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "## ") {
			got = append(got, line)
		}
	}
	if !equalSlices(got, want) {
		t.Errorf("H2s = %v, want %v\n\n%s", got, want, content)
	}
}

// The index's job is to route a cold prompt to the right area file; its
// per-area summary is the thing most likely to need a human's correction
// (the corpus has no file-level description field to derive it from
// perfectly). Files must read that correction back out of the area file
// rather than recompute its own guess, so fixing it once, in one place,
// actually fixes the index too.
func TestFiles_indexReadsBackAHandEditedSummary(t *testing.T) {
	corpus := loadFixture(t, map[string]string{
		"library-ui.yaml": `
version: 1
views:
  - name: LibraryView
    route: /library
components:
  - name: LibraryTable
    selector: '[data-component="LibraryTable"]'
`,
	})
	router, err := Plan(corpus, Options{SkillName: "verify-app", AppTitle: "App"})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	root := t.TempDir()

	// First generation: nothing on disk, so the index gets the derived guess.
	first, err := Files(root, router)
	if err != nil {
		t.Fatalf("Files (first): %v", err)
	}
	if _, err := Write(root, first); err != nil {
		t.Fatalf("Write: %v", err)
	}
	index, _ := os.ReadFile(filepath.Join(root, "references/README.md"))
	if !strings.Contains(string(index), "1 view, 1 component.") {
		t.Fatalf("first-generation index should carry the derived summary:\n%s", index)
	}

	// An author corrects the area file's summary in place.
	areaPath := filepath.Join(root, "references/areas/library-ui.md")
	area, _ := os.ReadFile(areaPath)
	corrected := strings.Replace(string(area), "> 1 view, 1 component.", "> The object library and its saved objects.", 1)
	if corrected == string(area) {
		t.Fatalf("test fixture didn't find the expected derived summary to replace:\n%s", area)
	}
	if err := os.WriteFile(areaPath, []byte(corrected), 0o644); err != nil {
		t.Fatal(err)
	}

	// Regenerating must read the correction back into the index.
	second, err := Files(root, router)
	if err != nil {
		t.Fatalf("Files (second): %v", err)
	}
	if _, err := Write(root, second); err != nil {
		t.Fatalf("Write: %v", err)
	}
	index, _ = os.ReadFile(filepath.Join(root, "references/README.md"))
	if !strings.Contains(string(index), "The object library and its saved objects.") {
		t.Errorf("index should reflect the hand-edited summary after regenerating:\n%s", index)
	}
	if strings.Contains(string(index), "1 view, 1 component.") {
		t.Errorf("index should no longer show the stale derived summary:\n%s", index)
	}
}

func TestRender_neverEmbedsAnAbsolutePathOrTimestamp(t *testing.T) {
	corpus := loadFixture(t, map[string]string{
		"library-ui.yaml": "version: 1\nviews:\n  - name: LibraryView\n    route: /library\n",
	})
	router, _ := Plan(corpus, Options{SkillName: "verify-app"})
	files, err := Render(router)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, f := range files {
		if bytes.Contains(f.Content, []byte(os.TempDir())) {
			t.Errorf("%s embeds an absolute temp path", f.Path)
		}
	}
}

func TestRender_hasExactlyOneTrailingNewlineAndNoTrailingWhitespace(t *testing.T) {
	corpus := loadFixture(t, map[string]string{
		"library-ui.yaml": "version: 1\nviews:\n  - name: LibraryView\n    route: /library\n",
	})
	router, _ := Plan(corpus, Options{SkillName: "verify-app"})
	files, err := Render(router)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, f := range files {
		s := string(f.Content)
		if strings.HasSuffix(s, "\n\n") || !strings.HasSuffix(s, "\n") {
			t.Errorf("%s: want exactly one trailing newline, got suffix %q", f.Path, s[max(0, len(s)-5):])
		}
		for i, line := range strings.Split(strings.TrimSuffix(s, "\n"), "\n") {
			if line != strings.TrimRight(line, " \t") {
				t.Errorf("%s: line %d has trailing whitespace: %q", f.Path, i, line)
			}
		}
	}
}
