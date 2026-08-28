package skillgen

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/sightmap/sightmap/go/sightmap"
)

func TestSplitWords(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"LibrarySearchBar", []string{"Library", "Search", "Bar"}},
		{"DQMTable", []string{"DQM", "Table"}},
		{"LibraryUI", []string{"Library", "UI"}},
		{"BulkActionTableSearchFilter", []string{"Bulk", "Action", "Table", "Search", "Filter"}},
		{"VisualizationLegendBaseItem", []string{"Visualization", "Legend", "Base", "Item"}},
		{"X", []string{"X"}},
		{"", nil},
		{"already-kebab", []string{"already", "kebab"}},
		{"H1Heading", []string{"H1", "Heading"}},
		{"BugReportFAB", []string{"Bug", "Report", "FAB"}},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := splitWords(tc.in)
			if !equalSlices(got, tc.want) {
				t.Errorf("splitWords(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestAreaTitle(t *testing.T) {
	cases := map[string]string{
		"library-ui":         "Library UI",
		"dqm-ui":             "DQM UI",
		"named-elements-ui":  "Named Elements UI",
		"exports-highlights": "Exports Highlights",
		"legacy-app":         "Legacy App",
		"requests":           "Requests",
		"subtext":            "Subtext",
	}
	for slug, want := range cases {
		if got := AreaTitle(slug); got != want {
			t.Errorf("AreaTitle(%q) = %q, want %q", slug, got, want)
		}
	}
}

func TestPhrase(t *testing.T) {
	if got := Phrase("LibrarySearchBar"); got != "library search bar" {
		t.Errorf("Phrase(LibrarySearchBar) = %q, want %q", got, "library search bar")
	}
}

func TestAliases(t *testing.T) {
	got := Aliases("LibrarySearchBar", "Library")
	want := []string{"library search bar", "search bar"}
	if !equalSlices(got, want) {
		t.Errorf("Aliases = %v, want %v", got, want)
	}

	// No stripped alias when the name doesn't lead with the area's word.
	got = Aliases("SaveAndShareModal", "Library")
	want = []string{"save and share modal"}
	if !equalSlices(got, want) {
		t.Errorf("Aliases = %v, want %v", got, want)
	}
}

func TestVerbFor(t *testing.T) {
	cases := map[string]string{
		"LibraryTableMenuActions": VerbClick,
		"SaveButton":              VerbClick,
		"LibrarySearchBar":        VerbFill,
		"SaveAndShareModal":       VerbWaitFor,
		"LibraryTable":            VerbWaitFor,
		"VisualizationLegend":     VerbHover,
	}
	for name, want := range cases {
		if got := VerbFor(name); got != want {
			t.Errorf("VerbFor(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestCommand_rendersAValidBrowserInvocationPerVerb(t *testing.T) {
	cases := []struct {
		name string
		comp sightmap.ComponentDef
		want string
	}{
		{"click", sightmap.ComponentDef{Name: "SaveButton"}, `sightmap browser click 'SaveButton'`},
		{"fill has a value argument", sightmap.ComponentDef{Name: "LibrarySearchBar"}, `sightmap browser fill --clear 'LibrarySearchBar' "…"`},
		{"wait-for has --component", sightmap.ComponentDef{Name: "SaveAndShareModal"}, `sightmap browser wait-for --component 'SaveAndShareModal'`},
		{"fallback is hover, not a bare browser-less command", sightmap.ComponentDef{Name: "VisualizationLegend"}, `sightmap browser hover 'VisualizationLegend'`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Command(tc.comp); got != tc.want {
				t.Errorf("Command(%+v) = %q, want %q", tc.comp, got, tc.want)
			}
		})
	}
}

func equalSlices(a, b []string) bool {
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

func TestRouterDescription_fitsBudgetAndDropsWholeAreas(t *testing.T) {
	areas := make([]Area, 0, 30)
	for i := range 30 {
		areas = append(areas, Area{
			Slug:  "area-" + strings.Repeat("x", i%3+1),
			Title: "Very Long Descriptive Area Title " + strings.Repeat("Z", 20),
		})
	}
	// The fixed trigger sentence (head+tail, no areas) has its own length;
	// a budget below that floor can't be met and isn't a realistic input
	// (the documented default is 900), so the smallest case here just clears
	// the floor for this fixture's naming.
	for _, budget := range []int{260, 480, 900} {
		desc := RouterDescription("Fullstory", areas, budget)
		if n := utf8.RuneCountInString(desc); n > budget {
			t.Errorf("budget %d: got %d runes: %q", budget, n, desc)
		}
		if strings.HasSuffix(strings.TrimSpace(desc), "…") {
			t.Errorf("budget %d: description ends mid-word: %q", budget, desc)
		}
	}
}

func TestRouterDescription_isStableUnderReordering(t *testing.T) {
	areas := []Area{
		{Slug: "a", Title: "Area A", Components: make([]sightmap.ComponentDef, 3)},
		{Slug: "b", Title: "Area B", Components: make([]sightmap.ComponentDef, 1)},
	}
	shuffled := []Area{areas[1], areas[0]}

	d1 := RouterDescription("App", areas, 900)
	d2 := RouterDescription("App", shuffled, 900)
	if d1 != d2 {
		t.Errorf("description should be stable regardless of input order:\n%q\n%q", d1, d2)
	}
}
