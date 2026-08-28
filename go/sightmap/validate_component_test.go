package sightmap

import "testing"

func compCorpus(props ...ComponentPropertyDef) *Corpus {
	return &Corpus{
		GlobalComponents: []ComponentDef{
			{Name: "Card", Selectors: []string{".card"}, Properties: props},
		},
	}
}

func codesFor(errs []ValidationError) map[string]int {
	m := map[string]int{}
	for _, e := range errs {
		m[e.Code]++
	}
	return m
}

func TestCheckComponentProperties_ValidForms(t *testing.T) {
	c := compCorpus(
		ComponentPropertyDef{Name: "label", Extract: "text"},
		ComponentPropertyDef{Name: "href", Extract: "attr=href"},
		ComponentPropertyDef{Name: "price", Extract: "Price.text"},
		ComponentPropertyDef{Name: "sold_out", Extract: "exists:SoldOutBadge"},
		ComponentPropertyDef{Name: "amount", Extract: "Row.Price.amount"},
	)
	if errs := checkComponentProperties(c); len(errs) != 0 {
		t.Fatalf("valid forms must not error, got %v", errs)
	}
}

func TestCheckComponentProperties_DuplicateName(t *testing.T) {
	c := compCorpus(
		ComponentPropertyDef{Name: "price", Extract: "text"},
		ComponentPropertyDef{Name: "price", Extract: "attr=data-price"},
	)
	if n := codesFor(checkComponentProperties(c))["component-property-duplicate"]; n != 1 {
		t.Fatalf("expected 1 duplicate-name error, got %d (%v)", n, checkComponentProperties(c))
	}
}

func TestCheckComponentProperties_RejectsRemovedAndMalformed(t *testing.T) {
	bad := []string{
		"inner_text",      // removed DOM mode
		"text_only",       // removed DOM mode
		"inner_html",      // removed DOM mode
		".price",          // bare CSS sub-selector (empty leading segment)
		"[data-testid=x]", // bare CSS selector
		"attr",            // mistyped attr (missing =)
		"attr=",           // attr with no name
		"exists",          // mistyped exists (missing :)
		"exists:",         // exists with empty path
		"Price.",          // path with empty prop
	}
	for _, extract := range bad {
		c := compCorpus(ComponentPropertyDef{Name: "x", Extract: extract})
		if n := codesFor(checkComponentProperties(c))["component-property-extract-invalid"]; n != 1 {
			t.Errorf("extract %q: expected 1 extract-invalid error, got %d", extract, n)
		}
	}
}
