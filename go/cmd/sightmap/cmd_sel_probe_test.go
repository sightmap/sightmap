package main

import (
	"github.com/sightmap/sightmap/go/sightmap"
	"testing"
)

// makeAnnotation is a test helper that builds a compAnnotation matching a
// specific tag+class combination.
func makeAnnotation(name, tag string, classes []string) compAnnotation {
	return compAnnotation{
		name: name,
		part: &sightmap.SelectorPart{Tag: tag, Classes: classes},
	}
}

// makeAnnotationDt builds an annotation matching a data-testid attribute.
func makeAnnotationDt(name, tag, dt string) compAnnotation {
	return compAnnotation{
		name: name,
		part: &sightmap.SelectorPart{
			Tag:   tag,
			Attrs: map[string]string{"data-testid": dt},
		},
	}
}

// ── summarizeNearestComponents ────────────────────────────────────────────────

func TestSummarizeNearestComponents_nil_when_no_annotations(t *testing.T) {
	results := []elementResult{
		{Tag: "a", Parents: []parentInfo{{Tag: "div", Cls: []string{"foo"}}}},
	}
	got := summarizeNearestComponents(results, nil)
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestSummarizeNearestComponents_nil_when_no_match(t *testing.T) {
	ann := []compAnnotation{makeAnnotation("Foo", "div", []string{"foo"})}
	results := []elementResult{
		{Tag: "a", Parents: []parentInfo{{Tag: "div", Cls: []string{"bar"}}}},
	}
	got := summarizeNearestComponents(results, ann)
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestSummarizeNearestComponents_single_component(t *testing.T) {
	ann := []compAnnotation{makeAnnotationDt("ProductionListings", "div", "country-grouped-productions")}
	results := []elementResult{
		{Tag: "a", Parents: []parentInfo{{Tag: "div", Dt: "country-grouped-productions"}}},
		{Tag: "a", Parents: []parentInfo{{Tag: "div", Dt: "country-grouped-productions"}}},
		{Tag: "a", Parents: []parentInfo{{Tag: "div", Dt: "country-grouped-productions"}}},
	}
	got := summarizeNearestComponents(results, ann)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(got), got)
	}
	if got[0].name != "ProductionListings" {
		t.Errorf("name = %q, want ProductionListings", got[0].name)
	}
	if got[0].count != 3 {
		t.Errorf("count = %d, want 3", got[0].count)
	}
	want := `div[data-testid="country-grouped-productions"]`
	if got[0].desc != want {
		t.Errorf("desc = %q, want %q", got[0].desc, want)
	}
}

func TestSummarizeNearestComponents_multiple_components_sorted(t *testing.T) {
	ann := []compAnnotation{
		makeAnnotationDt("ProductionListings", "div", "production-listings"),
		makeAnnotationDt("TopPicksCarousel", "div", "top-picks"),
	}
	// 10 results → ProductionListings, 2 results → TopPicksCarousel
	results := make([]elementResult, 12)
	for i := range results {
		dt := "production-listings"
		if i >= 10 {
			dt = "top-picks"
		}
		results[i] = elementResult{
			Tag:     "a",
			Parents: []parentInfo{{Tag: "div", Dt: dt}},
		}
	}
	got := summarizeNearestComponents(results, ann)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(got), got)
	}
	if got[0].name != "ProductionListings" || got[0].count != 10 {
		t.Errorf("first entry: got (%s, %d), want (ProductionListings, 10)", got[0].name, got[0].count)
	}
	if got[1].name != "TopPicksCarousel" || got[1].count != 2 {
		t.Errorf("second entry: got (%s, %d), want (TopPicksCarousel, 2)", got[1].name, got[1].count)
	}
}

func TestSummarizeNearestComponents_uses_nearest_ancestor_only(t *testing.T) {
	// Parent chain: outer=Foo, inner=Bar. Only the nearest (inner) should count.
	ann := []compAnnotation{
		makeAnnotation("Outer", "div", []string{"outer"}),
		makeAnnotation("Inner", "div", []string{"inner"}),
	}
	results := []elementResult{
		{
			Tag: "a",
			Parents: []parentInfo{
				{Tag: "div", Cls: []string{"inner"}}, // nearest
				{Tag: "div", Cls: []string{"outer"}},
			},
		},
	}
	got := summarizeNearestComponents(results, ann)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(got), got)
	}
	if got[0].name != "Inner" {
		t.Errorf("expected Inner (nearest), got %s", got[0].name)
	}
}

func TestSummarizeNearestComponents_tiebreak_alphabetical(t *testing.T) {
	ann := []compAnnotation{
		makeAnnotation("Zebra", "div", []string{"z"}),
		makeAnnotation("Alpha", "div", []string{"a"}),
	}
	results := []elementResult{
		{Tag: "x", Parents: []parentInfo{{Tag: "div", Cls: []string{"z"}}}},
		{Tag: "x", Parents: []parentInfo{{Tag: "div", Cls: []string{"a"}}}},
	}
	got := summarizeNearestComponents(results, ann)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	// Equal counts → alphabetical: Alpha before Zebra
	if got[0].name != "Alpha" || got[1].name != "Zebra" {
		t.Errorf("order = [%s, %s], want [Alpha, Zebra]", got[0].name, got[1].name)
	}
}

// ── parentDesc ────────────────────────────────────────────────────────────────

func TestParentDesc_prefers_data_testid(t *testing.T) {
	p := parentInfo{Tag: "div", Id: "foo", Cls: []string{"bar"}, Dt: "my-widget"}
	got := parentDesc(p)
	want := `div[data-testid="my-widget"]`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParentDesc_falls_back_to_elementDesc(t *testing.T) {
	p := parentInfo{Tag: "section", Id: "main", Cls: []string{"wrapper"}}
	got := parentDesc(p)
	want := "section#main.wrapper"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParentDesc_empty_parent(t *testing.T) {
	p := parentInfo{Tag: "div"}
	got := parentDesc(p)
	if got != "div" {
		t.Errorf("got %q, want %q", got, "div")
	}
}
