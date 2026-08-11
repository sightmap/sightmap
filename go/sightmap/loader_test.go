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
