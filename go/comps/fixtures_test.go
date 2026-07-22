package comps

import (
	"encoding/json"
	"os"
	"testing"
)

// TestFixtures_CanParse verifies that all test fixtures parse correctly.
func TestFixtures_CanParse(t *testing.T) {
	fixtures := []string{
		"../testdata/fixtures/multi-child-t2.snap.tree.json",
		"../testdata/fixtures/single-child-t2.snap.tree.json",
		"../testdata/fixtures/exhausted-t2.snap.tree.json",
		"../testdata/fixtures/large-t2.snap.tree.json",
	}

	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			data, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatalf("ReadFile failed: %v", err)
			}

			var node ComponentNode
			if err := json.Unmarshal(data, &node); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			// Basic sanity checks
			if node.Role == "" {
				t.Error("Role is empty")
			}
			if node.Children == nil {
				t.Error("Children is nil (should be empty slice)")
			}
			if node.Properties == nil {
				t.Error("Properties is nil (should be empty map)")
			}

			t.Logf("Parsed %s: role=%s, children=%d", fixture, node.Role, len(node.Children))
		})
	}
}
