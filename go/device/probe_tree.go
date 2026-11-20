package device

import (
	"strings"

	"github.com/sightmap/sightmap/go/projection"
)

// The probe reads the app's accessibility tree from inside the process, where
// the out-of-process helper cannot: SwiftUI containers that carry an identifier
// but no label of their own (a list, a card), the identifiers of every element,
// and the frames and traits the app set. ProbeTree turns that tree into the
// RawElement tree Normalize reads, after reducing it to the nodes a person or
// an agent can perceive or operate (package projection), so a snapshot of it
// has the shape lidar's tree of a captured session has.

// UIAccessibilityTraits bits, with the private ones SwiftUI and UIKit set.
const (
	traitButton      = 1 << 0
	traitLink        = 1 << 1
	traitImage       = 1 << 2
	traitSelected    = 1 << 3
	traitKeyboardKey = 1 << 5
	traitStaticText  = 1 << 6
	traitNotEnabled  = 1 << 8
	traitSearchField = 1 << 10
	traitAdjustable  = 1 << 12
	traitTextEntry   = 1 << 18
	traitSecureField = 1 << 24
	traitHeader      = 1 << 16
	traitToggle      = 1 << 53
)

var traitNames = []struct {
	bit  uint64
	name string
}{
	{traitButton, "Button"}, {traitLink, "Link"}, {traitImage, "Image"}, {traitSelected, "Selected"},
	{traitKeyboardKey, "KeyboardKey"}, {traitStaticText, "StaticText"}, {traitNotEnabled, "NotEnabled"},
	{traitSearchField, "SearchField"}, {traitAdjustable, "Adjustable"}, {traitHeader, "Header"},
}

// ProbeTree returns root with its children replaced by the projection of the
// probe's accessibility trees (one per window). It returns nil when the probe
// sent none, so the caller keeps the out-of-process tree.
func ProbeTree(root *RawElement, windows []*ProbeAX, screen Size) *RawElement {
	var kids []*projection.Node
	for _, w := range windows {
		kids = append(kids, axNodes(w, screen)...)
	}
	if len(kids) == 0 {
		return nil
	}
	p := projection.Project(&projection.Node{Kind: projection.KindRoot, Kids: dedupe(kids)})
	out := *root
	out.Children = nil
	for _, k := range p.Kids {
		out.Children = append(out.Children, rawFromNode(k))
	}
	return &out
}

// axNodes converts one in-process node, and what is below it, to projection
// nodes: none when it and its children carry nothing, its children alone when it
// is only a container.
func axNodes(n *ProbeAX, screen Size) []*projection.Node {
	// Something laid out entirely off screen (the next page of a pager, a row
	// scrolled away) is not on the screen the snapshot describes.
	if f := (Frame{X: n.F[0], Y: n.F[1], W: n.F[2], H: n.F[3]}); f.W > 0 && f.H > 0 && !intersects(f, screen) {
		return nil
	}
	var kids []*projection.Node
	for _, k := range n.Kids {
		kids = append(kids, axNodes(k, screen)...)
	}
	if isUIKitChrome(n) {
		return kids
	}
	id := appID(n)
	kind := axKind(n)
	if kind == "" && !n.El && id == "" && n.Label == "" {
		return kids
	}
	if kind == "" {
		kind = projection.KindGroup
	}
	return []*projection.Node{{Kind: kind, ID: id, Text: n.Label, Kids: dedupe(kids), Ref: n}}
}

// axKind is the neutral kind of n, from the traits the app set and the class UIKit
// gave it, or "" when it is a container.
func axKind(n *ProbeAX) string {
	t := n.Traits
	cls := shortClass(n.Class)
	switch {
	case t&traitToggle != 0:
		return projection.KindSwitch
	case strings.HasPrefix(cls, "_UITabButton"):
		return projection.KindTab
	case cls == "UITabBar":
		return projection.KindTabList
	case isNavigationBar(cls):
		return projection.KindNavigation
	case t&traitTextEntry != 0 || t&traitSearchField != 0:
		return projection.KindTextbox
	case t&traitButton != 0:
		return projection.KindButton
	case t&traitLink != 0:
		return projection.KindLink
	case t&traitHeader != 0:
		return projection.KindHeading
	case t&traitStaticText != 0:
		return projection.KindText
	case t&traitImage != 0:
		return projection.KindImage
	case t&traitAdjustable != 0:
		return projection.KindSlider
	}
	return ""
}

func shortClass(c string) string {
	if i := strings.LastIndex(c, "."); i >= 0 {
		return c[i+1:]
	}
	return c
}

// isNavigationBar reports whether class is a navigation bar itself, not one of
// the views UIKit builds inside it (the large-title view, the platter).
func isNavigationBar(class string) bool {
	return class == "UINavigationBar" || class == "UIKitNavigationBar"
}

// isUIKitChrome reports whether n is something UIKit draws around the app's
// content, not part of it: a scroll bar, or the dimming view it lays over a
// screen behind a sheet. What is inside it still belongs to the app.
func isUIKitChrome(n *ProbeAX) bool {
	cls := shortClass(n.Class)
	return strings.HasPrefix(cls, "_UIScrollViewScrollIndicator") || n.ID == "AdditionalDimmingOverlay"
}

// appID is n's identifier when the app set it. UIKit also names a navigation
// bar after its title, which masking can hide, so a selector must not lean on
// it, and it says nothing about the app.
func appID(n *ProbeAX) string {
	if isNavigationBar(shortClass(n.Class)) {
		return ""
	}
	return n.ID
}

// dedupe drops a sibling that repeats an earlier one's kind, text and frame:
// UIKit's tab bar exposes its buttons twice, once as the selection layer and
// once as the content. The survivor takes the identifier if only the repeat
// has one.
func dedupe(nodes []*projection.Node) []*projection.Node {
	type key struct {
		kind, text string
		f          [4]int
	}
	seen := map[key]*projection.Node{}
	var out []*projection.Node
	for _, n := range nodes {
		ax := n.Ref.(*ProbeAX)
		k := key{n.Kind, n.Text, [4]int{round(ax.F[0]), round(ax.F[1]), round(ax.F[2]), round(ax.F[3])}}
		if first, ok := seen[k]; ok && ax.F[2] > 0 {
			if first.ID == "" {
				first.ID = n.ID
			}
			continue
		}
		seen[k] = n
		out = append(out, n)
	}
	return out
}

// rawFromNode builds the element for a projected node. Its type follows the
// node's kind: the rules may have changed it (a group that carries an identifier
// and a text is a text), in which case the element's own class no longer says it.
func rawFromNode(n *projection.Node) *RawElement {
	ax := n.Ref.(*ProbeAX)
	e := &RawElement{
		Label:      n.Text,
		Identifier: n.ID,
		Value:      Value(ax.Value),
		Frame:      Frame{X: ax.F[0], Y: ax.F[1], W: ax.F[2], H: ax.F[3]},
		Traits:     axTraits(ax.Traits),
	}
	if ax.Traits&traitNotEnabled != 0 {
		off := false
		e.Enabled = &off
	}
	e.Type, e.Subrole = axType(n.Kind, ax)
	for _, k := range n.Kids {
		e.Children = append(e.Children, rawFromNode(k))
	}
	return e
}

// axType is the helper's element type (and subrole) for a projected kind.
func axType(kind string, ax *ProbeAX) (typ, subrole string) {
	switch kind {
	case projection.KindTab:
		return "Button", "AXTabButton"
	case projection.KindTabList:
		return "TabBar", ""
	case projection.KindNavigation:
		return "NavigationBar", ""
	case projection.KindButton:
		return "Button", ""
	case projection.KindLink:
		return "Link", ""
	case projection.KindHeading:
		return "Heading", ""
	case projection.KindText:
		return "StaticText", ""
	case projection.KindImage:
		return "Image", ""
	case projection.KindSwitch:
		return "Switch", ""
	case projection.KindSlider:
		return "Slider", ""
	case projection.KindTextbox:
		switch {
		case ax.Secure || ax.Traits&traitSecureField != 0:
			return "SecureTextField", ""
		case ax.Traits&traitSearchField != 0:
			return "SearchField", ""
		}
		return "TextField", ""
	}
	return "Group", ""
}

func axTraits(mask uint64) []string {
	var out []string
	for _, t := range traitNames {
		if mask&t.bit != 0 {
			out = append(out, t.name)
		}
	}
	return out
}
