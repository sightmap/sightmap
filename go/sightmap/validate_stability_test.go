package sightmap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidate_ComponentStability(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		wantError string
	}{
		{
			name: "valid uncertain",
			yaml: `
components:
  - name: TestComponent
    selector: button
    stability: uncertain
`,
			wantError: "",
		},
		{
			name: "valid unstable",
			yaml: `
components:
  - name: TestComponent
    selector: button
    stability: unstable
`,
			wantError: "",
		},
		{
			name: "invalid stability value",
			yaml: `
components:
  - name: TestComponent
    selector: button
    stability: broken
`,
			wantError: `invalid stability value "broken"`,
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

			errs := Validate(corpus)

			if tt.wantError == "" {
				if len(errs) > 0 {
					t.Errorf("expected no errors, got: %v", errs)
				}
			} else {
				found := false
				for _, e := range errs {
					if strings.Contains(e.Message, tt.wantError) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error containing %q, got: %v", tt.wantError, errs)
				}
			}
		})
	}
}

func TestValidate_ViewStability(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		wantError string
	}{
		{
			name: "valid stub",
			yaml: `
views:
  - name: TestView
    route: /test
    stability: stub
    components: []
`,
			wantError: "",
		},
		{
			name: "valid deferred",
			yaml: `
views:
  - name: TestView
    route: /test
    stability: deferred
    components: []
`,
			wantError: "",
		},
		{
			name: "invalid view stability value",
			yaml: `
views:
  - name: TestView
    route: /test
    stability: experimental
    components: []
`,
			wantError: `invalid stability value "experimental"`,
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

			errs := Validate(corpus)

			if tt.wantError == "" {
				if len(errs) > 0 {
					t.Errorf("expected no errors, got: %v", errs)
				}
			} else {
				found := false
				for _, e := range errs {
					if strings.Contains(e.Message, tt.wantError) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error containing %q, got: %v", tt.wantError, errs)
				}
			}
		})
	}
}
