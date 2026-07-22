package sightmap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sightmap/sightmap/go/match"
)

func TestLoader_ComponentStability(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		wantComp match.SightmapComponent
	}{
		{
			name: "omitted defaults to empty",
			yaml: `
components:
  - name: TestComponent
    selector: button
`,
			wantComp: match.SightmapComponent{
				Name:       "TestComponent",
				Selectors:  []string{"button"},
				Stability:  "",
				Properties: nil,
			},
		},
		{
			name: "uncertain",
			yaml: `
components:
  - name: UncertainComponent
    selector: '[data-testid="sheet"]'
    stability: uncertain
`,
			wantComp: match.SightmapComponent{
				Name:       "UncertainComponent",
				Selectors:  []string{`[data-testid="sheet"]`},
				Stability:  "uncertain",
				Properties: nil,
			},
		},
		{
			name: "unstable",
			yaml: `
components:
  - name: UnstableComponent
    selector: '.css-module-class'
    stability: unstable
    memory:
      - Selector uses volatile CSS module class names
`,
			wantComp: match.SightmapComponent{
				Name:      "UnstableComponent",
				Selectors: []string{".css-module-class"},
				Stability: "unstable",
				Memory:    []string{"Selector uses volatile CSS module class names"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			yamlPath := filepath.Join(tmpDir, "components.yaml")
			if err := os.WriteFile(yamlPath, []byte(tt.yaml), 0644); err != nil {
				t.Fatal(err)
			}

			corpus, err := loadDir(tmpDir)
			if err != nil {
				t.Fatalf("loadDir: %v", err)
			}

			if len(corpus.GlobalComponents) != 1 {
				t.Fatalf("expected 1 component, got %d", len(corpus.GlobalComponents))
			}

			got := corpus.GlobalComponents[0]
			if got.Name != tt.wantComp.Name {
				t.Errorf("Name: got %q, want %q", got.Name, tt.wantComp.Name)
			}
			if got.Stability != tt.wantComp.Stability {
				t.Errorf("Stability: got %q, want %q", got.Stability, tt.wantComp.Stability)
			}
		})
	}
}

func TestLoader_ViewStability(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		wantView View
	}{
		{
			name: "omitted defaults to empty",
			yaml: `
views:
  - name: HomePage
    route: /
    components: []
`,
			wantView: View{
				Name:       "HomePage",
				Route:      "/",
				Stability:  "",
				Components: []match.SightmapComponent{},
			},
		},
		{
			name: "stub view",
			yaml: `
views:
  - name: LegacyCMSPage
    route: /legacy/**
    stability: stub
    components: []
`,
			wantView: View{
				Name:       "LegacyCMSPage",
				Route:      "/legacy/**",
				Stability:  "stub",
				Components: []match.SightmapComponent{},
			},
		},
		{
			name: "deferred view",
			yaml: `
views:
  - name: AdminPanel
    route: /admin/**
    stability: deferred
    components: []
`,
			wantView: View{
				Name:       "AdminPanel",
				Route:      "/admin/**",
				Stability:  "deferred",
				Components: []match.SightmapComponent{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			yamlPath := filepath.Join(tmpDir, "views.yaml")
			if err := os.WriteFile(yamlPath, []byte(tt.yaml), 0644); err != nil {
				t.Fatal(err)
			}

			corpus, err := loadDir(tmpDir)
			if err != nil {
				t.Fatalf("loadDir: %v", err)
			}

			if len(corpus.Views) != 1 {
				t.Fatalf("expected 1 view, got %d", len(corpus.Views))
			}

			got := corpus.Views[0]
			if got.Name != tt.wantView.Name {
				t.Errorf("Name: got %q, want %q", got.Name, tt.wantView.Name)
			}
			if got.Stability != tt.wantView.Stability {
				t.Errorf("Stability: got %q, want %q", got.Stability, tt.wantView.Stability)
			}
		})
	}
}

func TestLoader_StabilityPropagatedToChildren(t *testing.T) {
	yaml := `
components:
  - name: ParentComponent
    selector: .parent
    stability: uncertain
    children:
      - name: ChildComponent
        selector: .child
        stability: unstable
`
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "components.yaml")
	if err := os.WriteFile(yamlPath, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	corpus, err := loadDir(tmpDir)
	if err != nil {
		t.Fatalf("loadDir: %v", err)
	}

	if len(corpus.GlobalComponents) != 2 {
		t.Fatalf("expected 2 flattened components, got %d", len(corpus.GlobalComponents))
	}

	// Find each component by name
	var parent, child *match.SightmapComponent
	for i := range corpus.GlobalComponents {
		if corpus.GlobalComponents[i].Name == "ParentComponent" {
			parent = &corpus.GlobalComponents[i]
		} else if corpus.GlobalComponents[i].Name == "ChildComponent" {
			child = &corpus.GlobalComponents[i]
		}
	}

	if parent == nil || child == nil {
		t.Fatal("could not find ParentComponent and ChildComponent")
	}

	if parent.Stability != "uncertain" {
		t.Errorf("ParentComponent.Stability: got %q, want %q", parent.Stability, "uncertain")
	}
	if child.Stability != "unstable" {
		t.Errorf("ChildComponent.Stability: got %q, want %q", child.Stability, "unstable")
	}
}
