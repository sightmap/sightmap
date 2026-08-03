package match_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/match"
	"gopkg.in/yaml.v3"
)

func TestConformance(t *testing.T) {
	fixtures, err := filepath.Glob("../conformance/fixtures/*")
	if err != nil || len(fixtures) == 0 {
		t.Fatal("no conformance fixtures found")
	}
	for _, dir := range fixtures {
		name := filepath.Base(dir)
		t.Run(name, func(t *testing.T) {
			// load component-tree.json
			treeData, err := os.ReadFile(filepath.Join(dir, "component-tree.json"))
			if err != nil {
				t.Fatalf("read tree: %v", err)
			}
			var root comps.ComponentNode
			if err := json.Unmarshal(treeData, &root); err != nil {
				t.Fatalf("parse tree: %v", err)
			}

			// load sightmap.yaml
			smData, err := os.ReadFile(filepath.Join(dir, "sightmap.yaml"))
			if err != nil {
				t.Fatalf("read sightmap: %v", err)
			}
			var defs []match.ComponentDef
			if err := yaml.Unmarshal(smData, &defs); err != nil {
				t.Fatalf("parse sightmap: %v", err)
			}

			// load expected-matches.json
			expData, err := os.ReadFile(filepath.Join(dir, "expected-matches.json"))
			if err != nil {
				t.Fatalf("read expected: %v", err)
			}
			var expected map[string][]string
			if err := json.Unmarshal(expData, &expected); err != nil {
				t.Fatalf("parse expected: %v", err)
			}

			// run
			result := match.ApplySightmap(&root, defs)

			// collect got: name → sorted []id
			got := make(map[string][]string)
			for node, sm := range result {
				got[sm.Name] = append(got[sm.Name], node.Id)
			}
			for k := range got {
				sort.Strings(got[k])
			}
			for k := range expected {
				sort.Strings(expected[k])
			}

			// compare
			for name, wantIDs := range expected {
				gotIDs := got[name]
				if !equalStringSlices(gotIDs, wantIDs) {
					t.Errorf("%s: got %v, want %v", name, gotIDs, wantIDs)
				}
			}
			// check no unexpected matches
			for name := range got {
				if _, ok := expected[name]; !ok {
					t.Errorf("unexpected match for component %q: %v", name, got[name])
				}
			}
		})
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
