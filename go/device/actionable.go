package device

import (
	"fmt"
	"math"
	"sort"
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
		dir := "left"
		switch {
		case float64(cy) > s.Screen.H:
			dir = "down"
		case cy < 0:
			dir = "up"
		case float64(cx) > s.Screen.W:
			dir = "right"
		}
		return withHint("offscreen", fmt.Sprintf("run `sightmap device scroll %s --until %s`, then retry", dir, quote(target)),
			"%s is off-screen (center %d,%d on a %gx%g screen); refusing to tap", desc, cx, cy, s.Screen.W, s.Screen.H)
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
			hint = "dismiss the keyboard (`sightmap device key return`) or scroll, then retry"
		}
		what := s.Describe(o)
		if o.Id == "-" {
			what = "the band below the tabs"
		}
		return withHint("covered", hint, "%s is covered by the %s (%s)", desc, barName(o), what)
	}
	if !Enabled(n) {
		return errorf("disabled", "%s is disabled", desc)
	}
	return nil
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
