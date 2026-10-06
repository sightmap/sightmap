package match_test

import (
	"reflect"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// A selector on an attribute whose HTML value compares case-insensitively must
// reach the node through the first-part index, which buckets by exact value.
func TestMatchChain_LegacyCaseInsensitiveAttributeThroughIndex(t *testing.T) {
	m := chainMatcher(sightmap.ComponentDef{Name: "Password", Selectors: []string{`input[type=password]`}})
	pw := sightmap.Element{Tag: "input", Attrs: map[string]string{"type": "PASSWORD"}}
	if got, want := m.NamesForChain([]sightmap.Element{pw}, ""), []string{"Password"}; !reflect.DeepEqual(got, want) {
		t.Errorf("NamesForChain = %v, want %v", got, want)
	}
	inSVG := []sightmap.Element{{Tag: "svg"}, {Tag: "input", Attrs: map[string]string{"type": "PASSWORD"}}}
	if got := m.NamesForChain(inSVG, ""); got != nil {
		t.Errorf("inside svg the value is case-sensitive: NamesForChain = %v, want none", got)
	}
}
