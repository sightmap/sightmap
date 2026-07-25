package coverage

import (
	"testing"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/match"
)

// gapsFixture builds a small tree and returns root + parentMap so each test can
// supply its own matches map. Layout:
//
//	root (generic)
//	└─ banner (generic, data-testid="hero")
//	   └─ image  name="UP TO $800 OFF select appliances"   (the content node)
func gapsFixture() (root, banner, image *comps.ComponentNode, parentMap map[*comps.ComponentNode]*comps.ComponentNode) {
	image = &comps.ComponentNode{
		Id:        "img",
		Role:      "image",
		Name:      "UP TO $800 OFF select appliances",
		IsVisible: true,
	}
	banner = &comps.ComponentNode{
		Id:        "banner",
		Role:      "generic",
		IsVisible: true,
		Selector:  &comps.SelectorPart{Tag: "div", Attrs: map[string]string{"data-testid": "hero"}},
		Children:  []*comps.ComponentNode{image},
	}
	root = &comps.ComponentNode{
		Id:        "root",
		Role:      "generic",
		IsVisible: true,
		Children:  []*comps.ComponentNode{banner},
	}
	return root, banner, image, BuildParentMap(root)
}

func TestAnnotationGaps_FlaggedWhenNoContext(t *testing.T) {
	root, _, image, pm := gapsFixture()
	matches := map[*comps.ComponentNode]*match.SightmapMatch{} // nothing matched

	gaps := Gaps(root, matches, pm, true)
	if len(gaps) != 1 {
		t.Fatalf("want 1 gap, got %d: %+v", len(gaps), gaps)
	}
	g := gaps[0]
	if g.Role != "image" {
		t.Errorf("role = %q, want image", g.Role)
	}
	if g.Name != image.Name {
		t.Errorf("name = %q, want %q", g.Name, image.Name)
	}
	if g.Selector != `div[data-testid="hero"]` {
		t.Errorf("selector = %q, want div[data-testid=\"hero\"]", g.Selector)
	}
}

func TestAnnotationGaps_NotFlaggedWhenDirectlyMatched(t *testing.T) {
	root, _, image, pm := gapsFixture()
	// The content node itself is now captured by a component (T1).
	matches := map[*comps.ComponentNode]*match.SightmapMatch{
		image: {Name: "BannerImage"},
	}
	if gaps := Gaps(root, matches, pm, true); len(gaps) != 0 {
		t.Fatalf("want 0 gaps once node is matched, got %d: %+v", len(gaps), gaps)
	}
}

func TestAnnotationGaps_NotFlaggedWhenAncestorMatched(t *testing.T) {
	root, banner, _, pm := gapsFixture()
	// A matched ancestor gives the node component context → precision guard.
	matches := map[*comps.ComponentNode]*match.SightmapMatch{
		banner: {Name: "HeroBanner"},
	}
	if gaps := Gaps(root, matches, pm, true); len(gaps) != 0 {
		t.Fatalf("want 0 gaps when an ancestor is matched, got %d: %+v", len(gaps), gaps)
	}
}

func TestAnnotationGaps_ShortAndEmptyNamesIgnored(t *testing.T) {
	short := &comps.ComponentNode{Id: "s", Role: "image", Name: "Sale", IsVisible: true}
	empty := &comps.ComponentNode{Id: "e", Role: "image", Name: "   ", IsVisible: true}
	symbols := &comps.ComponentNode{Id: "y", Role: "image", Name: "$$$ 100% !!!!!", IsVisible: true}
	root := &comps.ComponentNode{
		Id:        "root",
		Role:      "generic",
		IsVisible: true,
		Children:  []*comps.ComponentNode{short, empty, symbols},
	}
	pm := BuildParentMap(root)
	matches := map[*comps.ComponentNode]*match.SightmapMatch{}
	if gaps := Gaps(root, matches, pm, true); len(gaps) != 0 {
		t.Fatalf("want 0 gaps for short/empty/symbol names, got %d: %+v", len(gaps), gaps)
	}
}

func TestAnnotationGaps_InteractiveSkipped(t *testing.T) {
	// An interactive long-named orphan is scored by T1/T2/T3, not here.
	link := &comps.ComponentNode{
		Id:            "l",
		Role:          "link",
		Name:          "Shop all appliance deals today",
		IsVisible:     true,
		IsInteractive: true,
	}
	root := &comps.ComponentNode{
		Id:        "root",
		Role:      "generic",
		IsVisible: true,
		Children:  []*comps.ComponentNode{link},
	}
	pm := BuildParentMap(root)
	matches := map[*comps.ComponentNode]*match.SightmapMatch{}
	if gaps := Gaps(root, matches, pm, true); len(gaps) != 0 {
		t.Fatalf("want 0 gaps for interactive node, got %d: %+v", len(gaps), gaps)
	}
}

func TestAnnotationGaps_VisibleOnly(t *testing.T) {
	hidden := &comps.ComponentNode{
		Id:        "h",
		Role:      "image",
		Name:      "Hidden promotional banner copy",
		IsVisible: false,
	}
	root := &comps.ComponentNode{
		Id:        "root",
		Role:      "generic",
		IsVisible: true,
		Children:  []*comps.ComponentNode{hidden},
	}
	pm := BuildParentMap(root)
	matches := map[*comps.ComponentNode]*match.SightmapMatch{}

	if gaps := Gaps(root, matches, pm, true); len(gaps) != 0 {
		t.Fatalf("visibleOnly: want 0 gaps for hidden node, got %d", len(gaps))
	}
	if gaps := Gaps(root, matches, pm, false); len(gaps) != 1 {
		t.Fatalf("include-hidden: want 1 gap for hidden node, got %d", len(gaps))
	}
}

func TestIsContentName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"long with letters", "UP TO $800 OFF select appliances", true},
		{"exactly min letters", "abcdefghijkl", true}, // 12 runes
		{"too short", "Sale today", false},            // 10 runes
		{"empty", "", false},
		{"whitespace only", "        ", false},
		{"long but no letters", "1234567890123456", false},
		{"trimmed to short", "  hi  ", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isContentName(c.in); got != c.want {
				t.Errorf("isContentName(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestUnionContentGaps_DedupAcrossCaptures(t *testing.T) {
	g := ContentGap{Role: "image", Name: "UP TO $800 OFF appliances", Selector: `div[data-testid="hero"]`}
	// Same gap flagged in two captures, plus an unrelated gap in one.
	other := ContentGap{Role: "heading", Name: "Featured savings of the week", Selector: ""}
	capGaps := [][]ContentGap{
		{g},
		{g, other},
	}
	pres := UnionContentGaps(capGaps)
	if len(pres) != 2 {
		t.Fatalf("want 2 distinct gaps, got %d: %+v", len(pres), pres)
	}
	// Sorted by descending appearance count, so the 2-of-2 gap is first.
	if pres[0].AppearedIn != 2 || pres[0].Captures != 2 {
		t.Errorf("first gap AppearedIn/Captures = %d/%d, want 2/2", pres[0].AppearedIn, pres[0].Captures)
	}
	if pres[0].Gap.Role != "image" {
		t.Errorf("first gap role = %q, want image", pres[0].Gap.Role)
	}
	if pres[1].AppearedIn != 1 || pres[1].Captures != 2 {
		t.Errorf("second gap AppearedIn/Captures = %d/%d, want 1/2", pres[1].AppearedIn, pres[1].Captures)
	}
}

func TestUnionContentGaps_DoubleCountWithinCaptureCollapses(t *testing.T) {
	// The same gap appearing twice in ONE capture counts once for that capture.
	g := ContentGap{Role: "image", Name: "UP TO $800 OFF appliances", Selector: ""}
	pres := UnionContentGaps([][]ContentGap{{g, g}})
	if len(pres) != 1 {
		t.Fatalf("want 1 gap, got %d", len(pres))
	}
	if pres[0].AppearedIn != 1 {
		t.Errorf("AppearedIn = %d, want 1 (deduped within capture)", pres[0].AppearedIn)
	}
}
