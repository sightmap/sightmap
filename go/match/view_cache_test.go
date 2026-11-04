package match

import (
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// View names need not be unique, so two same-named views must not share
// compiled queries.
func TestMatchViewKeepsSameNamedViewsApart(t *testing.T) {
	corpus := &sightmap.Corpus{Views: []sightmap.ViewDef{
		{Name: "Login", Components: []sightmap.ComponentDef{{Name: "First", Selectors: []string{"button"}}}},
		{Name: "Login", Components: []sightmap.ComponentDef{{Name: "Second", Selectors: []string{"button"}}}},
	}}
	root := &sightmap.ComponentNode{Id: "1", Element: &sightmap.Element{Tag: "button"}}
	m := NewMatcher(corpus)
	for i, want := range []string{"First", "Second"} {
		got := m.MatchView(root, &corpus.Views[i])[root]
		if got == nil || got.Name != want {
			t.Errorf("view %d: matched %v, want %s", i, got, want)
		}
	}
}
