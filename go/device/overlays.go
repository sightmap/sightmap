package device

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sightmap/sightmap/go/sightmap"
)

// A React Native LogBox toast (Debug builds) is one accessible element labeled
// with its icon or count first; its ✕ has no element of its own.
var logBoxLabel = regexp.MustCompile(`^(!|\d+), `)

// LogBox is a React Native LogBox toast and the point of its ✕.
type LogBox struct {
	Node     *sightmap.ComponentNode
	DismissX int
	DismissY int
}

// logBoxToast returns the LogBox toast in the bottom half of the screen, or nil.
func (t *Tree) logBoxToast() *LogBox {
	for _, n := range t.Nodes {
		if n.Role == "other" && Identifier(n) == "" && n.IsVisible && float64(n.Bounds.Y) > t.Screen.H/2 && logBoxLabel.MatchString(n.Name) {
			b := n.Bounds
			return &LogBox{Node: n, DismissX: b.X + b.Width - 24, DismissY: b.Y + b.Height/2}
		}
	}
	return nil
}

// floatingOnly returns the visible nodes when everything visible fits in a
// small box: only something like the React Native / Expo dev-menu button, which
// lives in a separate window, so the app's own window is most likely hidden
// from accessibility. Otherwise nil.
func (t *Tree) floatingOnly() []*sightmap.ComponentNode {
	if t.Overlay {
		return nil
	}
	var shown []*sightmap.ComponentNode
	for _, n := range t.Nodes {
		if n != t.Root && n.IsVisible && n.Role != "window" && !n.IsIgnored {
			shown = append(shown, n)
		}
	}
	if len(shown) == 0 {
		return nil
	}
	x0, y0, x1, y1 := shown[0].Bounds.X, shown[0].Bounds.Y, 0, 0
	for _, n := range shown {
		b := n.Bounds
		x0, y0 = min(x0, b.X), min(y0, b.Y)
		x1, y1 = max(x1, b.X+b.Width), max(y1, b.Y+b.Height)
	}
	if float64((x1-x0)*(y1-y0)) < 0.1*t.Screen.W*t.Screen.H {
		return shown
	}
	return nil
}

var devMenuLabel = regexp.MustCompile(`(?i)\b(dev(eloper)?[\s-]?menu|dev[\s-]?tools)\b`)

// floatingMargin is how far beyond its frame a floating button still catches
// taps, in points.
const floatingMargin = 24

// devMenuSymbol matches the SF Symbol names dev-menu buttons draw, which the
// accessibility tree reports as their identifier (the Expo dev client's gear
// is `image[identifier="gearshape.fill"]`).
var devMenuSymbol = regexp.MustCompile(`^(gearshape(\.2)?|gear|ellipsis\.circle|wrench(\.and\.screwdriver)?|hammer)(\.(fill|circle))*$`)

func small(n *sightmap.ComponentNode) bool {
	return n.Bounds.Width <= 96 && n.Bounds.Height <= 96
}

// fabShaped: a small, roughly square leaf whose identifier, if any, is a
// dev-menu symbol rather than one the app gave it.
func fabShaped(n *sightmap.ComponentNode) bool {
	w, h := float64(n.Bounds.Width), float64(n.Bounds.Height)
	id := Identifier(n)
	return len(n.Children) == 0 && (id == "" || devMenuSymbol.MatchString(id)) && w >= 32 && h >= 32 && small(n) && w/h >= 0.6 && w/h <= 1.67
}

// floatingControls returns controls that most likely float in a separate small
// window above the app, like the React Native / Expo dev-menu button. describe
// reports no windows, so this is a heuristic: a dev-menu label or identifier; a
// small element named after a dev-menu symbol (a gear) outside any control of
// the app's; or a small square leaf drawn last at the root, as a window above
// the app's own would be.
func (t *Tree) floatingControls() []*sightmap.ComponentNode {
	var top []*sightmap.ComponentNode
	for _, c := range t.Root.Children {
		if c.IsVisible {
			top = append(top, c)
		}
	}
	var last *sightmap.ComponentNode
	if len(top) > 1 {
		last = top[len(top)-1]
	}
	var out []*sightmap.ComponentNode
	for _, n := range t.Nodes {
		if n == t.Root || !n.IsVisible {
			continue
		}
		symbol := devMenuSymbol.MatchString(Identifier(n)) && small(n) && !t.hasAncestor(n, func(a *sightmap.ComponentNode) bool { return a.IsInteractive })
		if devMenuLabel.MatchString(n.Name) || devMenuLabel.MatchString(Identifier(n)) || symbol || (n == last && fabShaped(n)) {
			out = append(out, n)
		}
	}
	return out
}

// sparseNote explains why a snapshot offers (next to) nothing to act on, as
// lines naming the likely causes, or nil when it looks healthy. snapshot shows
// it, and click, fill and scroll use it in their not-found errors.
func (s *Snapshot) sparseNote(udid string, accessibilityOn bool) []string {
	if s.Overlay {
		return nil
	}
	causes := emptyTreeCauses(udid, accessibilityOn)
	if floating := s.floatingOnly(); floating != nil {
		var desc []string
		for _, n := range floating[:min(3, len(floating))] {
			d := n.Role
			if n.Name != "" {
				d += " " + quote(n.Name)
			}
			desc = append(desc, d)
		}
		return append([]string{fmt.Sprintf("Only a small floating element is exposed (%s). It is probably a separate window, such as the React Native / Expo dev-menu button, and the app's own window shows nothing to accessibility.", strings.Join(desc, ", "))}, causes...)
	}
	if s.Coverage.Total == 0 {
		return append([]string{"The app exposes no interactive elements. If it is still loading, snapshot again in a moment; otherwise, likely causes:"}, causes...)
	}
	return nil
}
