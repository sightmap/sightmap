package sightmap

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// File-level memory applies whenever any definition from that file is active;
// loadDir doesn't track which component or view came from which file, so it
// concatenates file memory across the corpus in file-path order (loadDir already
// walks yamlPaths in that order for deterministic merging).
func TestLoadDir_FileLevelMemoryAccumulatesInPathOrder(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "a.yaml"),
		[]byte("version: 1\nmemory:\n  - from a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.yaml"),
		[]byte("version: 1\nmemory:\n  - from b\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	corpus, err := loadDir(dir)
	if err != nil {
		t.Fatalf("loadDir: %v", err)
	}
	want := []string{"from a", "from b"}
	if !reflect.DeepEqual(corpus.Memory, want) {
		t.Errorf("corpus.Memory = %v, want %v", corpus.Memory, want)
	}
}

// A component's tags: and source: flatten onto its ComponentDef, and neither
// is inherited by children — matching Memory/Properties/Stability's existing convention
// (only the selector prefix cascades to a child).
func TestLoadDir_ComponentTagsAndSourceDoNotInheritToChildren(t *testing.T) {
	dir := t.TempDir()
	yaml := `
version: 1
components:
  - name: CheckoutError
    selector: .error-banner
    source: src/components/CheckoutForm.tsx
    tags: [defect]
    children:
      - name: CheckoutErrorText
        selector: .error-text
`
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	corpus, err := loadDir(dir)
	if err != nil {
		t.Fatalf("loadDir: %v", err)
	}
	if len(corpus.GlobalComponents) != 2 {
		t.Fatalf("want 2 flattened components, got %d: %+v", len(corpus.GlobalComponents), corpus.GlobalComponents)
	}
	parent, child := corpus.GlobalComponents[0], corpus.GlobalComponents[1]

	if parent.Source != "src/components/CheckoutForm.tsx" {
		t.Errorf("parent.Source = %q, want the declared source", parent.Source)
	}
	if !reflect.DeepEqual(parent.Tags, []string{"defect"}) {
		t.Errorf("parent.Tags = %v, want [defect]", parent.Tags)
	}
	if child.Source != "" {
		t.Errorf("child.Source = %q, want empty (source is not inherited)", child.Source)
	}
	if child.Tags != nil {
		t.Errorf("child.Tags = %v, want nil (tags are not inherited)", child.Tags)
	}
}

// A file's memory: is also recorded per-file (FileMemory), alongside the
// existing corpus-wide concatenation (Memory) — tooling that groups the
// corpus by source file (e.g. skillgen) needs the per-file view; nothing
// existing needs Memory's shape to change.
func TestLoadDir_FileMemoryIsAttributedPerFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"),
		[]byte("version: 1\nmemory:\n  - from a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.yaml"),
		[]byte("version: 1\nmemory:\n  - from b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A file with no memory: contributes no FileMemory entry.
	if err := os.WriteFile(filepath.Join(dir, "c.yaml"),
		[]byte("version: 1\nviews:\n  - name: Empty\n    route: /empty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	corpus, err := loadDir(dir)
	if err != nil {
		t.Fatalf("loadDir: %v", err)
	}
	want := []FileMemory{
		{SourceFile: "a", Memory: []string{"from a"}},
		{SourceFile: "b", Memory: []string{"from b"}},
	}
	if !reflect.DeepEqual(corpus.FileMemory, want) {
		t.Errorf("corpus.FileMemory = %+v, want %+v", corpus.FileMemory, want)
	}
}

// description: was parsed off both components and views but dropped before
// reaching the compiled model — a natural-language field an agent-facing
// consumer (skillgen) needs. Verify it now survives compile.
func TestLoadDir_DescriptionReachesTheModel(t *testing.T) {
	dir := t.TempDir()
	yaml := `
version: 1
views:
  - name: Home
    route: /
    description: The landing page
    components:
      - name: Hero
        selector: .hero
        description: The hero banner
components:
  - name: Nav
    selector: nav
    description: Global navigation
`
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	corpus, err := loadDir(dir)
	if err != nil {
		t.Fatalf("loadDir: %v", err)
	}
	if len(corpus.Views) != 1 || corpus.Views[0].Description != "The landing page" {
		t.Errorf("view Description = %+v, want %q", corpus.Views, "The landing page")
	}
	if len(corpus.Views[0].Components) != 1 || corpus.Views[0].Components[0].Description != "The hero banner" {
		t.Errorf("view component Description = %+v, want %q", corpus.Views[0].Components, "The hero banner")
	}
	if len(corpus.GlobalComponents) != 1 || corpus.GlobalComponents[0].Description != "Global navigation" {
		t.Errorf("global component Description = %+v, want %q", corpus.GlobalComponents, "Global navigation")
	}
}

// SourceFile lets tooling attribute a flattened definition back to the
// .sightmap/*.yaml file it was declared in — the loader flattens hierarchy
// and expands $ref, both of which otherwise erase that attribution. Covers a
// top-level global, a nested child, a view-declared component, and a $ref
// expanded into a view: the $ref keeps the origin file of the global it
// names, not the referencing view's file.
func TestLoadDir_ComponentSourceFileAttribution(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "globals.yaml"), []byte(`
version: 1
components:
  - name: Sidebar
    selector: nav.rail
    children:
      - name: NavLink
        selector: a
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "home.yaml"), []byte(`
version: 1
views:
  - name: Home
    route: /
    components:
      - $ref: Sidebar
      - name: Composer
        selector: .composer
`), 0o644); err != nil {
		t.Fatal(err)
	}

	corpus, err := loadDir(dir)
	if err != nil {
		t.Fatalf("loadDir: %v", err)
	}

	globalsByName := map[string]ComponentDef{}
	for _, c := range corpus.GlobalComponents {
		globalsByName[c.Name] = c
	}
	if got := globalsByName["Sidebar"].SourceFile; got != "globals" {
		t.Errorf("Sidebar.SourceFile = %q, want %q", got, "globals")
	}
	if got := globalsByName["NavLink"].SourceFile; got != "globals" {
		t.Errorf("NavLink (nested child).SourceFile = %q, want %q", got, "globals")
	}

	if len(corpus.Views) != 1 {
		t.Fatalf("want 1 view, got %d", len(corpus.Views))
	}
	byName := map[string]ComponentDef{}
	for _, c := range corpus.Views[0].Components {
		byName[c.Name] = c
	}
	if got := byName["Composer"].SourceFile; got != "home" {
		t.Errorf("Composer (view-declared).SourceFile = %q, want %q", got, "home")
	}
	if got := byName["Sidebar"].SourceFile; got != "globals" {
		t.Errorf("Sidebar ($ref'd into home).SourceFile = %q, want %q (the origin file, not the referencing view's file)", got, "globals")
	}
	if got := byName["NavLink"].SourceFile; got != "globals" {
		t.Errorf("NavLink ($ref'd child).SourceFile = %q, want %q", got, "globals")
	}
}

// A global (file-root) request and a view-scoped request both carry the
// basename of the file they were declared in.
func TestLoadDir_RequestSourceFileAttribution(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "api.yaml"), []byte(`
version: 1
requests:
  - name: GetCurrentUser
    route: /api/me
    method: GET
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "home.yaml"), []byte(`
version: 1
views:
  - name: Home
    route: /
    requests:
      - name: LoadHome
        route: /api/home
        method: GET
`), 0o644); err != nil {
		t.Fatal(err)
	}

	corpus, err := loadDir(dir)
	if err != nil {
		t.Fatalf("loadDir: %v", err)
	}
	if len(corpus.Requests) != 1 || corpus.Requests[0].SourceFile != "api" {
		t.Errorf("global request SourceFile = %+v, want %q", corpus.Requests, "api")
	}
	if len(corpus.Views) != 1 || len(corpus.Views[0].Requests) != 1 || corpus.Views[0].Requests[0].SourceFile != "home" {
		t.Errorf("view request SourceFile = %+v, want %q", corpus.Views[0].Requests, "home")
	}
}

// A message (file-root only) carries the basename of its declaring file.
func TestLoadDir_MessageSourceFileAttribution(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "errors.yaml"), []byte(`
version: 1
messages:
  - name: CartVersionMismatch
    message: cart version mismatch
`), 0o644); err != nil {
		t.Fatal(err)
	}

	corpus, err := loadDir(dir)
	if err != nil {
		t.Fatalf("loadDir: %v", err)
	}
	if len(corpus.Messages) != 1 || corpus.Messages[0].SourceFile != "errors" {
		t.Errorf("message SourceFile = %+v, want %q", corpus.Messages, "errors")
	}
}

func TestSplitSelectors_ParenAware(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		// No parens — simple split
		{"a, b", []string{"a", "b"}},
		// Comma inside parens — must NOT split
		{`:is([data-testid="main"], [data-testid="top"]) [data-testid="foo"]`,
			[]string{`:is([data-testid="main"], [data-testid="top"]) [data-testid="foo"]`}},
		// Multiple top-level selectors, one with :is()
		{`:is(a, b) span, div.bar`,
			[]string{`:is(a, b) span`, `div.bar`}},
		// Nested parens
		{`:is(:not(.foo), .bar), button`, []string{`:is(:not(.foo), .bar)`, `button`}},
		// Empty
		{"", nil},
		// Whitespace only between commas
		{"a,  , b", []string{"a", "b"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			got := splitSelectors(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("splitSelectors(%q)\n  got  %v\n  want %v", tc.input, got, tc.want)
			}
		})
	}
}
