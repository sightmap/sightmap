package device

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/sightmap/sightmap/go/sightmap"
)

var obstructingTags = map[string]bool{"tabbar": true, "keyboard": true, "navbar": true, "toolbar": true}
var obstructingLabels = map[string]bool{"Tab Bar": true, "Toolbar": true}

func obstructs(o *sightmap.ComponentNode) bool {
	return obstructingTags[o.Role] || (o.Role == "other" && obstructingLabels[o.Name])
}

func barName(o *sightmap.ComponentNode) string {
	if obstructingTags[o.Role] {
		return o.Role
	}
	return strings.ToLower(o.Name)
}

type rect struct{ x, y, w, h int }

func rectOf(n *sightmap.ComponentNode) rect {
	b := n.Bounds
	return rect{b.X, b.Y, b.Width, b.Height}
}

func (r rect) contains(x, y int) bool { return x >= r.x && x <= r.x+r.w && y >= r.y && y <= r.y+r.h }

func (r rect) grow(d int) rect { return rect{r.x - d, r.y - d, r.w + 2*d, r.h + 2*d} }

// tabBand stands in for a JS tab bar (React Navigation), which has tabs but no
// container element: the band from the top of a bottom row of tabs to the
// screen's bottom.
func (t *Tree) tabBand() *sightmap.ComponentNode {
	y, found := math.MaxInt, false
	for _, n := range t.Nodes {
		if n.Role == "tab" && n.IsVisible && !t.hasAncestor(n, obstructs) {
			y, found = min(y, n.Bounds.Y), true
		}
	}
	if !found || float64(y) < t.Screen.H/2 {
		return nil
	}
	return &sightmap.ComponentNode{Id: "-", Role: "tabbar", IsVisible: true, State: map[string]string{},
		Element: &sightmap.Element{Tag: "tabbar"}, Bounds: &sightmap.Bounds{X: 0, Y: y, Width: int(t.Screen.W), Height: int(t.Screen.H) - y}}
}

// clearSpot returns a point inside n's frame, on screen and outside zone,
// nearest n's center.
func (t *Tree) clearSpot(n *sightmap.ComponentNode, zone rect) (x, y int, ok bool) {
	cx, cy := Center(n)
	b := n.Bounds
	best := math.Inf(1)
	for _, fx := range []float64{0.15, 0.3, 0.5, 0.7, 0.85} {
		for _, fy := range []float64{0.25, 0.5, 0.75} {
			px, py := b.X+round(float64(b.Width)*fx), b.Y+round(float64(b.Height)*fy)
			if px < 0 || py < 0 || float64(px) > t.Screen.W || float64(py) > t.Screen.H || zone.contains(px, py) {
				continue
			}
			if d := math.Hypot(float64(px-cx), float64(py-cy)); d < best {
				best, x, y, ok = d, px, py, true
			}
		}
	}
	return x, y, ok
}

// AssertActionable returns an error unless n's center is on the screen,
// uncovered by bars, the keyboard, a LogBox toast or a floating dev-menu
// button, and n is enabled. target is how the caller named n, for the hint.
func (s *Snapshot) AssertActionable(n *sightmap.ComponentNode, target string) error {
	desc := s.Describe(n)
	cx, cy := Center(n)
	if !n.IsVisible || !s.OnScreen(n) {
		dir := s.towards(n)
		return withHint("offscreen", fmt.Sprintf("run `sightmap device scroll %s --until %s`, then retry", dir, quote(target)),
			"%s is off-screen (center %d,%d on a %gx%g screen); refusing to tap", desc, cx, cy, s.Screen.W, s.Screen.H)
	}
	if top := s.systemTop(); cy < top {
		hint := fmt.Sprintf("run `sightmap device scroll up --until %s`, then retry", quote(target))
		if s.Platform == PlatformAndroid {
			if s.scrollContainer(n) == nil {
				hint = "it is not in a scrolling list, so scrolling won't move it: the app draws it under the status bar. Leave the screen with `sightmap device button back`, or tap a part of it that is below the bar with `sightmap device tap-xy`"
			}
			return withHint("covered", hint,
				"%s has its center at y=%d, under the status bar (the top %d dp); a tap there goes to the status bar, not the app", desc, cy, top)
		}
		return withHint("covered", hint,
			"%s has its center at y=%d, under the status bar (the top %d pt); a tap there scrolls the list to the top instead", desc, cy, top)
	}
	if bar := s.topBar(n); cy < bar {
		return withHint("covered", fmt.Sprintf("run `sightmap device scroll up --until %s`, then retry", quote(target)),
			"%s has its center at y=%d, under the navigation bar (down to y=%d); a tap there would not reach it", desc, cy, bar)
	}
	if c := s.scrollContainer(n); c != nil {
		if inner := rectOf(c).grow(-containerInset); !inner.contains(cx, cy) {
			dir := "down"
			if cy < inner.y || cx < inner.x {
				dir = "up"
			}
			return withHint("covered", fmt.Sprintf("run `sightmap device scroll %s --until %s`, then retry", dir, quote(target)),
				"%s has its center (%d, %d) outside the visible part of its list (%s), where something else is drawn", desc, cx, cy, s.Describe(c))
		}
	}
	if lb := s.logBoxToast(); lb != nil && lb.Node != n && rectOf(lb.Node).contains(cx, cy) {
		return withHint("covered", fmt.Sprintf("dismiss it with `sightmap device tap-xy %d %d` (its ✕), then retry", lb.DismissX, lb.DismissY),
			"%s is covered by a React Native LogBox toast (%s)", desc, s.Describe(lb.Node))
	}
	unrelated := func(o *sightmap.ComponentNode) bool { return o != n && !s.IsAncestor(o, n) && !s.IsAncestor(n, o) }
	for _, f := range s.floatingControls() {
		zone := rectOf(f).grow(floatingMargin)
		if !unrelated(f) || !zone.contains(cx, cy) {
			continue
		}
		away := "scroll the target away from it"
		if x, y, ok := s.clearSpot(n, zone); ok {
			away = fmt.Sprintf("tap another part of the target with `sightmap device tap-xy %d %d`", x, y)
		}
		return withHint("covered", away+", or hide the floating button (Expo dev client: open the dev menu, Settings, turn off \"Show floating action button\"; reopen the menu later with Device → Shake in Simulator), then retry",
			"%s is within %d pt of %s, which looks like the React Native / Expo dev-menu button. It floats in its own window and catches taps beyond its frame, so tapping (%d, %d) would likely open the dev menu",
			desc, floatingMargin, s.Describe(f), cx, cy)
	}
	candidates := s.Nodes
	if n.Role != "tab" && !s.hasAncestor(n, func(a *sightmap.ComponentNode) bool { return a.Role == "tab" }) {
		if band := s.tabBand(); band != nil {
			candidates = append(candidates[:len(candidates):len(candidates)], band)
		}
	}
	for _, o := range candidates {
		if !obstructs(o) || !o.IsVisible || !unrelated(o) || !rectOf(o).contains(cx, cy) {
			continue
		}
		hint := fmt.Sprintf("scroll it clear of the %s (`sightmap device scroll down` or `up`), then retry", barName(o))
		if o.Role == "keyboard" {
			hint = "dismiss the keyboard (`sightmap device key return` on iOS, `sightmap device button back` on Android) or scroll, then retry"
		}
		what := s.Describe(o)
		if o.Id == "-" {
			what = "the band below the tabs"
		}
		return withHint("covered", hint, "%s is covered by the %s (%s)", desc, barName(o), what)
	}
	if o := s.drawnOver(n, cx, cy); o != nil {
		dir := "up"
		if float64(cy) > s.Screen.H/2 {
			dir = "down"
		}
		hint := fmt.Sprintf("scroll it clear (`sightmap device scroll %s --until %s`), then retry", dir, quote(target))
		// A large target, such as a sheet's dimmed backdrop, is covered in the
		// middle but shows around what is drawn over it.
		if x, y, ok := s.uncoveredSpot(n, rectOf(s.branchOf(o, n))); ok {
			hint = fmt.Sprintf("tap a part of it that shows with `sightmap device tap-xy %d %d`", x, y)
		} else if 2*float64(n.Bounds.Width*n.Bounds.Height) >= s.Screen.W*s.Screen.H {
			hint = "tap a part of it that shows with `sightmap device tap-xy` (see `sightmap device screenshot`), or close what is drawn over it"
		}
		return withHint("covered", hint,
			"%s is covered by %s, which is drawn over it; a tap at (%d, %d) would land there", desc, s.Describe(o), cx, cy)
	}
	if !Enabled(n) {
		return errorf("disabled", "%s is disabled", desc)
	}
	return nil
}

// drawnOver returns an element that takes taps drawn over the point (cx, cy)
// of n, or nil: e.g. a custom tab bar of plain buttons floating over a list, or
// a search field in a header the list scrolls under. Elements as large as half
// the screen are left out: they are containers, not something in the way.
func (s *Snapshot) drawnOver(n *sightmap.ComponentNode, cx, cy int) *sightmap.ComponentNode {
	if !slices.Contains(s.Nodes, n) {
		return nil
	}
	for _, o := range s.Nodes {
		b := o.Bounds
		if o == n || !s.takesTaps(o) || !o.IsVisible || s.IsAncestor(n, o) || s.IsAncestor(o, n) || !rectOf(o).contains(cx, cy) ||
			float64(b.Width*b.Height) >= s.Screen.W*s.Screen.H/2 || !s.drawnAbove(o, n) {
			continue
		}
		return o
	}
	return nil
}

// uncoveredSpot returns a point of n, outside zone, below the status bar and
// with nothing drawn over it, nearest n's center.
func (s *Snapshot) uncoveredSpot(n *sightmap.ComponentNode, zone rect) (x, y int, ok bool) {
	cx, cy := Center(n)
	b := n.Bounds
	best := math.Inf(1)
	for _, fx := range []float64{0.15, 0.3, 0.5, 0.7, 0.85} {
		for _, fy := range []float64{0.15, 0.25, 0.5, 0.75} {
			px, py := b.X+round(float64(b.Width)*fx), b.Y+round(float64(b.Height)*fy)
			if px < 0 || py < s.systemTop() || float64(px) > s.Screen.W || float64(py) > s.Screen.H || zone.contains(px, py) || s.drawnOver(n, px, py) != nil {
				continue
			}
			if d := math.Hypot(float64(px-cx), float64(py-cy)); d < best {
				best, x, y, ok = d, px, py, true
			}
		}
	}
	return x, y, ok
}

// takesTaps reports whether a tap on o reaches o: an interactive element, or a
// disabled one that would be, which still swallows the tap on Android.
func (t *Tree) takesTaps(o *sightmap.ComponentNode) bool {
	return o.IsInteractive || !Enabled(o) && slices.Contains(t.traits[o], "Button")
}

// drawnAbove reports whether o, which is neither n's ancestor nor its
// descendant, is drawn over n. Where they branch apart, the branch with the
// higher drawing order is in front (Android reports it, following elevation
// and zIndex); without one, the accessibility tree is in drawing order, so the
// later element is.
func (t *Tree) drawnAbove(o, n *sightmap.ComponentNode) bool {
	a, b := t.branchOf(o, n), t.branchOf(n, o)
	if oa, ob := t.order[a], t.order[b]; oa > 0 && ob > 0 && oa != ob {
		return oa > ob
	}
	return t.index(o) > t.index(n)
}

// branchOf returns o's ancestor (or o itself) whose parent is also n's
// ancestor: the subtree o is drawn in, apart from n's.
func (t *Tree) branchOf(o, n *sightmap.ComponentNode) *sightmap.ComponentNode {
	for b := o; b != nil; b = t.parent[b] {
		if p := t.parent[b]; p == nil || t.IsAncestor(p, n) {
			return b
		}
	}
	return o
}

func (t *Tree) index(n *sightmap.ComponentNode) int {
	i, _ := strconv.Atoi(n.Id)
	return i
}

// Relocate finds, in a fresh snapshot, the element that node (from an earlier
// snapshot) was. A snapshot id is matched by role, identifier and label, nearest
// the old position; any other target is resolved again.
func Relocate(fresh *Snapshot, node *sightmap.ComponentNode, target string) (*sightmap.ComponentNode, error) {
	if !IsSnapshotID(target) {
		return Resolve(fresh, target)
	}
	var same []*sightmap.ComponentNode
	for _, n := range fresh.Nodes {
		if n.Role == node.Role && Identifier(n) == Identifier(node) && n.Name == node.Name {
			same = append(same, n)
		}
	}
	if len(same) == 0 {
		return nil, errorf("stale_target", "element %s %s is no longer on screen", node.Id, node.Role+nameSuffix(node))
	}
	ox, oy := Center(node)
	dist := func(n *sightmap.ComponentNode) float64 {
		x, y := Center(n)
		return math.Hypot(float64(x-ox), float64(y-oy))
	}
	sort.SliceStable(same, func(i, j int) bool { return dist(same[i]) < dist(same[j]) })
	return same[0], nil
}

// towards returns the scroll direction that brings n's center onto the screen.
func (t *Tree) towards(n *sightmap.ComponentNode) string {
	cx, cy := Center(n)
	switch {
	case float64(cy) > t.Screen.H:
		return "down"
	case cy < 0:
		return "up"
	case float64(cx) > t.Screen.W:
		return "right"
	}
	return "left"
}

// scrollRegion is where a scroll swipes when no container is named: the
// largest visible scroll view, table or collection covering at least a quarter
// of the screen, else the whole screen.
func (t *Tree) scrollRegion() rect {
	screen := rect{0, 0, int(t.Screen.W), int(t.Screen.H)}
	best, bestArea := screen, int(t.Screen.W*t.Screen.H)/4
	for _, n := range t.Nodes {
		switch n.Role {
		case "scrollview", "table", "collection":
		default:
			continue
		}
		if r := clipRect(rectOf(n), screen); n.IsVisible && r.w*r.h >= bestArea {
			best, bestArea = r, r.w*r.h
		}
	}
	return best
}

func clipRect(r, to rect) rect {
	x0, y0 := max(r.x, to.x), max(r.y, to.y)
	x1, y1 := min(r.x+r.w, to.x+to.w), min(r.y+r.h, to.y+to.h)
	return rect{x0, y0, max(0, x1-x0), max(0, y1-y0)}
}

// containerInset keeps a tap off the very edge of a scroll container, where a
// row is clipped or a bar is drawn over it.
const containerInset = 8

// systemTop is the height of the status bar area, where a tap scrolls the
// list to the top rather than reaching what is drawn there: generous on a
// notched or Dynamic Island iPhone, small on an iPad or a home-button iPhone.
// It stays below the center of a navigation bar item on every device.
func (t *Tree) systemTop() int {
	if t.StatusBar > 0 {
		return int(math.Ceil(t.StatusBar))
	}
	switch {
	case t.Screen.W >= 600:
		return 24
	case t.Screen.H >= 812:
		return 60
	}
	return 20
}

// scrollContainer returns n's nearest scroll view, table or collection, or nil.
func (t *Tree) scrollContainer(n *sightmap.ComponentNode) *sightmap.ComponentNode {
	for p := t.parent[n]; p != nil; p = t.parent[p] {
		switch p.Role {
		case "scrollview", "table", "collection":
			return p
		}
	}
	return nil
}

// topBar returns the bottom edge of a navigation bar drawn over n's list, or
// 0. A collapsed navigation bar covers the rows scrolled under it but shows in
// the tree only as its title (and its buttons): a heading or navbar element,
// or a control, in the top 15% of the screen and outside n's list. The bar
// reaches a little below its title.
func (t *Tree) topBar(n *sightmap.ComponentNode) int {
	list := t.scrollContainer(n)
	if list == nil {
		list = t.parent[n]
	}
	// In a flat tree (React Native's, often) the list is the root, so only a
	// title says where a bar is; any control could be another row.
	flat := list == t.Root
	if flat {
		list = nil
	}
	bottom := 0
	for _, o := range t.Nodes {
		b := o.Bounds
		if !o.IsVisible || o == n || b.Y+b.Height > int(0.15*t.Screen.H) || b.Height <= 0 {
			continue
		}
		// The bar's own items (a back button) sit beside its title; only what
		// lies under the bar, in another layer, is covered by it.
		if list != nil && (o == list || t.IsAncestor(list, o)) || t.IsAncestor(o, n) || t.parent[o] == t.parent[n] {
			continue
		}
		if o.Role == "heading" || o.Role == "navbar" || (o.IsInteractive && !flat) {
			bottom = max(bottom, b.Y+b.Height+navBarMargin)
		}
	}
	return bottom
}

// navBarMargin is how far a navigation bar reaches below its title.
const navBarMargin = 16
