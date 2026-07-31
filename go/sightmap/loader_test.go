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

// A component's tags: and source: flatten onto its match.SightmapComponent, and neither
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

// A request's properties: (field or pattern extraction) round-trip onto its
// match.Request, and a view-scoped request is kept separate from a global one
// with the same route.
func TestLoadDir_RequestProperties(t *testing.T) {
	dir := t.TempDir()
	yaml := `
version: 1
requests:
  - name: CheckoutPayment
    route: /api/checkout/pay
    method: POST
    tags: [defect]
    properties:
      - name: status
        field: rsp.body.status
      - name: rateLimitRemaining
        pattern: 'X-RateLimit-Remaining:\s*(\d+)'
`
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	corpus, err := loadDir(dir)
	if err != nil {
		t.Fatalf("loadDir: %v", err)
	}
	if len(corpus.GlobalRequests) != 1 {
		t.Fatalf("want 1 global request, got %d: %+v", len(corpus.GlobalRequests), corpus.GlobalRequests)
	}
	req := corpus.GlobalRequests[0]
	if req.Route != "/api/checkout/pay" || req.Method != "POST" {
		t.Errorf("request identity = %+v, want route=/api/checkout/pay method=POST", req)
	}
	if !reflect.DeepEqual(req.Tags, []string{"defect"}) {
		t.Errorf("request.Tags = %v, want [defect]", req.Tags)
	}
	if len(req.Properties) != 2 {
		t.Fatalf("want 2 properties, got %d: %+v", len(req.Properties), req.Properties)
	}
	if req.Properties[0].Name != "status" || req.Properties[0].Field != "rsp.body.status" || req.Properties[0].Pattern != "" {
		t.Errorf("properties[0] = %+v, want a field-only extraction named status", req.Properties[0])
	}
	if req.Properties[1].Name != "rateLimitRemaining" || req.Properties[1].Pattern == "" || req.Properties[1].Field != "" {
		t.Errorf("properties[1] = %+v, want a pattern-only extraction named rateLimitRemaining", req.Properties[1])
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
