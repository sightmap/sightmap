package sightmap_test

import (
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// corpusFrom is a convenience constructor for test corpora.
func corpusFrom(globals []sightmap.ComponentDef, views []sightmap.ViewDef) *sightmap.Corpus {
	return &sightmap.Corpus{GlobalComponents: globals, Views: views}
}

func TestValidate_EmptyName(t *testing.T) {
	c := corpusFrom([]sightmap.ComponentDef{
		{Name: "", Selectors: []string{"div"}},
	}, nil)
	errs := sightmap.Validate(c)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Message != "component has empty name" {
		t.Errorf("unexpected message: %q", errs[0].Message)
	}
}

func TestValidate_NoSelector(t *testing.T) {
	c := corpusFrom([]sightmap.ComponentDef{
		{Name: "NavBar", Selectors: nil},
	}, nil)
	errs := sightmap.Validate(c)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Component != "NavBar" {
		t.Errorf("expected Component=NavBar, got %q", errs[0].Component)
	}
}

func TestValidate_BadSelector(t *testing.T) {
	c := corpusFrom([]sightmap.ComponentDef{
		{Name: "NavBar", Selectors: []string{":hover"}},
	}, nil)
	errs := sightmap.Validate(c)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Component != "NavBar" {
		t.Errorf("expected Component=NavBar, got %q", errs[0].Component)
	}
	if errs[0].Selector != ":hover" {
		t.Errorf("expected Selector=:hover, got %q", errs[0].Selector)
	}
}

func TestValidate_DuplicateNameGlobal(t *testing.T) {
	// True duplicate: same name AND same selector — should error.
	c := corpusFrom([]sightmap.ComponentDef{
		{Name: "NavBar", Selectors: []string{"nav"}},
		{Name: "NavBar", Selectors: []string{"nav"}},
	}, nil)
	errs := sightmap.Validate(c)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Component != "NavBar" {
		t.Errorf("expected Component=NavBar, got %q", errs[0].Component)
	}
}

func TestValidate_DuplicateNameInView(t *testing.T) {
	// True duplicate within a view: same name AND same selector — should error.
	c := corpusFrom(nil, []sightmap.ViewDef{
		{
			Name:  "Home",
			Route: "/",
			Components: []sightmap.ComponentDef{
				{Name: "Hero", Selectors: []string{".hero"}},
				{Name: "Hero", Selectors: []string{".hero"}},
			},
		},
	})
	errs := sightmap.Validate(c)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Component != "Hero" {
		t.Errorf("expected Component=Hero, got %q", errs[0].Component)
	}
}

func TestValidate_SameNameDifferentSelector_OK(t *testing.T) {
	// Intentional reuse: same child name under different parent components
	// (e.g. CarouselScrollButton under multiple carousels). Must NOT error.
	c := corpusFrom([]sightmap.ComponentDef{
		{Name: "ScrollBtn", Selectors: []string{"[data-testid='carousel-a'] button"}},
		{Name: "ScrollBtn", Selectors: []string{"[data-testid='carousel-b'] button"}},
		{Name: "ScrollBtn", Selectors: []string{"[data-testid='carousel-c'] button"}},
	}, nil)
	errs := sightmap.Validate(c)
	if len(errs) != 0 {
		t.Errorf("expected no errors for same-name different-selector reuse, got: %v", errs)
	}
}

func TestValidate_SameNameSelectorDifferentParent_OK(t *testing.T) {
	// Two definitions whose expanded children share a name and a selector: the
	// definitions scope is flattened, but a child name is scoped to its parent.
	c := &sightmap.Corpus{Definitions: []sightmap.ComponentDef{
		{Name: "SaveButton", Selectors: []string{"button.save"}},
		{Name: "Label", Selectors: []string{"button.save span"}, ParentChain: []string{"SaveButton"}},
		{Name: "SaveButton2", Selectors: []string{"button.save"}},
		{Name: "Label", Selectors: []string{"button.save span"}, ParentChain: []string{"SaveButton2"}},
	}}
	if errs := sightmap.Validate(c); len(errs) != 0 {
		t.Errorf("expected no errors for one child name under two parents, got: %v", errs)
	}
}

func TestValidate_DuplicateChildSameParent(t *testing.T) {
	c := corpusFrom(nil, []sightmap.ViewDef{{
		Name:  "Home",
		Route: "/",
		Components: []sightmap.ComponentDef{
			{Name: "Card", Selectors: []string{".card"}},
			{Name: "Title", Selectors: []string{".card h2"}, ParentChain: []string{"Card"}},
			{Name: "Title", Selectors: []string{".card h2"}, ParentChain: []string{"Card"}},
		},
	}})
	errs := sightmap.Validate(c)
	if len(errs) != 1 || errs[0].Component != "Title" {
		t.Fatalf("expected 1 duplicate error on Title, got: %v", errs)
	}
}

func TestValidate_MissingRoute(t *testing.T) {
	c := corpusFrom(nil, []sightmap.ViewDef{
		{
			Name:       "Home",
			Route:      "",
			Components: []sightmap.ComponentDef{{Name: "Hero", Selectors: []string{".hero"}}},
		},
	})
	errs := sightmap.Validate(c)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Message != "view has empty route" {
		t.Errorf("unexpected message: %q", errs[0].Message)
	}
}

func TestValidate_Clean(t *testing.T) {
	c := corpusFrom(
		[]sightmap.ComponentDef{
			{Name: "NavBar", Selectors: []string{"nav"}},
		},
		[]sightmap.ViewDef{
			{
				Name:       "Home",
				Route:      "/",
				Components: []sightmap.ComponentDef{{Name: "Hero", Selectors: []string{".hero"}}},
			},
		},
	)
	errs := sightmap.Validate(c)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(errs), errs)
	}
}
