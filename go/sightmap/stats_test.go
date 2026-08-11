package sightmap

import (
	"os"
	"path/filepath"
	"testing"
)

// TestStats_DedupesSharedGlobals models the loader's output for a multi-file
// corpus where a global component (Navigation) is $ref-reused by two views: the
// loader deep-copies the global into each view's flattened list, so it appears
// three times across the corpus. Totals must count it — and its memory entries
// — exactly once, while each per-view row still counts its own expanded copy.
func TestStats_DedupesSharedGlobals(t *testing.T) {
	nav := ComponentDef{
		Name:      "Navigation",
		Selectors: []string{`nav[data-component="Navigation"]`},
		Memory:    []string{"sticky on scroll"},
	}
	corpus := &Corpus{
		Memory: []string{"file-level note"},
		GlobalComponents: []ComponentDef{
			nav,
			{Name: "Footer", Selectors: []string{`footer[data-component="Footer"]`}},
		},
		Requests: []RequestDef{
			{Name: "GetCurrentUser", Route: "/api/me", Method: "GET"},
		},
		Views: []View{
			{
				Name:   "Checkout",
				Route:  "/checkout",
				Memory: []string{"guest checkout allowed"},
				Components: []ComponentDef{
					nav, // $ref-expanded copy of the global
					{Name: "CartSummary", Selectors: []string{`[data-component="CartSummary"]`}},
					{Name: "PaymentForm", Selectors: []string{`[data-component="PaymentForm"]`}},
					{Name: "submit", Selectors: []string{`[data-component="PaymentForm"] button[type="submit"]`}},
				},
				Requests: []RequestDef{
					{Name: "PlaceOrder", Route: "/api/orders", Method: "POST",
						Memory: []string{"Idempotency-Key header is required"}},
				},
			},
			{
				Name:  "Dashboard",
				Route: "/dashboard",
				Components: []ComponentDef{
					nav, // same global, reused by a second view
					{Name: "ActivityFeed", Selectors: []string{`[data-component="ActivityFeed"]`},
						Properties: []Property{{Name: "count", Extract: "text"}}},
				},
			},
		},
	}

	s := corpus.Stats()

	if s.Views != 2 {
		t.Errorf("Views = %d, want 2", s.Views)
	}
	// Navigation appears 3× (global + 2 views) but is one distinct component:
	// Navigation, Footer, CartSummary, PaymentForm, submit, ActivityFeed.
	if s.Components != 6 {
		t.Errorf("Components = %d, want 6 (Navigation deduped to one)", s.Components)
	}
	if s.Requests != 2 {
		t.Errorf("Requests = %d, want 2 (1 global + 1 view-scoped)", s.Requests)
	}
	if s.Properties != 1 {
		t.Errorf("Properties = %d, want 1", s.Properties)
	}
	// file(1) + Navigation(1, once despite 3 copies) + Checkout view(1) + PlaceOrder(1).
	if s.Memory != 4 {
		t.Errorf("Memory = %d, want 4 (Navigation's entry counted once)", s.Memory)
	}

	want := []ViewStats{
		{Name: "Checkout", Route: "/checkout", Components: 4, Requests: 1},
		{Name: "Dashboard", Route: "/dashboard", Components: 2, Requests: 0},
	}
	if len(s.PerView) != len(want) {
		t.Fatalf("PerView has %d rows, want %d: %+v", len(s.PerView), len(want), s.PerView)
	}
	for i, w := range want {
		if s.PerView[i] != w {
			t.Errorf("PerView[%d] = %+v, want %+v", i, s.PerView[i], w)
		}
	}
}

// TestStats_SameNamedViewComponents pins the identity (not name) rule for the
// properties/memory totals: two views may each define a DIFFERENT component
// under the same local name — only global name collisions are rejected — so
// both definitions' properties and memory must be counted. Summing over the
// name-deduped component list drops the second view's entirely.
func TestStats_SameNamedViewComponents(t *testing.T) {
	corpus := &Corpus{
		Views: []View{
			{
				Name:  "Products",
				Route: "/products",
				Components: []ComponentDef{{
					Name:       "Card",
					Selectors:  []string{`[data-component="ProductCard"]`},
					Memory:     []string{"price hides while the quote refreshes"},
					Properties: []Property{{Name: "title", Extract: "text"}},
				}},
			},
			{
				Name:  "Blog",
				Route: "/blog",
				Components: []ComponentDef{{
					Name:      "Card", // same local name, a different component
					Selectors: []string{`[data-component="PostCard"]`},
					Memory:    []string{"excerpt is truncated server-side"},
					Properties: []Property{
						{Name: "headline", Extract: "text"},
						{Name: "author", Extract: "attr=data-author"},
					},
				}},
			},
		},
	}

	s := corpus.Stats()

	// The vocabulary total still dedupes by name — one name, one entry — which
	// is what the per-view rows and the "N distinct components" line mean.
	if s.Components != 1 {
		t.Errorf("Components = %d, want 1 (both definitions share the name Card)", s.Components)
	}
	if s.Properties != 3 {
		t.Errorf("Properties = %d, want 3 (1 from Products + 2 from Blog)", s.Properties)
	}
	if s.Memory != 2 {
		t.Errorf("Memory = %d, want 2 (one entry per definition)", s.Memory)
	}
}

// TestStats_GlobalsOnlyCorpus: a corpus with no views still reports its global
// components and requests in the totals, with an empty (not nil) per-view list
// so the JSON contract serializes it as [].
func TestStats_GlobalsOnlyCorpus(t *testing.T) {
	corpus := &Corpus{
		GlobalComponents: []ComponentDef{
			{Name: "Header", Selectors: []string{"#header"},
				Properties: []Property{{Name: "brand", Extract: "text"}}},
		},
		Requests: []RequestDef{
			{Name: "Ping", Route: "/api/ping"},
		},
	}

	s := corpus.Stats()

	if s.Views != 0 || s.Components != 1 || s.Requests != 1 || s.Properties != 1 {
		t.Errorf("got views=%d components=%d requests=%d properties=%d, want 0/1/1/1",
			s.Views, s.Components, s.Requests, s.Properties)
	}
	if s.PerView == nil || len(s.PerView) != 0 {
		t.Errorf("PerView = %#v, want empty non-nil slice", s.PerView)
	}
	if s.IsEmpty() {
		t.Error("IsEmpty() = true for a corpus with global components")
	}
}

// TestStats_IsEmpty: only a corpus with nothing at all is empty. A corpus that
// carries just memory entries is legal and must not be treated as empty.
func TestStats_IsEmpty(t *testing.T) {
	if !(&Corpus{}).Stats().IsEmpty() {
		t.Error("IsEmpty() = false for a wholly empty corpus")
	}
	memoryOnly := (&Corpus{Memory: []string{"the app is a single-page React app"}}).Stats()
	if memoryOnly.IsEmpty() {
		t.Error("IsEmpty() = true for a memory-only corpus")
	}
	if memoryOnly.Memory != 1 {
		t.Errorf("Memory = %d, want 1", memoryOnly.Memory)
	}
}

// TestStats_RefExpansionFromDisk runs the real loader so the dedupe is exercised
// against actual $ref expansion rather than a hand-built corpus: a top-level
// $ref expands to an exact copy of the global (counted once), while a $ref
// nested under a parent is a differently-scoped definition and counts again.
func TestStats_RefExpansionFromDisk(t *testing.T) {
	dir := t.TempDir()
	writeCorpusFile(t, dir, "components.yaml", `version: 1
components:
  - name: Navigation
    selector: 'nav'
    memory:
      - sticky on scroll
    properties:
      - name: active
        extract: attr=aria-current
`)
	writeCorpusFile(t, dir, "views/home.yaml", `version: 1
views:
  - name: Home
    route: /
    components:
      - $ref: Navigation
`)
	writeCorpusFile(t, dir, "views/admin.yaml", `version: 1
views:
  - name: Admin
    route: /admin
    components:
      - name: Shell
        selector: '.shell'
        children:
          - $ref: Navigation
`)

	corpus, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if diags := errorFindings(Validate(corpus)); len(diags) != 0 {
		t.Fatalf("fixture corpus does not validate: %v", diags)
	}

	s := corpus.Stats()

	// Navigation, Shell, and Navigation-under-Shell all share two names.
	if s.Components != 2 {
		t.Errorf("Components = %d, want 2 (Navigation, Shell)", s.Components)
	}
	// Global Navigation + Home's identical expansion = 1; Admin's copy is
	// scoped to ".shell nav" and is its own extraction site = 1 more.
	if s.Properties != 2 {
		t.Errorf("Properties = %d, want 2 (global/Home copy once, Admin's parent-scoped copy again)", s.Properties)
	}
	if s.Memory != 2 {
		t.Errorf("Memory = %d, want 2 (same rule as properties)", s.Memory)
	}
	want := []ViewStats{
		{Name: "Admin", Route: "/admin", Components: 2},
		{Name: "Home", Route: "/", Components: 1},
	}
	if len(s.PerView) != len(want) {
		t.Fatalf("PerView = %+v, want %+v", s.PerView, want)
	}
	for i, w := range want {
		if s.PerView[i] != w {
			t.Errorf("PerView[%d] = %+v, want %+v", i, s.PerView[i], w)
		}
	}
}

// errorFindings filters validation findings down to the error-severity ones.
func errorFindings(findings []ValidationError) []ValidationError {
	var out []ValidationError
	for _, f := range findings {
		if f.IsError() {
			out = append(out, f)
		}
	}
	return out
}

// writeCorpusFile writes one YAML file (creating parent directories) into a
// temporary .sightmap/ corpus.
func writeCorpusFile(t *testing.T, dir, rel, body string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}
